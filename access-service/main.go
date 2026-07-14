// Command mogi-access is the Mogi test-phase "access service": it gates
// developer-funded API keys behind an invite code + email OTP so testers can
// run the app without obtaining their own keys. It hands out keys; it never
// proxies interview traffic (screenshots and audio go straight from the app to
// the providers). See docs/managed-keys-plan.md in the app repo.
//
// It is a separate Go module (module mogi-access) so its dependencies never
// touch the root mogi app module. Everything is wired from environment
// variables here; the packages under internal/ hold the logic.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"mogi-access/internal/mailer"
	"mogi-access/internal/openrouter"
	"mogi-access/internal/server"
	"mogi-access/internal/store"
)

// config is the fully-resolved runtime configuration, read once from the
// environment at startup. Every field maps to one env var (see loadConfig).
type config struct {
	port            string
	store           string // "memory" (local/dev) | "firestore" (prod)
	mailer          string // "log" (local/dev) | "resend" (prod)
	devInviteCode   string // seeds a high-use invite in the memory store only
	testPhaseActive bool   // kill switch default for the memory store's config
	pinnedModel     string // model id served to managed clients

	openRouterProvKey string  // OpenRouter provisioning key; empty ⇒ stub minter
	orKeyLimitUSD     float64 // per-tester OpenRouter credit cap

	googleSharedKey     string // shared, TTS/STT-restricted Google key
	elevenLabsSharedKey string // shared, STT-scoped ElevenLabs key

	resendAPIKey string // Resend key (only used when mailer == "resend")
	mailFrom     string // From: address for OTP mail
	gcpProject   string // GCP project id (only used when store == "firestore")
}

// loadConfig resolves configuration from the environment, applying the same
// local-first defaults the README documents: an in-memory store and a log
// mailer, so the service runs with zero cloud setup.
func loadConfig() config {
	return config{
		port:            getenv("PORT", "8787"),
		store:           getenv("STORE", "memory"),
		mailer:          getenv("MAILER", "log"),
		devInviteCode:   getenv("DEV_INVITE_CODE", "MOGI-DEV"),
		testPhaseActive: getenvBool("TEST_PHASE_ACTIVE", true),
		pinnedModel:     getenv("PINNED_MODEL", "google/gemini-2.5-flash"),

		openRouterProvKey: os.Getenv("OPENROUTER_PROVISIONING_KEY"),
		orKeyLimitUSD:     getenvFloat("OR_KEY_LIMIT_USD", 3),

		googleSharedKey:     os.Getenv("GOOGLE_SHARED_KEY"),
		elevenLabsSharedKey: os.Getenv("ELEVENLABS_SHARED_KEY"),

		resendAPIKey: os.Getenv("RESEND_API_KEY"),
		mailFrom:     getenv("MAIL_FROM", "Mogi <onboarding@resend.dev>"),
		gcpProject:   os.Getenv("GCP_PROJECT"),
	}
}

func main() {
	if err := run(); err != nil {
		log.Fatalf("mogi-access: %v", err)
	}
}

// run wires the service from config and serves until interrupted. It is split
// from main so startup errors return rather than exit, keeping main trivial.
func run() error {
	cfg := loadConfig()

	handler, err := buildHandler(cfg)
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              ":" + cfg.port,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Serve in the background so the main goroutine can wait for a shutdown
	// signal and drain in-flight requests gracefully.
	errCh := make(chan error, 1)
	go func() {
		log.Printf("mogi-access: listening on :%s (store=%s mailer=%s)", cfg.port, cfg.store, cfg.mailer)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}

// buildHandler constructs the HTTP handler from config. It is the single place
// that selects concrete store/mailer/minter implementations, so swapping local
// stubs for cloud services is env-only.
func buildHandler(cfg config) (http.Handler, error) {
	// Store: in-memory for local/dev, Firestore for prod (Phase 3.2).
	var st store.Store
	switch cfg.store {
	case "memory":
		st = store.NewMemory(cfg.devInviteCode, store.Config{
			TestPhaseActive: cfg.testPhaseActive,
			PinnedModel:     cfg.pinnedModel,
		})
	case "firestore":
		return nil, fmt.Errorf("store %q not implemented yet (Phase 3.2)", cfg.store)
	default:
		return nil, fmt.Errorf("unknown STORE %q (want memory|firestore)", cfg.store)
	}

	// Mailer: log to stdout for local/dev, Resend for prod.
	var m mailer.Mailer
	switch cfg.mailer {
	case "log":
		m = mailer.NewLog()
	case "resend":
		m = mailer.NewResend(cfg.resendAPIKey, cfg.mailFrom)
	default:
		return nil, fmt.Errorf("unknown MAILER %q (want log|resend)", cfg.mailer)
	}

	// Minter: real OpenRouter provisioning when a key is set, else a stub so the
	// full flow runs locally without spend.
	var minter openrouter.KeyMinter
	if cfg.openRouterProvKey != "" {
		minter = openrouter.NewClient(cfg.openRouterProvKey)
	} else {
		minter = openrouter.NewStubMinter()
		log.Printf("mogi-access: no OPENROUTER_PROVISIONING_KEY set — using stub key minter (no real keys minted)")
	}

	return server.New(st, m, minter, server.Config{
		ORKeyLimitUSD: cfg.orKeyLimitUSD,
		GoogleKey:     cfg.googleSharedKey,
		ElevenLabsKey: cfg.elevenLabsSharedKey,
	}), nil
}

// getenv returns the env var or a default when unset/empty.
func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// getenvBool parses a boolean env var, falling back to def on empty/invalid.
func getenvBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

// getenvFloat parses a float env var, falling back to def on empty/invalid.
func getenvFloat(key string, def float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return def
	}
	return f
}
