package store

import (
	"context"
	"sync"
)

// devInviteMaxUses is the seeded local invite's cap — generous so a dev never
// exhausts it while testing the flow repeatedly.
const devInviteMaxUses = 100

// Memory is an in-process Store for local development and unit tests. A single
// mutex guards every map; the access service is low-QPS, so lock granularity
// does not matter and coarse locking keeps ConsumeInvite trivially atomic.
type Memory struct {
	mu       sync.Mutex
	invites  map[string]Invite  // keyed by Code
	otps     map[string]OTP     // keyed by EmailHash
	testers  map[string]Tester  // keyed by Email
	sessions map[string]Session // keyed by TokenHash
	cfg      Config
}

// NewMemory returns a Store seeded with a single high-use invite (so the local
// flow works out of the box) and the given config. An empty devInviteCode
// seeds no invite.
func NewMemory(devInviteCode string, cfg Config) *Memory {
	m := &Memory{
		invites:  make(map[string]Invite),
		otps:     make(map[string]OTP),
		testers:  make(map[string]Tester),
		sessions: make(map[string]Session),
		cfg:      cfg,
	}
	if devInviteCode != "" {
		m.invites[devInviteCode] = Invite{
			Code:    devInviteCode,
			MaxUses: devInviteMaxUses,
			Active:  true,
		}
	}
	return m
}

// compile-time proof that Memory satisfies the Store contract.
var _ Store = (*Memory)(nil)

func (m *Memory) GetInvite(_ context.Context, code string) (Invite, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	inv, ok := m.invites[code]
	if !ok {
		return Invite{}, ErrNotFound
	}
	return inv, nil
}

func (m *Memory) ConsumeInvite(_ context.Context, code string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	inv, ok := m.invites[code]
	if !ok {
		return ErrNotFound
	}
	if !inv.Active || inv.Uses >= inv.MaxUses {
		return ErrInviteUnavailable
	}
	inv.Uses++
	m.invites[code] = inv
	return nil
}

func (m *Memory) PutOTP(_ context.Context, o OTP) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.otps[o.EmailHash] = o
	return nil
}

func (m *Memory) GetOTP(_ context.Context, emailHash string) (OTP, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	o, ok := m.otps[emailHash]
	if !ok {
		return OTP{}, ErrNotFound
	}
	return o, nil
}

func (m *Memory) DeleteOTP(_ context.Context, emailHash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.otps, emailHash)
	return nil
}

func (m *Memory) GetTester(_ context.Context, email string) (Tester, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.testers[email]
	if !ok {
		return Tester{}, ErrNotFound
	}
	return t, nil
}

func (m *Memory) PutTester(_ context.Context, t Tester) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.testers[t.Email] = t
	return nil
}

func (m *Memory) PutSession(_ context.Context, s Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[s.TokenHash] = s
	return nil
}

func (m *Memory) GetSession(_ context.Context, tokenHash string) (Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[tokenHash]
	if !ok {
		return Session{}, ErrNotFound
	}
	return s, nil
}

func (m *Memory) GetConfig(_ context.Context) (Config, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cfg, nil
}
