package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"mogi-access/internal/mailer"
	"mogi-access/internal/openrouter"
	"mogi-access/internal/store"
)

// recordingMailer captures the last OTP so tests can complete verification.
type recordingMailer struct {
	calls    int
	lastTo   string
	lastCode string
}

func (m *recordingMailer) SendOTP(_ context.Context, toEmail, code string) error {
	m.calls++
	m.lastTo = toEmail
	m.lastCode = code
	return nil
}

// fakeMinter returns a distinct key per call so "minted again" vs "reused" is
// observable via the call count.
type fakeMinter struct{ calls int }

func (f *fakeMinter) Mint(_ context.Context, name string, _ float64) (string, string, error) {
	f.calls++
	return fmt.Sprintf("sk-or-test-%d-%s", f.calls, name), fmt.Sprintf("hash-%d", f.calls), nil
}

var (
	_                    mailer.Mailer        = (*recordingMailer)(nil)
	_                    openrouter.KeyMinter = (*fakeMinter)(nil)
	pinnedModelForTest                        = "google/gemini-2.5-flash"
	googleKeyForTest                          = "google-shared"
	elevenLabsKeyForTest                      = "el-shared"
)

func activeConfig() store.Config {
	return store.Config{TestPhaseActive: true, PinnedModel: pinnedModelForTest}
}

func newTestServer(t *testing.T, cfg store.Config, devInvite string) (http.Handler, *store.Memory, *recordingMailer, *fakeMinter) {
	t.Helper()
	st := store.NewMemory(devInvite, cfg)
	m := &recordingMailer{}
	minter := &fakeMinter{}
	h := New(st, m, minter, Config{
		ORKeyLimitUSD: 3,
		GoogleKey:     googleKeyForTest,
		ElevenLabsKey: elevenLabsKeyForTest,
	})
	return h, st, m, minter
}

func doJSON(t *testing.T, h http.Handler, method, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		req = httptest.NewRequest(method, path, bytes.NewReader(b))
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func assertErrorBody(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("error response is not JSON: %v (body: %s)", err, w.Body.String())
	}
	if body["error"] == "" {
		t.Fatalf("error response missing non-empty 'error' field: %s", w.Body.String())
	}
}

