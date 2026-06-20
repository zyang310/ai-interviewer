# AI Interviewer — Summary

## What it is

A desktop app that runs a live, voice-driven mock coding interview while you code in your own IDE. It watches your screen, listens to you talk through your approach, and pushes back like a real technical interviewer — Socratic, terse, never volunteering the answer.

## Who it's for

Engineers prepping for technical interviews who want realistic, on-demand practice without booking a peer or paying a coach. The user keeps their normal coding environment (VS Code, JetBrains, terminal — whatever) and the app floats alongside it.

## The core experience

1. User picks (or pastes) a problem and starts a session.
2. User codes in their own IDE. The app captures the screen on a short interval (default ~3s).
3. User holds push-to-talk and explains their thinking out loud.
4. The interviewer's voice replies through the speakers within ~1 second:
   - Probes assumptions ("What happens if the array is empty?")
   - Asks for time/space complexity when an approach is proposed
   - Nudges, never solves ("What data structure gives you O(1) lookup?")
   - Stays silent until spoken to — no unprompted interruptions
5. After the session ends, the user gets a transcript and (optionally) a debrief where the AI drops the interviewer persona and gives direct, honest feedback.

## Why it's different

- **Sees the code, doesn't read the code.** Vision-based screen capture means it works with any IDE, any language, any setup — no plugins, no language servers, no copy-paste.
- **Behaves like an interviewer, not a tutor.** The system prompt is tuned hard against the chatbot reflex to explain. Short responses. Socratic nudges. Silence by default.
- **Voice-native loop.** The user talks instead of typing, the interviewer talks back. The feel is closer to a real phone screen than a chat window.
- **Local-first and private.** All session data lives in a local SQLite file. No telemetry, no cloud sync. API keys stay in the Go backend and never reach the frontend.
- **Model-agnostic.** Routes through OpenRouter, so the user picks Claude / GPT / Gemini per session.

## How it works (tech)

- **Wails v2** packages a Go backend and a React + TypeScript frontend into one native binary using the OS webview (no bundled Chromium).
- **Go backend** owns every external API call and all sensitive state: screen capture, OpenRouter requests, ElevenLabs TTS/STT, SQLite storage.
- **React frontend** is only UI: chat panel, problem panel, mic recording (MediaRecorder), audio playback (Web Audio API), settings.
- **AI gateway:** OpenRouter — one API, any frontier model.
- **Voice:**
  - TTS via ElevenLabs Flash v2.5 (~75ms latency, streaming chunks play as they arrive)
  - STT via ElevenLabs Scribe v2 (push-to-talk → WebM/opus blob → text)
- **Screen capture:** `kbinani/screenshot` (Go-native, cross-platform). User can pick a display and crop to a region so the app's own window is excluded.
- **Storage:** SQLite via `mattn/go-sqlite3` — sessions, messages, problem bank, encrypted keys, preferences.

## Data flow (one turn)

```
User speaks (push-to-talk)
   └─► Frontend records audio (MediaRecorder)
        └─► Go: ElevenLabs Scribe v2 → text
             └─► Go bundles: text + latest screenshot (base64) + history + problem
                  └─► OpenRouter → AI text response (streaming)
                       └─► Go: ElevenLabs TTS Flash v2.5 → audio chunks (streaming)
                            └─► Frontend plays chunks via Web Audio API
                                 └─► SQLite logs the turn
```

## Constraints that shape the design

- **Latency is the product.** A 5-second pause kills the interview feel, so every stage streams: AI response streams, TTS streams, audio plays as chunks arrive.
- **Brevity is the product.** Long AI replies break immersion *and* cost more on TTS (billed per character). The system prompt enforces 1–3 sentence responses.
- **The AI must never reveal the answer.** This is the single hardest part of the prompt and the thing that distinguishes a useful interviewer from a useless one.
- **Privacy by default.** Screen content can be sensitive. Nothing leaves the machine except the calls to OpenRouter and ElevenLabs, and only the Go backend ever sees keys.

## Roadmap (phases)

1. **MVP** — typed chat + screen capture + single hardcoded problem + Socratic prompt.
2. **Voice** — ElevenLabs TTS/STT integrated, push-to-talk, streaming playback.
3. **Problem bank + UX** — multiple problems, model picker, voice picker, session history, settings.
4. **Auth + polish** — OpenRouter OAuth, always-on-top floating window, debrief mode, transcript export.
5. **Stretch** — adaptive difficulty, timer/pressure mode, multi-problem rounds, LeetCode URL import, custom interviewer voice cloning.

## Surfaces the user sees

- **Header:** Start / End interview, Settings.
- **Capture panel:** live screenshot preview, region selector.
- **Chat panel:** transcript of the conversation, push-to-talk button, recording / speaking indicators.
- **Settings modal:** API keys (or "loaded from environment"), model picker, voice picker, capture interval, region.
- **Session history:** past interviews with full transcripts.

## One-line pitch

> A floating desktop interviewer that watches your screen, talks back in real time, and refuses to give you the answer.
