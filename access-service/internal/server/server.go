// Package server implements the access service's three HTTP endpoints —
// /activate, /verify, /keys — over the store, mailer, and key-minter seams.
// It holds the wire contract the app's internal/access client mirrors.
package server

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"

	"mogi-access/internal/mailer"
	"mogi-access/internal/openrouter"
	"mogi-access/internal/store"
)

const (
	// otpTTL is how long a code stays valid. The mailer text states this too.
	otpTTL = 10 * time.Minute
	// maxOTPAttempts is the wrong-guess ceiling before a code is discarded —
	// this cap plus the short TTL, not hash strength, protect the 6-digit space.
	maxOTPAttempts = 5
	// tokenBytes is the session token entropy (256 bits, base64url-encoded).
	tokenBytes = 32
)

// Config carries the request-time values the handlers need beyond the store's
// own Config (which holds the console-editable kill switch and model pin).
type Config struct {
	ORKeyLimitUSD float64 // per-tester OpenRouter credit cap passed to Mint
	GoogleKey     string  // shared TTS/STT-restricted Google key
	ElevenLabsKey string  // shared STT-scoped ElevenLabs key
}

// server holds the dependencies and rate limiters for the handlers.
type server struct {
	store  store.Store
	mailer mailer.Mailer
	minter openrouter.KeyMinter
	cfg    Config

	activateEmailRL *limiter // 3 activations/email/hour
	activateIPRL    *limiter // 10 activations/IP/hour
}

