// Package service holds the backend's business logic — the middle layer of the
// 3-layer backend. The thin Wails binding facade (package main) delegates here;
// services in turn talk to the data layer (internal/store) and the external API
// clients (internal/ai, internal/voice, internal/googletts) through narrow
// interfaces, so each service is unit-testable with fakes — no SQLite file, no
// OS hooks, no network.
package service

import (
	"context"
	"sync"

	"mogi/internal/ai"
	"mogi/internal/googletts"
	"mogi/internal/models"
	"mogi/internal/voice"
)

// AI is the OpenRouter surface the services consume. *ai.Client satisfies it;
// tests substitute a fake so no tokens are ever spent.
type AI interface {
	Complete(ctx context.Context, model string, messages []ai.ChatMessage) (string, error)
	ExtractSessionMeta(ctx context.Context, model, transcript, screenshotB64 string) (ai.SessionMeta, error)
	GenerateDebrief(ctx context.Context, model, transcript, finalCode string) (models.Debrief, error)
	ListModels(ctx context.Context) ([]models.Model, error)
}

// TTS is the minimal text-to-speech contract. Both *voice.Client (ElevenLabs)
// and *googletts.Client satisfy it, so the active provider is chosen at call
// time without branching on concrete types.
type TTS interface {
	Synthesize(ctx context.Context, voiceID, text string) ([]byte, error)
	ListVoices(ctx context.Context) ([]models.Voice, error)
}

// STT is the minimal speech-to-text contract. Both *voice.Client (ElevenLabs
// Scribe) and *googletts.Client (Google STT) satisfy it.
type STT interface {
	Transcribe(ctx context.Context, audio []byte, mimeType string) (string, error)
}

// Speech is a full voice provider: both synthesis and transcription. Each
// provider is self-sufficient — one key gives full voice support.
type Speech interface {
	TTS
	STT
}

// Providers is the registry of live API clients, keyed by the same provider
// names the key store uses ("openrouter", "elevenlabs", "google"). Settings
// swaps entries as the user edits keys; every other service fetches at call
// time, so "which client is live" is decided in exactly one place. The RWMutex
// makes the swap safe against concurrent readers — Wails dispatches each
// frontend call on its own goroutine.
type Providers struct {
	mu     sync.RWMutex
	ai     AI
	eleven Speech
	google Speech
}

// NewProviders returns an empty registry; NewApp loads persisted keys into it
// at startup and the Settings service swaps entries afterwards.
func NewProviders() *Providers { return &Providers{} }

// SetKey activates the client for a provider slot, or deactivates it when key
// is empty. Unknown provider names are ignored, matching the key store's
// permissive writes. Each arm assigns a literal nil on empty key — assigning a
// typed nil pointer (e.g. (*voice.Client)(nil)) would make the interface value
// non-nil and defeat every getter nil-check downstream.
func (p *Providers) SetKey(provider, key string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	switch provider {
	case "openrouter":
		if key == "" {
			p.ai = nil
		} else {
			p.ai = ai.NewClient(key)
		}
	case "elevenlabs":
		if key == "" {
			p.eleven = nil
		} else {
			p.eleven = voice.NewClient(key)
		}
	case "google":
		if key == "" {
			p.google = nil
		} else {
			p.google = googletts.NewClient(key)
		}
	}
}

// AI returns the live OpenRouter client, or nil when no key is configured.
func (p *Providers) AI() AI {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.ai
}

// ElevenLabs returns the live ElevenLabs voice client, or nil.
func (p *Providers) ElevenLabs() Speech {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.eleven
}

// Google returns the live Google Cloud voice client, or nil.
func (p *Providers) Google() Speech {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.google
}
