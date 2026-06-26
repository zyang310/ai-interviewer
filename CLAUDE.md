# CLAUDE.md — AI Mock Interviewer

Rules and a map of the codebase. Roadmap/status → [docs/roadmap.md](docs/roadmap.md). Deep reference (data flow, full bindings, prompt spec, API contracts) → [docs/architecture.md](docs/architecture.md).

## Purpose (why this exists)

A desktop app that runs a **live AI mock coding interview**. The user codes in their own IDE or a browser tab (VS Code, IntelliJ, terminal, LeetCode, NeetCode); the app screenshots their screen and an AI interviewer reads the problem *and* the code from the screenshot, then nudges Socratically — never handing over the answer.

**Screen-driven:** there is no problem bank and no written problem statement. The screenshot is the problem.

## Stack (what it's built with)

- **Wails v2** — Go backend + web frontend in one native binary, OS webview (no Chromium). Window runs **frameless + transparent** so the overlay can float over the user's IDE.
- **Go backend** — screen capture, all external API calls, SQLite, window/overlay control.
- **Frontend** — React + TypeScript + Vite. **Styling is plain CSS with Material Design 3 tokens (CSS variables) — no Tailwind.**
- **AI** — OpenRouter (vision models) via the Go backend.
- **Voice** — via the Go backend, **non-streaming** v1. Both STT and TTS resolve a provider behind app.go's `sttProvider`/`ttsProvider` interfaces — **Google** (default voice, low-cost) and **ElevenLabs** (Scribe STT + Flash TTS, premium). Each provider is self-sufficient (one key = full voice); the Settings toggle is **voice-only** (STT auto-prefers Scribe when its key is present, else Google), so with both keys the default is the optimal combo: Scribe STT + Google TTS. The mic records via `MediaRecorder` and is re-encoded to 16 kHz mono WAV client-side (`audioToWav`, since Google STT can't ingest WKWebView's AAC); all API calls happen in Go. Playback speed is applied client-side via `playbackRate`. The mic has two triggers: the click-to-toggle button, and **global push-to-talk** (below).
- **Voice hotkey** — a configurable global hotkey (default `Right ⌥ Option` — a bare modifier that avoids the macOS unhandled-key beep a combo would cause; Settings → Voice Hotkey). A backend OS-level keyboard hook (`internal/hotkey`, via `robotn/gohook`) fires a Wails `ptt:down` event per press; the frontend **toggles** recording on it — press to start, press again to stop & send — through the same recorder path as the mic button. Works while the IDE (not this window) is focused, cross-platform. The hook is **passive** (the key still reaches the IDE) and on **macOS requires Input Monitoring permission** (surfaced in Settings). Defaults to **enabled**.
- **Deps** — `kbinani/screenshot` + `golang.org/x/image` (capture), `mattn/go-sqlite3` (storage), `google/uuid`, `robotn/gohook` (global hotkey).

## Codebase map (where things live)