// New builds the HTTP handler for the access service.
func New(st store.Store, m mailer.Mailer, minter openrouter.KeyMinter, cfg Config) http.Handler {
	s := &server{
		store:           st,
		mailer:          m,
		minter:          minter,
		cfg:             cfg,
		activateEmailRL: newLimiter(3, time.Hour),
		activateIPRL:    newLimiter(10, time.Hour),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /activate", s.handleActivate)
	mux.HandleFunc("POST /verify", s.handleVerify)
	mux.HandleFunc("GET /keys", s.handleKeys)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return mux
}

// --- wire types ---

type activateRequest struct {
	Email      string `json:"email"`
	InviteCode string `json:"inviteCode"`
}

type verifyRequest struct {
	Email string `json:"email"`
	Code  string `json:"code"`
}

// keysPayload is the managed key set. It is returned flat by /keys and nested
// under "keys" by /verify — one struct, so both endpoints stay identical.
type keysPayload struct {
	OpenRouter  string `json:"openrouter"`
	Google      string `json:"google"`
	ElevenLabs  string `json:"elevenlabs"`
	PinnedModel string `json:"pinnedModel"`
}

type verifyResponse struct {
	Token string      `json:"token"`
	Keys  keysPayload `json:"keys"`
}

// --- handlers ---

// handleActivate validates the invite, then (only for valid invites) rate-limits
// and mails an OTP. The ordering is load-bearing: validating the invite before
// consuming rate budget or sending mail stops a non-invitee from using this
// endpoint as a spam relay.
func (s *server) handleActivate(w http.ResponseWriter, r *http.Request) {
	var req activateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	email := normalizeEmail(req.Email)
	code := strings.TrimSpace(req.InviteCode)
	if email == "" || code == "" {
		writeError(w, http.StatusBadRequest, "email and inviteCode are required")
		return
	}
	ctx := r.Context()

	// Kill switch: no new activations when the test phase is off.
	cfg, err := s.store.GetConfig(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !cfg.TestPhaseActive {
		writeError(w, http.StatusForbidden, "the test phase is not currently active")
		return
	}

	// Validate the invite BEFORE any rate-limit consumption or mail.
	inv, err := s.store.GetInvite(ctx, code)
	if err != nil || !inv.Active || inv.Uses >= inv.MaxUses {
		writeError(w, http.StatusBadRequest, "invalid or exhausted invite code")
		return
	}

	// Rate limits — reached only by valid-invite requests. Evaluate both (no
	// short-circuit) so the counters increment consistently.
	emailOK := s.activateEmailRL.allow(email)
	ipOK := s.activateIPRL.allow(clientIP(r))
	if !emailOK || !ipOK {
		writeError(w, http.StatusTooManyRequests, "too many requests — please try again later")
		return
	}

	otp, err := genOTP()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	rec := store.OTP{
		EmailHash:  hashString(email),
		CodeHash:   hashString(otp),
		InviteCode: code,
		ExpiresAt:  time.Now().Add(otpTTL),
	}
	if err := s.store.PutOTP(ctx, rec); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := s.mailer.SendOTP(ctx, email, otp); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to send verification code")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleVerify checks the OTP and, on success, commits the invite use (new
// testers only), mints/reuses the tester's OpenRouter key, opens a session, and
// returns the token plus the managed key set.
func (s *server) handleVerify(w http.ResponseWriter, r *http.Request) {
	var req verifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	email := normalizeEmail(req.Email)
	code := strings.TrimSpace(req.Code)
	if email == "" || code == "" {
		writeError(w, http.StatusBadRequest, "email and code are required")
		return
	}
	ctx := r.Context()
	emailHash := hashString(email)

	otp, err := s.store.GetOTP(ctx, emailHash)
	if err != nil {
		writeError(w, http.StatusBadRequest, "no pending code for this email — request a new one")
		return
	}
	if time.Now().After(otp.ExpiresAt) {
		_ = s.store.DeleteOTP(ctx, emailHash)
		writeError(w, http.StatusBadRequest, "code expired — request a new one")
		return
	}
	if hashString(code) != otp.CodeHash {
		otp.Attempts++
		if otp.Attempts >= maxOTPAttempts {
			_ = s.store.DeleteOTP(ctx, emailHash)
			writeError(w, http.StatusBadRequest, "too many incorrect attempts — request a new code")
			return
		}
		_ = s.store.PutOTP(ctx, otp)
		writeError(w, http.StatusBadRequest, "incorrect code")
		return
	}

	// Correct code — spend it.
	_ = s.store.DeleteOTP(ctx, emailHash)

	// Reuse an existing tester's key so re-verifying never mints twice or
	// consumes a second invite use; only first-time testers mint.
	tester, terr := s.store.GetTester(ctx, email)
	if terr != nil || tester.ORKey == "" {
		if err := s.store.ConsumeInvite(ctx, otp.InviteCode); err != nil {
			writeError(w, http.StatusBadRequest, "invite code is no longer available")
			return
		}
		key, hash, err := s.minter.Mint(ctx, email, s.cfg.ORKeyLimitUSD)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not provision access — please try again")
			return
		}
		tester = store.Tester{
			Email:      email,
			InviteCode: otp.InviteCode,
			ORKey:      key,
			ORKeyHash:  hash,
			CreatedAt:  time.Now(),
		}
		if err := s.store.PutTester(ctx, tester); err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	token, err := genToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := s.store.PutSession(ctx, store.Session{
		TokenHash: hashString(token),
		Email:     email,
		CreatedAt: time.Now(),
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	cfg, err := s.store.GetConfig(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, verifyResponse{Token: token, Keys: s.keysFor(tester, cfg)})
}

// handleKeys re-serves the managed key set for a valid session. It is the
// enforcement point: the kill switch and per-tester revocation both surface
// here as a 403, so the app signs out on its next launch refresh.
func (s *server) handleKeys(w http.ResponseWriter, r *http.Request) {
	token := bearerToken(r)
	if token == "" {
		writeError(w, http.StatusUnauthorized, "missing bearer token")
		return
	}
	ctx := r.Context()

	sess, err := s.store.GetSession(ctx, hashString(token))
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid or expired session")
		return
	}

	cfg, err := s.store.GetConfig(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !cfg.TestPhaseActive {
		writeError(w, http.StatusForbidden, "the Mogi test phase has ended — thanks for testing!")
		return
	}

	tester, err := s.store.GetTester(ctx, sess.Email)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid or expired session")
		return
	}
	if tester.Revoked {
		writeError(w, http.StatusForbidden, "this test account has been deactivated")
		return
	}

	writeJSON(w, http.StatusOK, s.keysFor(tester, cfg))
}

// keysFor assembles the managed key set from the tester's minted key, the
// shared voice keys, and the current pinned model.
func (s *server) keysFor(t store.Tester, cfg store.Config) keysPayload {
	return keysPayload{
		OpenRouter:  t.ORKey,
		Google:      s.cfg.GoogleKey,
		ElevenLabs:  s.cfg.ElevenLabsKey,
		PinnedModel: cfg.PinnedModel,
	}
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// normalizeEmail is the single place emails are canonicalized, so hashing and
// tester lookups always agree.
func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// hashString returns the hex SHA-256 of s. Used for emails, codes, and tokens
// so raw values are not persisted.
func hashString(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// genOTP returns a uniformly-random 6-digit code (crypto/rand, no modulo bias).
func genOTP() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// genToken returns a 256-bit base64url session token.
func genToken() (string, error) {
	b := make([]byte, tokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// bearerToken extracts the token from an "Authorization: Bearer <token>" header.
func bearerToken(r *http.Request) string {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if len(h) > len(prefix) && strings.EqualFold(h[:len(prefix)], prefix) {
		return strings.TrimSpace(h[len(prefix):])
	}
	return ""
}

// clientIP returns the caller's IP for rate limiting. Cloud Run sets
// X-Forwarded-For as "client, proxy…"; the first hop is the real client.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	return r.RemoteAddr
}
