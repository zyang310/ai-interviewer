# AI Interviewer

A desktop app that acts as a live AI-powered mock coding interview coach. You code in your own IDE while the app captures your screen and provides real-time Socratic interviewer feedback.

## Prerequisites

| Tool | Version | Install |
|------|---------|---------|
| Go | 1.23+ | https://go.dev/dl |
| Node.js | 18+ | https://nodejs.org |
| Wails CLI | v2 | `go install github.com/wailsapp/wails/v2/cmd/wails@latest` |
| Xcode Command Line Tools | any | `xcode-select --install` (not the full Xcode IDE) |

> **Why Xcode Command Line Tools?** Three dependencies use CGO (C bindings) on macOS: Wails itself binds to `WKWebView` (the OS webview), `go-sqlite3` embeds a C build of SQLite, and `kbinani/screenshot` calls CoreGraphics for screen capture. The CLT provides `clang` and the macOS SDK headers — the full Xcode IDE is not needed.

Verify everything is in place:

```bash
wails doctor
```

## Setup

```bash
# Install frontend dependencies
cd frontend && npm install && cd ..

# Download Go dependencies
go mod tidy
```

## Running in Development

```bash
wails dev
```

This starts hot-reload for both the Go backend and the React frontend. The app window opens automatically. Frontend changes reflect instantly; Go changes trigger a rebuild.

To work on the frontend UI only (no Go backend, faster iteration):

```bash
cd frontend && npm run dev
```

## Building for Production

```bash
wails build
```

Output: `build/bin/ai-interviewer.app` (macOS). Double-click to run, or:

```bash
open build/bin/ai-interviewer.app
```

## Project Structure

```
ai-interviewer/
├── main.go              # Entry point — Wails app config
├── app.go               # Go methods exposed to the frontend
├── internal/            # Backend packages (added in later phases)
├── frontend/
│   ├── src/             # React + TypeScript UI
│   ├── wailsjs/         # Auto-generated Wails bindings (do not edit)
│   └── package.json
├── build/               # App icons and platform metadata
└── wails.json           # Wails project config
```

## How the Wails Bridge Works

Go methods on the `App` struct in `app.go` are automatically available in the frontend as async TypeScript functions under `window.go.main.App.*`. The generated bindings live in `frontend/wailsjs/go/main/App.d.ts` — re-run `wails dev` or `wails generate module` to regenerate them after changing Go method signatures.

## Verifying the Build (No GUI)

```bash
# Compile Go only
go build ./...

# Compile frontend only
cd frontend && npm run build
```

Both should exit cleanly with no errors.

## Configuration

API keys are entered through the Settings panel inside the app (no `.env` file needed). All keys are stored locally in SQLite and never leave the machine.

Required for full functionality:
- **OpenRouter API key** — get one at https://openrouter.ai (used for AI responses)
- **ElevenLabs API key** — get one at https://elevenlabs.io (used for voice, Phase 2+)
