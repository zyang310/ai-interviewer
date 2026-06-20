# CLAUDE.md — Mock Interview Desktop App

## Project overview

This is a desktop application that acts as a live AI-powered mock coding interview coach. The user codes in their own IDE while the app captures their screen, listens to their voice, and provides real-time interviewer feedback through an AI assistant. The AI behaves like a real technical interviewer — Socratic, nudging, never giving away answers.

## Tech stack

* Framework: Wails v2 — Go backend + web frontend in a single native binary, uses OS webview (no Chromium)
* Backend: Go — handles screen capture, all external API calls (OpenRouter, ElevenLabs), local storage, and system-level operations
* Frontend: React + TypeScript + Vite — UI for the interviewer chat, problem display, audio playback, settings, and session history
* AI gateway: OpenRouter (https://openrouter.ai) — unified API for Claude, GPT-4, Gemini, etc. with OAuth PKCE for user auth
* Voice I/O: ElevenLabs API via Go backend — TTS (text-to-speech) for interviewer voice using Flash v2.5 (~75ms latency, streaming), STT (speech-to-text) using Scribe v2 for transcribing user speech. All voice processing routes through Go so API keys never touch the frontend.
* Screen capture: Go-native using `kbinani/screenshot` — periodic screenshots sent as base64 to the vision API
* Local storage: SQLite via `mattn/go-sqlite3` — session history, user preferences, problem bank

## Architecture

```
┌───────────────────────────────────────────────────────┐
│  Wails Desktop App (single binary)                    │
│                                                        │
│  ┌──────────────────────┐   ┌──────────────────────┐  │
│  │   Go Backend          │   │  React/TS Frontend   │  │
│  │                       │   │                      │  │
│  │  - Screen capture     │◄──┤  - Chat UI           │  │
│  │  - OpenRouter API     │──►│  - Problem panel     │  │
│  │  - ElevenLabs TTS/STT │   │  - Audio playback    │  │
│  │  - SQLite store       │   │  - Mic recording     │  │
│  │  - Session mgmt       │   │  - Settings/auth     │  │
│  └──────────┬────────────┘   └──────────────────────┘  │
│             │                         │                 │
│             ▼                         ▼                 │
│        OS native APIs          OS native webview        │
└───────────────────────────────────────────────────────┘
              │
              ├──► OpenRouter API ──► Claude / GPT-4 / Gemini
              │
              └──► ElevenLabs API ──► TTS (Flash v2.5) / STT (Scribe v2)
```

All external API calls (AI inference and voice) are centralized in the Go backend. The frontend handles UI rendering, mic recording (raw audio capture via MediaRecorder API), and audio playback only. API keys and tokens never touch the frontend layer.

## Data flow (core interview loop)

1. User codes in their own IDE (VS Code, IntelliJ, terminal, etc.)
2. Go backend captures screen every N seconds via `kbinani/screenshot`
3. User speaks (push-to-talk) — frontend records raw audio via MediaRecorder API, sends audio blob to Go backend
4. Go backend sends audio to ElevenLabs Scribe v2 for transcription → receives text
5. Go backend bundles: transcribed text + latest screenshot (base64) + conversation history + problem context
6. Sends to OpenRouter API with the interviewer system prompt
7. AI response text streams back → Go sends text to ElevenLabs TTS (Flash v2.5, streaming) → audio chunks stream to frontend
8. Frontend renders text in chat panel and plays audio chunks via Web Audio API concurrently
9. Session (transcript + timestamps) logged to SQLite

## Project structure

```
mock-interviewer/
├── CLAUDE.md
├── wails.json                  # Wails project config
├── main.go                     # App entry point
├── app.go                      # Core app struct, bound methods
├── internal/
│   ├── capture/
│   │   └── screen.go           # Screen capture logic (kbinani/screenshot)
│   ├── ai/
│   │   ├── client.go           # OpenRouter API client
│   │   ├── models.go           # Request/response types
│   │   └── prompts.go          # System prompts for interviewer persona
│   ├── voice/
│   │   ├── tts.go              # ElevenLabs TTS — text to streaming audio (Flash v2.5)
│   │   ├── stt.go              # ElevenLabs Scribe v2 — audio to text transcription
│   │   ├── voices.go           # Voice listing and selection
│   │   └── models.go           # ElevenLabs request/response types
│   ├── auth/
│   │   ├── openrouter.go       # OAuth PKCE flow for OpenRouter
│   │   └── keys.go             # Encrypted API key storage (ElevenLabs key)
│   ├── store/
│   │   ├── db.go               # SQLite initialization and migrations
│   │   ├── sessions.go         # Session CRUD
│   │   ├── problems.go         # Problem bank queries
│   │   └── preferences.go     # User settings
│   └── models/
│       ├── session.go          # Session, Message structs
│       └── problem.go          # Problem struct
├── frontend/
│   ├── src/
│   │   ├── App.tsx
│   │   ├── main.tsx
│   │   ├── components/
│   │   │   ├── Chat.tsx            # Interviewer chat panel
│   │   │   ├── MessageBubble.tsx   # Individual message
│   │   │   ├── ProblemPanel.tsx    # Problem description display
│   │   │   ├── VoiceControls.tsx   # Mic toggle, voice status indicator
│   │   │   ├── ModelPicker.tsx     # Model selection dropdown
│   │   │   ├── VoicePicker.tsx     # ElevenLabs voice selection
│   │   │   ├── SessionHistory.tsx  # Past interview sessions
│   │   │   └── Settings.tsx        # Preferences, auth, capture interval
│   │   ├── hooks/
│   │   │   ├── useAudioRecorder.ts   # MediaRecorder wrapper — captures mic audio as blobs
│   │   │   ├── useAudioPlayback.ts   # Web Audio API — plays streaming audio from Go backend
│   │   │   └── useInterviewSession.ts
│   │   ├── lib/
│   │   │   └── wailsBridge.ts  # Typed wrappers around bound Go methods
│   │   └── styles/
│   │       └── globals.css
│   ├── index.html
│   ├── tsconfig.json
│   ├── vite.config.ts
│   └── package.json
├── problems/
│   └── seed.json               # Default problem bank (JSON seed data)
└── build/
    └── ...                     # Wails build output
```

## Key Go bindings (exposed to frontend)

These Go methods are callable from TypeScript as async functions. Wails auto-generates TS types from the Go structs.

```go
// app.go — methods bound to frontend

// Auth
func (a *App) StartOAuthFlow() (string, error)       // Returns OpenRouter auth URL to open in browser
func (a *App) CompleteOAuth(code string) error         // Exchanges code for token
func (a *App) GetAuthStatus() AuthStatus               // Check if user is authenticated
func (a *App) SetAPIKey(provider, key string) error    // Manual key entry fallback (supports "openrouter" and "elevenlabs")

// Interview
func (a *App) SendMessage(text string) (string, error) // Send user message + screenshot to OpenRouter, returns AI text response
func (a *App) StartSession(problemID string, model string) (Session, error)
func (a *App) EndSession(sessionID string) (SessionSummary, error)

// Voice — ElevenLabs (all processing happens in Go, frontend only records/plays audio)
func (a *App) TranscribeAudio(audioBase64 string) (string, error) // Send recorded audio → ElevenLabs Scribe v2 → returns text
func (a *App) SynthesizeSpeech(text string) (string, error)       // Send text → ElevenLabs TTS Flash v2.5 → returns audio base64
func (a *App) ListVoices() ([]Voice, error)                        // Fetch available ElevenLabs voices
func (a *App) SetVoice(voiceID string) error                       // Set preferred interviewer voice

// Screen capture
func (a *App) StartCapture(intervalMs int) error       // Begin periodic screen capture
func (a *App) StopCapture() error
func (a *App) GetLatestScreenshot() (string, error)    // Base64 encoded PNG

// Problems
func (a *App) ListProblems() ([]Problem, error)
func (a *App) GetProblem(id string) (Problem, error)

// Sessions
func (a *App) ListSessions() ([]SessionSummary, error)
func (a *App) GetSessionTranscript(id string) ([]Message, error)

// Settings
func (a *App) GetPreferences() (Preferences, error)
func (a *App) UpdatePreferences(prefs Preferences) error
func (a *App) ListAvailableModels() ([]Model, error)   // Fetch from OpenRouter
```

## AI interviewer system prompt (core behavior)

The system prompt is the most important tunable. The interviewer must:

* Never give away the answer or key insight
* Use Socratic questioning: "What data structure could help you look things up in O(1)?"
* React to the code it sees in the screenshot without volunteering solutions
* Keep responses short (1-3 sentences) like a real interviewer
* Ask about time/space complexity when the candidate proposes an approach
* Probe edge cases: "What happens if the input is empty?"
* Only respond when spoken to — don't interrupt unprompted
* Match the tone of a senior engineer, not a cheerful chatbot

The system prompt receives the problem description and the latest screenshot on every message. The conversation history is included for continuity.

## Coding conventions

### Go

* Standard `gofmt` formatting
* Error handling: return errors, don't panic. Wrap with `fmt.Errorf("context: %w", err)`
* Structs that cross the Wails boundary need `json:"fieldName"` tags
* Use `context.Context` for cancellable operations (API calls, screen capture loops)
* Keep `app.go` thin — delegate to `internal/` packages

### TypeScript / React

* Functional components with hooks only
* State management: React state + context (no Redux). Session state lives in `useInterviewSession` hook
* Styling: CSS modules or Tailwind (decide during setup, but pick one)
* All Wails-bound Go calls go through `lib/wailsBridge.ts` for a single import point
* Handle loading/error states for every async Go call

### General

* Commit messages: conventional commits (`feat:`, `fix:`, `chore:`, `docs:`)
* No hardcoded API keys anywhere — always runtime config or OAuth tokens
* Screen capture runs on a configurable interval (default 3 seconds)
* All user data stays local (SQLite). No telemetry, no cloud sync.

## Development workflow

```bash
# Scaffold the project
wails init -n mock-interviewer -t react-ts

# Dev mode (hot reload frontend + Go rebuild)
wails dev

# Build production binary
wails build

# Run frontend only (for UI work)
cd frontend && npm run dev
```

## Implementation phases

### Phase 1 — Core loop (MVP)

* [ ] Wails project scaffold with React-TS template
* [ ] Manual API key input for OpenRouter (skip OAuth for now)
* [ ] Single hardcoded problem (Two Sum)
* [ ] Screen capture on a timer → base64 encoding
* [ ] Chat UI: send typed message + screenshot to OpenRouter → display response
* [ ] Interviewer system prompt tuned for Socratic behavior
* [ ] Basic SQLite schema: sessions and messages tables

### Phase 2 — Voice integration (ElevenLabs)

* [ ] Manual API key input for ElevenLabs
* [ ] Frontend mic recording via MediaRecorder API (push-to-talk, outputs audio blob)
* [ ] Go backend: receive audio blob → send to ElevenLabs Scribe v2 → return transcribed text
* [ ] Go backend: receive AI response text → send to ElevenLabs TTS Flash v2.5 (streaming) → return audio
* [ ] Frontend audio playback via Web Audio API (play streamed chunks as they arrive)
* [ ] Voice selection UI (fetch and display available ElevenLabs voices)
* [ ] Visual indicators: recording state, AI speaking state, transcription in progress

### Phase 3 — Problem bank and UX

* [ ] Problem bank with multiple problems seeded from JSON
* [ ] Problem selector UI with difficulty tags
* [ ] Model picker (fetch available models from OpenRouter)
* [ ] Session history list with transcript review
* [ ] Settings panel (capture interval, voice selection, model preference, key management)
* [ ] Keyboard shortcuts (push-to-talk, end session, toggle capture)

### Phase 4 — Auth and polish

* [ ] OpenRouter OAuth PKCE flow (login button opens browser, callback completes auth)
* [ ] Token and key persistence in SQLite (encrypted)
* [ ] Always-on-top floating window mode
* [ ] Post-interview debrief mode (AI drops interviewer persona, gives direct feedback)
* [ ] Session export (markdown transcript with timestamps)

### Phase 5 — Stretch goals

* [ ] Difficulty adaptation (AI adjusts hint level based on progress)
* [ ] Timer / time pressure mode
* [ ] Multi-problem interview sets (simulate a full interview round)
* [ ] Custom problem import (paste a LeetCode URL, scrape description)
* [ ] ElevenLabs voice cloning (user uploads interviewer voice sample)

## Key dependencies

### Go

```
github.com/wailsapp/wails/v2              # Wails framework
github.com/kbinani/screenshot              # Cross-platform screen capture
github.com/mattn/go-sqlite3                # SQLite driver
github.com/dhia-gharsallaoui/go-elevenlabs # ElevenLabs API client (TTS + STT, streaming, zero dependencies)
```

### Frontend (npm)

```
react, react-dom                 # UI framework
typescript                       # Type safety
vite                             # Build tool (included in Wails template)
```

## ElevenLabs API reference

Two endpoints are used. Both are called from the Go backend only.

### Text-to-Speech (streaming)

`POST https://api.elevenlabs.io/v1/text-to-speech/{voice_id}/stream`

Returns raw audio bytes (MP3) via chunked transfer encoding. Frontend plays chunks as they arrive for minimal perceived latency. Use Flash v2.5 model for lowest latency (~75ms).

```json
{
  "text": "Have you considered what happens with an empty array?",
  "model_id": "eleven_flash_v2_5",
  "voice_settings": {
    "stability": 0.5,
    "similarity_boost": 0.75
  }
}
```

Auth: `xi-api-key` header with ElevenLabs API key.

### Speech-to-Text (Scribe v2)

`POST https://api.elevenlabs.io/v1/speech-to-text`

Accepts audio file upload (WAV, MP3, WebM), returns transcribed text. Frontend records audio via MediaRecorder API as WebM/opus, sends base64 blob to Go, Go forwards to Scribe.

```
Content-Type: multipart/form-data
- file: audio blob
- model_id: "scribe_v2"
```

### Cost model

TTS is billed per character of input text. STT is billed per minute of audio. For a typical interview session (30-60 minutes, short interviewer responses of 1-3 sentences each), costs are minimal. Keep responses short to optimize both latency and cost.

## OpenRouter API reference

Base URL: `https://openrouter.ai/api/v1/chat/completions`

Request format follows the OpenAI chat completions spec. Vision messages include image_url with base64 data URIs. Auth is via Bearer token (from OAuth or manual key entry).

```json
{
  "model": "anthropic/claude-sonnet-4",
  "messages": [
    { "role": "system", "content": "You are a technical interviewer..." },
    {
      "role": "user",
      "content": [
        { "type": "text", "text": "I think I should use a hashmap here" },
        {
          "type": "image_url",
          "image_url": { "url": "data:image/png;base64,..." }
        }
      ]
    }
  ]
}
```

## Notes

* The app window should support an always-on-top mode so it floats over the user's IDE
* Screen capture should exclude the app's own window if possible to avoid recursive capture
* Voice input should have a clear visual indicator (recording state, transcribing state) so the user knows when they're being heard
* The AI should not speak unless the user has spoken or typed first — no unprompted interruptions
* Keep latency low: screenshot compression, conversation history trimming (last 10 exchanges), streaming TTS playback, and streaming AI responses all matter for the interview feel
* All API keys (OpenRouter token, ElevenLabs key) are stored and used exclusively in the Go backend — the frontend never sees them
* The frontend's only role in voice is: (1) recording raw audio from the mic via MediaRecorder API, (2) playing back audio bytes from Go via Web Audio API. All processing and API calls happen in Go.
* ElevenLabs TTS audio should start playing as soon as the first chunks arrive, not after the full response is synthesized — use Wails runtime events to push audio chunks from Go to the frontend incrementally
* For typed messages (no voice), skip both ElevenLabs endpoints — send text directly to OpenRouter and display the response as text only, with an optional "read aloud" button