func TestActivateInvalidInviteSendsNoMail(t *testing.T) {
	h, _, m, _ := newTestServer(t, activeConfig(), "MOGI-DEV")
	w := doJSON(t, h, "POST", "/activate", activateRequest{Email: "a@b.com", InviteCode: "WRONG"}, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	assertErrorBody(t, w)
	if m.calls != 0 {
		t.Fatalf("mailer called %d times, want 0 (no mail for an invalid invite)", m.calls)
	}
}

func TestActivateVerifyKeysRoundTrip(t *testing.T) {
	h, _, m, minter := newTestServer(t, activeConfig(), "MOGI-DEV")

	// Activate with a mixed-case email; the server normalizes it.
	w := doJSON(t, h, "POST", "/activate", activateRequest{Email: "User@B.com", InviteCode: "MOGI-DEV"}, nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("activate status = %d, want 204 (body: %s)", w.Code, w.Body.String())
	}
	if m.calls != 1 || m.lastCode == "" {
		t.Fatalf("mailer calls=%d code=%q, want 1 call with a code", m.calls, m.lastCode)
	}

	// Verify with the normalized email and captured code.
	w = doJSON(t, h, "POST", "/verify", verifyRequest{Email: "user@b.com", Code: m.lastCode}, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("verify status = %d, want 200 (body: %s)", w.Code, w.Body.String())
	}
	var vr verifyResponse
	if err := json.Unmarshal(w.Body.Bytes(), &vr); err != nil {
		t.Fatalf("decode verify: %v", err)
	}
	if vr.Token == "" {
		t.Fatal("verify returned an empty token")
	}
	if vr.Keys.OpenRouter == "" || vr.Keys.Google != googleKeyForTest ||
		vr.Keys.ElevenLabs != elevenLabsKeyForTest || vr.Keys.PinnedModel != pinnedModelForTest {
		t.Fatalf("verify keys = %+v, want minted OR key + shared voice keys + pinned model", vr.Keys)
	}
	if minter.calls != 1 {
		t.Fatalf("minter calls = %d, want 1", minter.calls)
	}

	// /keys with the token re-serves the identical set.
	w = doJSON(t, h, "GET", "/keys", nil, map[string]string{"Authorization": "Bearer " + vr.Token})
	if w.Code != http.StatusOK {
		t.Fatalf("keys status = %d, want 200 (body: %s)", w.Code, w.Body.String())
	}
	var kp keysPayload
	if err := json.Unmarshal(w.Body.Bytes(), &kp); err != nil {
		t.Fatalf("decode keys: %v", err)
	}
	if kp != vr.Keys {
		t.Fatalf("/keys payload %+v != /verify keys %+v", kp, vr.Keys)
	}
}

func TestVerifyExpiredCode(t *testing.T) {
	h, st, _, _ := newTestServer(t, activeConfig(), "MOGI-DEV")
	ctx := context.Background()
	email := "exp@b.com"
	// Seed an already-expired OTP for a known code.
	if err := st.PutOTP(ctx, store.OTP{
		EmailHash:  hashString(email),
		CodeHash:   hashString("123456"),
		InviteCode: "MOGI-DEV",
		ExpiresAt:  time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatalf("seed OTP: %v", err)
	}

	w := doJSON(t, h, "POST", "/verify", verifyRequest{Email: email, Code: "123456"}, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for an expired code", w.Code)
	}
	assertErrorBody(t, w)
	if _, err := st.GetOTP(ctx, hashString(email)); err == nil {
		t.Fatal("expired OTP was not deleted")
	}
}

func TestVerifyAttemptsExhausted(t *testing.T) {
	h, _, m, _ := newTestServer(t, activeConfig(), "MOGI-DEV")
	email := "tries@b.com"
	doJSON(t, h, "POST", "/activate", activateRequest{Email: email, InviteCode: "MOGI-DEV"}, nil)
	if m.lastCode == "" {
		t.Fatal("no OTP captured")
	}
	correct := m.lastCode

	for i := 0; i < maxOTPAttempts; i++ {
		w := doJSON(t, h, "POST", "/verify", verifyRequest{Email: email, Code: "wrong"}, nil)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("wrong attempt %d status = %d, want 400", i+1, w.Code)
		}
	}
	// The code is now discarded — even the correct value fails.
	w := doJSON(t, h, "POST", "/verify", verifyRequest{Email: email, Code: correct}, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("post-lockout correct code status = %d, want 400", w.Code)
	}
}

func TestInviteConsumedOncePerActivation(t *testing.T) {
	h, st, m, minter := newTestServer(t, activeConfig(), "MOGI-DEV")
	ctx := context.Background()
	email := "once@b.com"

	doJSON(t, h, "POST", "/activate", activateRequest{Email: email, InviteCode: "MOGI-DEV"}, nil)
	w := doJSON(t, h, "POST", "/verify", verifyRequest{Email: email, Code: m.lastCode}, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("first verify status = %d, want 200", w.Code)
	}
	inv, _ := st.GetInvite(ctx, "MOGI-DEV")
	if inv.Uses != 1 {
		t.Fatalf("invite uses = %d after one activation, want 1", inv.Uses)
	}

	// Re-activate and re-verify the SAME email: key is reused, no second use.
	doJSON(t, h, "POST", "/activate", activateRequest{Email: email, InviteCode: "MOGI-DEV"}, nil)
	w = doJSON(t, h, "POST", "/verify", verifyRequest{Email: email, Code: m.lastCode}, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("second verify status = %d, want 200", w.Code)
	}
	inv, _ = st.GetInvite(ctx, "MOGI-DEV")
	if inv.Uses != 1 {
		t.Fatalf("invite uses = %d after re-verify, want still 1 (reuse, not a new consume)", inv.Uses)
	}
	if minter.calls != 1 {
		t.Fatalf("minter calls = %d after re-verify, want still 1 (key reused)", minter.calls)
	}
}

func TestActivateRateLimitPerEmail(t *testing.T) {
	h, _, _, _ := newTestServer(t, activeConfig(), "MOGI-DEV")
	email := "flood@b.com"
	// The email limiter allows 3/hour.
	for i := 0; i < 3; i++ {
		w := doJSON(t, h, "POST", "/activate", activateRequest{Email: email, InviteCode: "MOGI-DEV"}, nil)
		if w.Code != http.StatusNoContent {
			t.Fatalf("activate %d status = %d, want 204", i+1, w.Code)
		}
	}
	w := doJSON(t, h, "POST", "/activate", activateRequest{Email: email, InviteCode: "MOGI-DEV"}, nil)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("4th activate status = %d, want 429", w.Code)
	}
	assertErrorBody(t, w)
}

