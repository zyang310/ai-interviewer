package store

// Integration tests for the Firestore store, run against the local emulator:
//
//	gcloud emulators firestore start --host-port=127.0.0.1:8790
//	FIRESTORE_EMULATOR_HOST=127.0.0.1:8790 go test ./internal/store/
//
// They skip when FIRESTORE_EMULATOR_HOST is unset, so the normal suite stays
// cloud-free. Each test uses a unique project id, which the emulator treats as
// an isolated namespace — no cross-test cleanup needed.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// newEmulatorStore returns a Firestore store bound to a fresh emulator
// namespace, or skips the test when no emulator is configured.
func newEmulatorStore(t *testing.T) (*Firestore, context.Context) {
	t.Helper()
	if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		t.Skip("FIRESTORE_EMULATOR_HOST not set — Firestore emulator tests skipped")
	}
	ctx := context.Background()
	f, err := NewFirestore(ctx, fmt.Sprintf("mogi-access-test-%d", time.Now().UnixNano()))
	if err != nil {
		t.Fatalf("NewFirestore: %v", err)
	}
	return f, ctx
}

// seedInvite writes an invite doc directly — the Store interface deliberately
// has no PutInvite (invites are operator-minted via the console / seed docs).
func seedInvite(t *testing.T, f *Firestore, inv Invite) {
	t.Helper()
	if _, err := f.client.Collection(colInvites).Doc(inv.Code).Set(context.Background(), inv); err != nil {
		t.Fatalf("seed invite: %v", err)
	}
}

func TestFirestoreInviteLifecycle(t *testing.T) {
	f, ctx := newEmulatorStore(t)

	if _, err := f.GetInvite(ctx, "NOPE"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing invite err = %v, want ErrNotFound", err)
	}
	if err := f.ConsumeInvite(ctx, "NOPE"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("consume missing invite err = %v, want ErrNotFound", err)
	}

	seedInvite(t, f, Invite{Code: "MOGI-1", MaxUses: 2, Active: true})
	inv, err := f.GetInvite(ctx, "MOGI-1")
	if err != nil || inv.Code != "MOGI-1" || inv.MaxUses != 2 || !inv.Active || inv.Uses != 0 {
		t.Fatalf("round trip = %+v (err %v)", inv, err)
	}

	if err := f.ConsumeInvite(ctx, "MOGI-1"); err != nil {
		t.Fatalf("consume 1: %v", err)
	}
	if err := f.ConsumeInvite(ctx, "MOGI-1"); err != nil {
		t.Fatalf("consume 2: %v", err)
	}
	if err := f.ConsumeInvite(ctx, "MOGI-1"); !errors.Is(err, ErrInviteUnavailable) {
		t.Fatalf("consume past MaxUses err = %v, want ErrInviteUnavailable", err)
	}
	if inv, _ = f.GetInvite(ctx, "MOGI-1"); inv.Uses != 2 {
		t.Fatalf("uses after exhaustion = %d, want 2", inv.Uses)
	}

	seedInvite(t, f, Invite{Code: "MOGI-OFF", MaxUses: 5, Active: false})
	if err := f.ConsumeInvite(ctx, "MOGI-OFF"); !errors.Is(err, ErrInviteUnavailable) {
		t.Fatalf("consume disabled invite err = %v, want ErrInviteUnavailable", err)
	}
}

// TestFirestoreConsumeInviteRace is the reason ConsumeInvite is a transaction:
// concurrent racers for a nearly-exhausted invite must win exactly MaxUses
// times, never more.
func TestFirestoreConsumeInviteRace(t *testing.T) {
	f, ctx := newEmulatorStore(t)
	seedInvite(t, f, Invite{Code: "MOGI-RACE", MaxUses: 3, Active: true})

	const racers = 10
	var wg sync.WaitGroup
	errs := make([]error, racers)
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = f.ConsumeInvite(ctx, "MOGI-RACE")
		}(i)
	}
	wg.Wait()

	wins := 0
	for i, err := range errs {
		switch {
		case err == nil:
			wins++
		case errors.Is(err, ErrInviteUnavailable):
		default:
			t.Fatalf("racer %d unexpected err: %v", i, err)
		}
	}
	if wins != 3 {
		t.Fatalf("wins = %d, want exactly MaxUses (3)", wins)
	}
	if inv, _ := f.GetInvite(ctx, "MOGI-RACE"); inv.Uses != 3 {
		t.Fatalf("final uses = %d, want 3 (no over-consume)", inv.Uses)
	}
}