| Path | What / why |
|---|---|
| `main.go` | Entry point; Wails window options (frameless + transparent). |
| `app.go` | **All** Wails-bound methods. Kept thin — delegates to `internal/`. |
| `internal/ai/` | OpenRouter client (`client.go`, incl. `ExtractProblemMeta` for history labels) + prompts (`prompts.go`: interviewer system prompt + `ProblemMetaPrompt`). |
| `internal/voice/` | ElevenLabs client (`client.go`): Scribe STT, Flash TTS, voice catalog. |
| `internal/googletts/` | Google Cloud client (`client.go`): TTS (synthesize + English voice catalog) **and** STT (`Transcribe`). Satisfies the same `Synthesize`/`ListVoices`/`Transcribe` shapes as `internal/voice`. |
| `internal/capture/` | Screen capture + region cropping. |
| `internal/hotkey/` | Global voice-hotkey keyboard hook (`listener.go`, via `robotn/gohook`) + hotkey spec↔keycode↔label mapping (`keymap.go`). Emits a Wails `ptt:down` event per press (frontend toggles recording on it); passive (doesn't swallow the key). |
| `internal/store/` | SQLite (`data.db`): sessions + messages (transcripts), preferences, API keys. Session-history reads/writes (`ListSessions`, `GetSessionTranscript`, `UpdateSessionMeta`, `DeleteSession`) live in `sessions.go`. |
| `internal/models/` | Structs that cross the Wails boundary (Session, Message, Preferences, AuthStatus, Model, Voice). |
| `frontend/src/App.tsx` | UI shell: floating pill nav → idle hub / active session / overlay. |
| `frontend/src/components/` | One component + its own CSS each (SetupPage, HubReady, CapturePanel, RegionSelector, Chat, MessageBubble, Overlay, Settings, ModelPicker, VoicePicker, WindowControls, History, SessionHistoryCard). |
| `frontend/src/lib/` | `wailsBridge.ts` (single import point for bound Go methods + models + runtime `EventsOn`/`EventsOff`) + hooks (`useVoiceRecorder`, `useAudioPlayer`) + `hotkey.ts` (browser mirror of the Go keymap, for the Settings hotkey-capture UI) + `format.ts` (history date/duration/model formatting). |
| `frontend/src/style.css` | MD3 design tokens (`:root` CSS variables) + global reset. |
| `frontend/wailsjs/` | Auto-generated bindings — **do not hand-edit**. |
| `docs/` | Roadmap, architecture reference, feature plans. |

## How to work on it

- **Toolchain is Go + npm** (not bun).
- **Run:** `wails dev` (hot reload) or `wails build` (binary). Frontend-only UI work: `cd frontend && npm run dev` — but Wails calls (`window.go.main.App.*`) no-op in a plain browser; stub them to preview.
- **Changed a bound Go method?** Run `wails generate module` (regenerates `frontend/wailsjs`), then export it from `lib/wailsBridge.ts`.
- **Verify changes:**
  - Go: `go build ./...`, `go test ./...`, `gofmt`.
  - Frontend types: `cd frontend && npx tsc --noEmit`.
  - UI behavior: browser preview with Wails calls stubbed. **Native window behavior** (overlay, always-on-top, transparency) can only be confirmed with `wails dev`.

## Rules

- **Comment for humans.** Every exported Go func and every React component/hook gets a short doc comment stating its purpose; comment the *why* for non-obvious logic. Match the existing density in `app.go` and `internal/ai/client.go`.
- **Modularize.** Keep `app.go` thin — logic lives in `internal/` packages, one concern each; a new external integration is a new package (e.g. `internal/voice`). Frontend: one component per file + its own CSS; all Go calls go only through `lib/wailsBridge.ts`.
- **Reusable UI.** Build small, single-responsibility components and compose them. Before adding markup, look for an existing component or class to reuse. Extract repeated UI into shared components/classes instead of re-implementing per screen — buttons (use the shared `.btn*` classes in `App.css`, not per-screen button styles), chips/badges, the pulsing status dot, icon buttons, the modal shell, glass panels. Reuse the MD3 tokens rather than duplicating values; lift shared behavior into hooks.
- **Screen-driven invariant.** Never send a written problem statement — the screenshot carries it. The interviewer persona lives in `internal/ai/prompts.go`.
- **Styling.** Plain CSS + MD3 CSS-variable tokens (`style.css :root`); no Tailwind. Mockups come from Google Stitch (Tailwind) — port them to the tokens. One CSS file per component.
- **Secrets.** API keys live only in the Go backend (SQLite). The frontend never sees them. The AI never speaks unless the user typed/spoke first — no unprompted interruptions.
- **AI calls.** Always set `max_tokens` (replies are short; an unset cap 402s on low OpenRouter balances). See `internal/ai/client.go`.
- **Go.** `gofmt`; return errors, don't panic; wrap with `fmt.Errorf("context: %w", err)`; `json:"..."` tags on boundary structs; `context.Context` for cancellable ops (API calls, capture loops).
- **React.** Functional components + hooks only; handle loading/error state for every async Go call.
- **Commits.** Conventional commits (`feat:`, `fix:`, `chore:`, `docs:`). No hardcoded keys, ever.

## See also

- [docs/roadmap.md](docs/roadmap.md) — phases & current status
- [docs/architecture.md](docs/architecture.md) — data flow, full bindings, prompt spec, OpenRouter/ElevenLabs API contracts
- [docs/model-picker-plan.md](docs/model-picker-plan.md) — model picker design reference (Phase 3, implemented)
- [docs/voice-integration-plan.md](docs/voice-integration-plan.md) — voice (ElevenLabs) implementation plan (Phase 2)
- [docs/push-to-talk-plan.md](docs/push-to-talk-plan.md) — global voice hotkey (toggle) design reference + the global-vs-in-app scope decision (Phase 3, implemented)
- [docs/history-feature-plan.md](docs/history-feature-plan.md) — session history feature plan + storage/data-flow notes (Phase 3, implemented)
