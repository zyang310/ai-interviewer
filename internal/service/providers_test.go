package service

import "testing"

// TestProvidersSetKey guards the typed-nil interface trap: clearing a slot must
// yield a getter result that compares equal to nil, not a non-nil interface
// value wrapping a nil concrete client.
func TestProvidersSetKey(t *testing.T) {
	p := NewProviders()

	if p.AI() != nil || p.ElevenLabs() != nil || p.Google() != nil {
		t.Fatal("new registry should have no live clients")
	}

	p.SetKey("openrouter", "k")
	if p.AI() == nil {
		t.Error("openrouter: expected live client after SetKey")
	}
	p.SetKey("openrouter", "")
	if p.AI() != nil {
		t.Error("openrouter: expected nil after clearing key")
	}

	p.SetKey("elevenlabs", "k")
	if p.ElevenLabs() == nil {
		t.Error("elevenlabs: expected live client after SetKey")
	}
	p.SetKey("elevenlabs", "")
	if p.ElevenLabs() != nil {
		t.Error("elevenlabs: expected nil after clearing key")
	}

	p.SetKey("google", "k")
	if p.Google() == nil {
		t.Error("google: expected live client after SetKey")
	}
	p.SetKey("google", "")
	if p.Google() != nil {
		t.Error("google: expected nil after clearing key")
	}
}

// TestProvidersSetKeyUnknownProvider verifies unknown names are ignored rather
// than panicking, matching the key store's permissive writes.
func TestProvidersSetKeyUnknownProvider(t *testing.T) {
	p := NewProviders()
	p.SetKey("mystery", "k") // must not panic
	if p.AI() != nil || p.ElevenLabs() != nil || p.Google() != nil {
		t.Fatal("unknown provider must not populate any slot")
	}
}
