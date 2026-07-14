package openrouter

import (
	"context"
	"crypto/rand"
	"encoding/hex"
)

// StubMinter returns fake keys instead of calling OpenRouter. main.go wires it
// when OPENROUTER_PROVISIONING_KEY is unset, so the full activation flow runs
// locally without spending real credits. The fakes are shaped like real keys
// (distinct per mint) so downstream storage/serving code is exercised honestly.
type StubMinter struct{}

// NewStubMinter returns a StubMinter.
func NewStubMinter() *StubMinter { return &StubMinter{} }

var _ KeyMinter = (*StubMinter)(nil)

// Mint returns a unique fake key/hash pair. name and limitUSD are ignored.
func (StubMinter) Mint(_ context.Context, _ string, _ float64) (string, string, error) {
	suffix := randomHex(8)
	return "sk-or-fake-" + suffix, "fake-hash-" + suffix, nil
}

// randomHex returns 2n hex chars of crypto-random data.
func randomHex(n int) string {
	b := make([]byte, n)
	// crypto/rand.Read never returns an error on supported platforms; if it
	// somehow did, an empty suffix is harmless for a dev-only stub.
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
