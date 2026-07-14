// Package store is the persistence seam for the access service. The handlers
// depend only on the Store interface, so the in-memory implementation (local
// dev and unit tests) and the Firestore implementation (production, added in
// Phase 3) are interchangeable with no handler changes.
package store

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is returned by getters when the requested record is absent, so
// callers can distinguish "missing" from a real backend error.
var ErrNotFound = errors.New("store: not found")

// ErrInviteUnavailable is returned by ConsumeInvite when the invite exists but
// is disabled or has no uses left. Distinct from ErrNotFound so a race to the
// last use is diagnosable, though handlers map both to the same client 4xx.
var ErrInviteUnavailable = errors.New("store: invite unavailable")

// Invite is a shared redemption code. One code admits up to MaxUses testers;
// disabling it (Active=false) cuts off everyone who would redeem it next.
type Invite struct {
	Code    string
	MaxUses int
	Uses    int
	Active  bool
}

// OTP is a pending one-time code for an email. It is keyed and looked up by a
// hash of the email (the raw address is not needed until a tester is created),
// and the code itself is stored hashed. Short expiry + the attempt cap, not
// hash strength, are what protect the small 6-digit space. InviteCode is
// carried here so verification can commit the invite use without the client
// re-sending the code it already proved at /activate.
type OTP struct {
	EmailHash  string
	CodeHash   string
	InviteCode string
	ExpiresAt  time.Time
	Attempts   int
}

// Tester is an activated account. It stores the raw email (for attribution,
// key naming, and support) and the raw provisioned OpenRouter key, because
// OpenRouter returns the usable key only once at mint time — /keys must be able
// to re-serve it on every launch refresh. ORKeyHash is kept for revocation.
type Tester struct {
	Email      string
	InviteCode string
	ORKeyHash  string
	ORKey      string
	CreatedAt  time.Time
	Revoked    bool
}

// Session ties a bearer token (stored hashed) to a tester's email. The app
// presents the token on GET /keys; the raw token lives only on the device.
type Session struct {
	TokenHash string
	Email     string
	CreatedAt time.Time
}

// Config is the service-wide runtime config. In production it is a single
// Firestore doc so the kill switch and model pin are editable without a
// redeploy; in memory it is seeded once at startup.
type Config struct {
	TestPhaseActive bool
	PinnedModel     string
}

// Store is the minimal persistence surface the three handlers need. Every
// method takes a context so the Firestore implementation can honor request
// deadlines; the in-memory implementation ignores it.
type Store interface {
	// GetInvite reads an invite for the pre-mail validation in /activate.
	GetInvite(ctx context.Context, code string) (Invite, error)
	// ConsumeInvite atomically increments Uses, guarded by Active && Uses <
	// MaxUses. It is called once on successful verification, so the pre-check
	// in /activate never over-counts. Returns ErrInviteUnavailable if the last
	// use was taken between the pre-check and here.
	ConsumeInvite(ctx context.Context, code string) error

	// PutOTP upserts the pending OTP for an email hash (a re-request or a failed
	// attempt overwrites the prior record).
	PutOTP(ctx context.Context, o OTP) error
	GetOTP(ctx context.Context, emailHash string) (OTP, error)
	DeleteOTP(ctx context.Context, emailHash string) error

	// GetTester returns ErrNotFound for a first-time verifier; an existing
	// (non-revoked) tester's key is reused so re-verifying never mints twice.
	GetTester(ctx context.Context, email string) (Tester, error)
	PutTester(ctx context.Context, t Tester) error

	PutSession(ctx context.Context, s Session) error
	GetSession(ctx context.Context, tokenHash string) (Session, error)

	GetConfig(ctx context.Context) (Config, error)
}