func TestKeysInvalidToken(t *testing.T) {
	h, _, _, _ := newTestServer(t, activeConfig(), "MOGI-DEV")

	w := doJSON(t, h, "GET", "/keys", nil, nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("no-token status = %d, want 401", w.Code)
	}

	w = doJSON(t, h, "GET", "/keys", nil, map[string]string{"Authorization": "Bearer garbage"})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("garbage-token status = %d, want 401", w.Code)
	}
	assertErrorBody(t, w)
}

func TestKeysRevokedTester(t *testing.T) {
	h, st, m, _ := newTestServer(t, activeConfig(), "MOGI-DEV")
	ctx := context.Background()
	email := "revoked@b.com"

	doJSON(t, h, "POST", "/activate", activateRequest{Email: email, InviteCode: "MOGI-DEV"}, nil)
	w := doJSON(t, h, "POST", "/verify", verifyRequest{Email: email, Code: m.lastCode}, nil)
	var vr verifyResponse
	if err := json.Unmarshal(w.Body.Bytes(), &vr); err != nil {
		t.Fatalf("decode verify: %v", err)
	}

	tester, _ := st.GetTester(ctx, email)
	tester.Revoked = true
	if err := st.PutTester(ctx, tester); err != nil {
		t.Fatalf("revoke tester: %v", err)
	}

	w = doJSON(t, h, "GET", "/keys", nil, map[string]string{"Authorization": "Bearer " + vr.Token})
	if w.Code != http.StatusForbidden {
		t.Fatalf("revoked keys status = %d, want 403", w.Code)
	}
	assertErrorBody(t, w)
}

func TestActivatePhaseOff(t *testing.T) {
	st := store.NewMemory("MOGI-DEV", store.Config{TestPhaseActive: false, PinnedModel: pinnedModelForTest})
	h := New(st, &recordingMailer{}, &fakeMinter{}, Config{})
	w := doJSON(t, h, "POST", "/activate", activateRequest{Email: "x@b.com", InviteCode: "MOGI-DEV"}, nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("activate phase-off status = %d, want 403", w.Code)
	}
	assertErrorBody(t, w)
}

func TestKeysPhaseOff(t *testing.T) {
	st := store.NewMemory("MOGI-DEV", store.Config{TestPhaseActive: false, PinnedModel: pinnedModelForTest})
	h := New(st, &recordingMailer{}, &fakeMinter{}, Config{})
	ctx := context.Background()

	// Seed a valid tester + session directly, since activation is blocked while
	// the phase is off. This isolates the /keys kill switch.
	email := "off@b.com"
	if err := st.PutTester(ctx, store.Tester{Email: email, ORKey: "sk-or-x"}); err != nil {
		t.Fatalf("seed tester: %v", err)
	}
	token := "tok-abc"
	if err := st.PutSession(ctx, store.Session{TokenHash: hashString(token), Email: email}); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	w := doJSON(t, h, "GET", "/keys", nil, map[string]string{"Authorization": "Bearer " + token})
	if w.Code != http.StatusForbidden {
		t.Fatalf("keys phase-off status = %d, want 403", w.Code)
	}
	assertErrorBody(t, w)
}

