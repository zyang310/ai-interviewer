package store

import (
	"context"
	"errors"
	"fmt"

	"cloud.google.com/go/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Collection / document names. Consts so the seeding steps in the deploy docs
// (3.4) and any future admin tooling reference the same strings.
const (
	colInvites  = "invites"
	colOTPs     = "otps"
	colTesters  = "testers"
	colSessions = "sessions"
	colConfig   = "config"
	docConfig   = "config"
)

// Firestore is the production Store: one collection per record type, documents
// keyed by the same natural keys the memory maps use (invite code, email hash,
// email, token hash), and the service-wide Config in a single console-editable
// config/config doc. Structs are stored with their Go field names (no tags),
// so what the Firestore console shows matches store.go one-to-one.
type Firestore struct {
	client *firestore.Client
}

// NewFirestore connects to the project's (default) Firestore database using
// Application Default Credentials — the service account on Cloud Run, the
// gcloud ADC locally, or the emulator when FIRESTORE_EMULATOR_HOST is set.
// The client lives for the process lifetime; the Store interface deliberately
// has no teardown, so there is no Close (Cloud Run reaps it with the process).
func NewFirestore(ctx context.Context, projectID string) (*Firestore, error) {
	client, err := firestore.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("store: create firestore client: %w", err)
	}
	return &Firestore{client: client}, nil
}

// compile-time proof that Firestore satisfies the Store contract.
var _ Store = (*Firestore)(nil)

// getDoc reads one document into out, mapping Firestore's NotFound onto the
// package's ErrNotFound sentinel so handlers keep their existing error checks.
// Errors name only the collection, not the doc id (tester ids are emails —
// keep them out of error strings that get logged).
func (f *Firestore) getDoc(ctx context.Context, col, id string, out any) error {
	snap, err := f.client.Collection(col).Doc(id).Get(ctx)
	if status.Code(err) == codes.NotFound {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("store: get %s doc: %w", col, err)
	}
	if err := snap.DataTo(out); err != nil {
		return fmt.Errorf("store: decode %s doc: %w", col, err)
	}
	return nil
}

// putDoc upserts one document (Set == full overwrite, matching the memory
// maps' assignment semantics).
func (f *Firestore) putDoc(ctx context.Context, col, id string, v any) error {
	if _, err := f.client.Collection(col).Doc(id).Set(ctx, v); err != nil {
		return fmt.Errorf("store: put %s doc: %w", col, err)
	}
	return nil
}

func (f *Firestore) GetInvite(ctx context.Context, code string) (Invite, error) {
	var inv Invite
	err := f.getDoc(ctx, colInvites, code, &inv)
	return inv, err
}

// ConsumeInvite runs the guarded increment in a Firestore transaction, so a
// race to the last use cannot over-consume — the same guarantee the memory
// store gets from its coarse mutex. The sentinel errors abort the transaction
// and surface unchanged (RunTransaction retries only backend contention).
func (f *Firestore) ConsumeInvite(ctx context.Context, code string) error {
	ref := f.client.Collection(colInvites).Doc(code)
	err := f.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		snap, err := tx.Get(ref)
		if status.Code(err) == codes.NotFound {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		var inv Invite
		if err := snap.DataTo(&inv); err != nil {
			return err
		}
		if !inv.Active || inv.Uses >= inv.MaxUses {
			return ErrInviteUnavailable
		}
		return tx.Update(ref, []firestore.Update{{Path: "Uses", Value: inv.Uses + 1}})
	})
	if err != nil && !errors.Is(err, ErrNotFound) && !errors.Is(err, ErrInviteUnavailable) {
		return fmt.Errorf("store: consume invite: %w", err)
	}
	return err
}

func (f *Firestore) PutOTP(ctx context.Context, o OTP) error {
	return f.putDoc(ctx, colOTPs, o.EmailHash, o)
}

func (f *Firestore) GetOTP(ctx context.Context, emailHash string) (OTP, error) {
	var o OTP
	err := f.getDoc(ctx, colOTPs, emailHash, &o)
	return o, err
}

func (f *Firestore) DeleteOTP(ctx context.Context, emailHash string) error {
	// Delete is idempotent (no error for a missing doc), matching the memory
	// store's map delete.
	if _, err := f.client.Collection(colOTPs).Doc(emailHash).Delete(ctx); err != nil {
		return fmt.Errorf("store: delete %s doc: %w", colOTPs, err)
	}
	return nil
}

func (f *Firestore) GetTester(ctx context.Context, email string) (Tester, error) {
	var t Tester
	err := f.getDoc(ctx, colTesters, email, &t)
	return t, err
}

func (f *Firestore) PutTester(ctx context.Context, t Tester) error {
	return f.putDoc(ctx, colTesters, t.Email, t)
}

func (f *Firestore) PutSession(ctx context.Context, s Session) error {
	return f.putDoc(ctx, colSessions, s.TokenHash, s)
}

func (f *Firestore) GetSession(ctx context.Context, tokenHash string) (Session, error) {
	var s Session
	err := f.getDoc(ctx, colSessions, tokenHash, &s)
	return s, err
}

// GetConfig reads the console-editable config/config doc. A missing doc is a
// deployment mistake, not a normal state — fail loudly (and effectively
// closed: handlers 500 rather than treating the phase as active) instead of
// inventing defaults.
func (f *Firestore) GetConfig(ctx context.Context) (Config, error) {
	var c Config
	err := f.getDoc(ctx, colConfig, docConfig, &c)
	if errors.Is(err, ErrNotFound) {
		return Config{}, fmt.Errorf("store: %s/%s doc missing — seed it before serving (see README ops notes)", colConfig, docConfig)
	}
	return c, err
}