func TestFirestoreOTPRoundTrip(t *testing.T) {
	f, ctx := newEmulatorStore(t)

	if _, err := f.GetOTP(ctx, "h1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing OTP err = %v, want ErrNotFound", err)
	}

	// Truncate to microseconds — Firestore timestamp precision — so equality
	// holds through the round trip.
	exp := time.Now().Add(10 * time.Minute).UTC().Truncate(time.Microsecond)
	want := OTP{EmailHash: "h1", CodeHash: "c1", InviteCode: "MOGI-1", ExpiresAt: exp}
	if err := f.PutOTP(ctx, want); err != nil {
		t.Fatalf("put OTP: %v", err)
	}
	got, err := f.GetOTP(ctx, "h1")
	if err != nil || got.CodeHash != "c1" || got.InviteCode != "MOGI-1" || got.Attempts != 0 || !got.ExpiresAt.Equal(exp) {
		t.Fatalf("round trip = %+v (err %v), want %+v", got, err, want)
	}

	// PutOTP is an upsert (failed attempts overwrite the record).
	want.Attempts = 3
	if err := f.PutOTP(ctx, want); err != nil {
		t.Fatalf("upsert OTP: %v", err)
	}
	if got, _ = f.GetOTP(ctx, "h1"); got.Attempts != 3 {
		t.Fatalf("attempts after upsert = %d, want 3", got.Attempts)
	}

	if err := f.DeleteOTP(ctx, "h1"); err != nil {
		t.Fatalf("delete OTP: %v", err)
	}
	if _, err := f.GetOTP(ctx, "h1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted OTP err = %v, want ErrNotFound", err)
	}
	if err := f.DeleteOTP(ctx, "h1"); err != nil {
		t.Fatalf("second delete should be idempotent, got %v", err)
	}
}

func TestFirestoreTesterAndSessionRoundTrip(t *testing.T) {
	f, ctx := newEmulatorStore(t)

	if _, err := f.GetTester(ctx, "a@b.com"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing tester err = %v, want ErrNotFound", err)
	}
	created := time.Now().UTC().Truncate(time.Microsecond)
	tester := Tester{Email: "a@b.com", InviteCode: "MOGI-1", ORKey: "sk-or-x", ORKeyHash: "hash-x", CreatedAt: created}
	if err := f.PutTester(ctx, tester); err != nil {
		t.Fatalf("put tester: %v", err)
	}
	got, err := f.GetTester(ctx, "a@b.com")
	if err != nil || got.Email != tester.Email || got.ORKey != tester.ORKey || got.ORKeyHash != tester.ORKeyHash || got.Revoked || !got.CreatedAt.Equal(created) {
		t.Fatalf("tester round trip = %+v (err %v)", got, err)
	}

	if _, err := f.GetSession(ctx, "tok-hash"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing session err = %v, want ErrNotFound", err)
	}
	if err := f.PutSession(ctx, Session{TokenHash: "tok-hash", Email: "a@b.com", CreatedAt: created}); err != nil {
		t.Fatalf("put session: %v", err)
	}
	sess, err := f.GetSession(ctx, "tok-hash")
	if err != nil || sess.Email != "a@b.com" || !sess.CreatedAt.Equal(created) {
		t.Fatalf("session round trip = %+v (err %v)", sess, err)
	}
}

func TestFirestoreConfig(t *testing.T) {
	f, ctx := newEmulatorStore(t)

	// Missing config/config is a deployment mistake — a loud error, not a
	// silent default (and effectively fail-closed for the kill switch).
	if _, err := f.GetConfig(ctx); err == nil || !strings.Contains(err.Error(), "config/config") {
		t.Fatalf("missing config err = %v, want an error naming config/config", err)
	}

	if _, err := f.client.Collection(colConfig).Doc(docConfig).Set(ctx, Config{TestPhaseActive: true, PinnedModel: "google/gemini-2.5-flash"}); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	cfg, err := f.GetConfig(ctx)
	if err != nil || !cfg.TestPhaseActive || cfg.PinnedModel != "google/gemini-2.5-flash" {
		t.Fatalf("config round trip = %+v (err %v)", cfg, err)
	}
}