// errStore wraps a real store and lets selected reads fail with an
// infrastructure error, standing in for a Firestore outage (the memory store
// itself cannot fail, so the infra-vs-NotFound distinction is only reachable
// through a wrapper). Everything else delegates.
type errStore struct {
	store.Store
	failGetSession error
	failGetTester  error
}

func (e *errStore) GetSession(ctx context.Context, tokenHash string) (store.Session, error) {
	if e.failGetSession != nil {
		return store.Session{}, e.failGetSession
	}
	return e.Store.GetSession(ctx, tokenHash)
}

func (e *errStore) GetTester(ctx context.Context, email string) (store.Tester, error) {
	if e.failGetTester != nil {
		return store.Tester{}, e.failGetTester
	}
	return e.Store.GetTester(ctx, email)
}

// TestKeysStoreErrorIs500NotSignOut pins the infra-vs-NotFound distinction on
// /keys: a store outage must surface as a 500 (the app keeps its cached keys),
// never a 401 (which the app treats as revoked and purges the account).
func TestKeysStoreErrorIs500NotSignOut(t *testing.T) {
	st := store.NewMemory("MOGI-DEV", activeConfig())
	es := &errStore{Store: st}
	m := &recordingMailer{}
	h := New(es, m, &fakeMinter{}, Config{})

	email := "blip@b.com"
	doJSON(t, h, "POST", "/activate", activateRequest{Email: email, InviteCode: "MOGI-DEV"}, nil)
	w := doJSON(t, h, "POST", "/verify", verifyRequest{Email: email, Code: m.lastCode}, nil)
	var vr verifyResponse
	if err := json.Unmarshal(w.Body.Bytes(), &vr); err != nil {
		t.Fatalf("decode verify: %v", err)
	}

	es.failGetSession = errors.New("rpc error: firestore unavailable")
	w = doJSON(t, h, "GET", "/keys", nil, map[string]string{"Authorization": "Bearer " + vr.Token})
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("store-outage status = %d, want 500 (a 401 would sign the tester out)", w.Code)
	}
	assertErrorBody(t, w)

	// The same token works again once the backend recovers — nothing was purged.
	es.failGetSession = nil
	w = doJSON(t, h, "GET", "/keys", nil, map[string]string{"Authorization": "Bearer " + vr.Token})
	if w.Code != http.StatusOK {
		t.Fatalf("post-outage status = %d, want 200 (same token still valid)", w.Code)
	}
}

// TestVerifyStoreErrorDoesNotMint pins the distinction on /verify's tester
// lookup: an outage must not be mistaken for "first-time tester", which would
// consume an invite use and mint (then overwrite) an OpenRouter key.
func TestVerifyStoreErrorDoesNotMint(t *testing.T) {
	st := store.NewMemory("MOGI-DEV", activeConfig())
	es := &errStore{Store: st}
	m := &recordingMailer{}
	minter := &fakeMinter{}
	h := New(es, m, minter, Config{})

	email := "outage@b.com"
	doJSON(t, h, "POST", "/activate", activateRequest{Email: email, InviteCode: "MOGI-DEV"}, nil)

	es.failGetTester = errors.New("rpc error: firestore unavailable")
	w := doJSON(t, h, "POST", "/verify", verifyRequest{Email: email, Code: m.lastCode}, nil)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("verify-during-outage status = %d, want 500", w.Code)
	}
	if minter.calls != 0 {
		t.Fatalf("minter called %d times during the outage, want 0", minter.calls)
	}
	inv, err := st.GetInvite(context.Background(), "MOGI-DEV")
	if err != nil || inv.Uses != 0 {
		t.Fatalf("invite uses = %d (err %v), want 0 — an outage must not burn an invite use", inv.Uses, err)
	}
}
