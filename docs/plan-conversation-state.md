# Plan: Conversation State, Cost Control, and Session Limit (revised)

## Context

The OpenRouter chat completions API is stateless — every call must include the full conversation history. The current app already trims to the last 20 messages (`internal/ai/client.go:14-16`), so context size is bounded by message count, but two problems remain:

1. **Cost runaway from screenshots.** `BuildUserMessage` (`internal/ai/client.go:145-162`) attaches a base64 screenshot to every user message. The 20-message trim keeps ~10 user turns, each still carrying its original screenshot. At ~1.4k image tokens per screenshot (capture resizes to ≤1280px wide), every API call ships ~14k image tokens (~$0.04 in Sonnet 4 input). For a 60-turn session that's ~$2.50 — dominated almost entirely by old screenshots the model no longer needs.
2. **No session length boundary.** Sessions can run indefinitely. There is no clock, no warning, no cutoff. A 30-minute window matches industry phone-screen norms and bounds worst-case cost — but only the app can enforce it; the AI is unaware of time.

There is also a third, quieter problem that only becomes visible once the first two are fixed:

3. **Silent eviction loses continuity.** `trimHistory` drops everything past the last 20 messages with no replacement. Ten exchanges go by quickly in a 30-minute voice session, after which the interviewer simply forgets approaches the candidate discussed earlier. This is a quality gap, not a cost gap — text cost is already bounded by the trim.

The original idea was to summarize after every exchange to bound payload size. That misidentifies the cost driver (images, not text) and doubles API calls per turn — so it costs more, not less. The right fix is to strip stale screenshots before sending, keep recent text history verbatim, and — as an optional last step — summarize *evicted* messages so long sessions keep continuity.

## Critique of the "summarize every turn" approach

| Concern | Reality |
|---|---|
| Text history will balloon and cost a lot | Text turns are ~50 tokens each. Even 60 turns is ~6k text tokens — negligible vs. ~14k *image* tokens per call. |
| Summarizing each turn bounds payload size | True, but it doubles API call count (response + summary) and adds 1-2s of perceived latency per exchange. |
| Summarization is cheap | A 2-sentence reply summarized to a 2-sentence summary saves nothing. Summarization only helps when input >> compressed output. |
| Result | More cost, more latency, lossy dialogue, no help with the actual cost driver (images). |

The *concern* (unbounded growth) is correct. The *lever* (text summarization per turn) is wrong. The right lever is image stripping; summarization earns its keep only at eviction time (see step 4).

## Recommended approach

Four small, independently shippable steps. Step 1 alone solves ~90% of the cost concern. Step 4 is optional and addresses continuity, not cost.

### 1. Strip past screenshots + update the system-prompt contract (biggest win)

Build a pre-send transform in `internal/ai/client.go`. Walk the message slice and rewrite every user message that has `[]ContentPart` content so it keeps only the text — collapsed to plain string content. The **last** user message keeps its screenshot. Pipeline inside `Complete`: `stripPastImages` → `trimHistory`. Result: per-call image tokens drop from ~14k to ~1.4k (~90% cut) with zero quality loss; old text dialogue is preserved verbatim.

**System-prompt contract.** `BuildSystemPrompt` (`internal/ai/prompts.go`) currently tells the model *"a screenshot of the candidate's screen is attached to each of their messages."* After stripping, that is false for every message but the last — the model may be confused reading history or claim it can't see earlier screens. Update the prompt to state the new contract: only the *most recent* message carries a screenshot, it shows the candidate's *current* screen, and earlier screenshots are intentionally omitted. With that explained, stripped messages need no placeholder parts.

**Boundary:** do this in `client.go`, not `app.go` — pure send-time transforms belong next to the existing `trimHistory`.

**Invariant:** construct new `ChatMessage` slices for the stripped messages; do not mutate the caller's slice. The value of this is engineering hygiene — pure transforms are unit-testable, idempotent, and immune to slice-aliasing bugs. (Accepted trade-off: the in-memory `activeSession.history` retains all screenshots for the session, ~0.3–0.7MB per turn; the 30-minute cap bounds this.)

**No storage implications:** screenshots are never persisted to SQLite (`AddMessage` stores only a `HasImage` flag), so stripping touches nothing on disk.

**Caching side effect:** post-stripping, only the tail of the message list changes between turns, making history prefix-stable — this tees up OpenRouter prompt caching later (still out of scope).

### 2. Session timer + soft warning + hard cutoff (no new API)

No new bound method, no polling. The pieces already exist:

- `models.Session.StartedAt` is set by `CreateSession` (`internal/store/sessions.go`) and already returned to the frontend by `StartSession`.
- The frontend already loads preferences on mount and reloads them when Settings closes (`loadPrefs` in `App.tsx`).

**Frontend:** compute elapsed locally with a 1s `setInterval` from `session.startedAt`; limits come from the already-loaded prefs. Same-machine clocks make skew a non-issue.

- **0–25 min:** normal. Header shows `mm:ss / 30:00`.
- **25–30 min:** warning banner ("5 minutes remaining").
- **At 30 min:** disable the chat input, show "Session time limit reached." The End button stays enabled so the candidate can review the transcript before closing.

**Backend (enforcement backstop):** at the top of `SendMessage`, check elapsed via `a.active.session.StartedAt` against the configured limit; past it, return a user-presentable error (it lands in the existing error banner — do not rely on the frontend string-matching it; the frontend's own timer is the primary UX). Skip the check when the limit is 0.

### 3. Configurable limits in preferences

Add two fields to `Preferences`: `SessionLimitMinutes` (default 30) and `SoftWarningMinutes` (default 25). Surface number inputs in `Settings.tsx`. Persist via the existing `preferences` table — extend `internal/store/preferences.go` with two new keys following the `keyCaptureIntervalMs` pattern.

**Validation:** `SoftWarningMinutes < SessionLimitMinutes`; `0` means "no limit" (untimed practice) and disables both the warning and the cutoff check.

### 4. Eviction-based rolling summary (optional, last — fine to defer)

After image stripping and the session cap, text cost is a non-issue. What remains is **continuity**: once the conversation exceeds 20 messages, `trimHistory` silently drops the oldest turns and the interviewer forgets them. Fix that by summarizing *evicted* messages — not messages inside the verbatim window.

- **Trigger:** when the count of evicted-and-not-yet-summarized messages reaches ≥ 8 (≈ once per 4 exchanges in long sessions — batched so it never runs per-turn). Export `ai.MaxHistoryMsgs` so `app.go` can compute the eviction boundary.
- **State:** `summary string` and `summarizedThroughIdx int` live on `activeSession` in `app.go` — the layer that owns `history` and the session lifecycle. The `ai` package stays pure/stateless.
- **Call:** `(c *Client) Summarize(ctx, messages)` using `anthropic/claude-haiku-4-5` (hardcoded — small, cheap, fine for mechanical summarization; not a preference) and the prompt from `BuildSummarizationPrompt`. 10s timeout; on failure, fall through and send without an updated summary (fail-open). Synchronous inside `SendMessage`, so the existing `ctx` propagation cancels it if the user ends the session mid-call. Adds ~1s on roughly every 4th exchange — acceptable.
- **Splice:** `Complete` gains one optional `summary string` parameter (no request-struct refactor). If non-empty, splice it as a second system message — `"Context from earlier in this interview: ..."` — *after* trimming, so the summary itself can never be evicted.

This will genuinely fire in long sessions (unlike a token-threshold trigger, which short interviewer-style messages would never reach), and it fixes real information loss rather than re-bounding already-bounded text cost.

## File changes

### `internal/ai/client.go`
- Add `stripPastImages(messages []ChatMessage) []ChatMessage` — returns a NEW slice; only the last user message keeps its `image_url` content part; stripped messages collapse to plain string content.
- `Complete` pipeline: `stripPastImages` → `trimHistory` → POST. Signature unchanged for steps 1–3.
- Step 4: export `MaxHistoryMsgs`; add `(c *Client) Summarize(ctx, messages)`; add optional `summary string` param to `Complete`, spliced after trim.

### `internal/ai/prompts.go`
- Step 1: update `BuildSystemPrompt` — only the latest message carries a screenshot; it reflects the current screen; earlier screenshots are intentionally omitted.
- Step 4: add `BuildSummarizationPrompt() string`: *"Summarize the following interview conversation in 2-3 sentences. Capture: the problem being worked on, approaches the candidate has tried, key feedback already given. Do not invent details. Output the summary only."*

### `internal/models/session.go`
- Extend `Preferences` with `SessionLimitMinutes int` (default 30) and `SoftWarningMinutes int` (default 25). No new structs — no `SessionStatus`.

### `internal/store/preferences.go`
- New keys: `keySessionLimitMinutes`, `keySoftWarningMinutes`. Load/save with defaults (30 / 25), following the existing `keyCaptureIntervalMs` pattern.

### `app.go`
- `SendMessage`: at the top, check elapsed via `a.active.session.StartedAt` against the configured limit (skip if 0); past it, return a user-presentable "session time limit reached" error. No new `startedAt` field, no `SessionStatus` binding.
- Step 4: add `summary` / `summarizedThroughIdx` to `activeSession` (init `-1` in `StartSession`); orchestrate `Summarize` from `SendMessage` when the eviction batch threshold is met; pass the summary to `Complete`.

### `frontend/src/App.tsx`
- 1s `setInterval` while a session is active, computing elapsed from `session.startedAt` (returned by `StartSession`).
- Header timer `mm:ss / 30:00`; warning banner past the soft threshold; disable chat input + cutoff banner past the limit (End stays enabled); skip all of it when limit is 0.
- A backend "limit reached" error surfaces through the existing error banner — no special-casing needed.

### `frontend/src/components/Settings.tsx`
- Two number inputs: `sessionLimitMinutes`, `softWarningMinutes`. Wire to `GetPreferences` / `UpdatePreferences`. Validate warning < limit; 0 = unlimited.

### `frontend/src/lib/wailsBridge.ts`
- Unchanged — no new bindings.

## Sequencing (independently shippable)

1. **Image stripping + prompt contract** — `stripPastImages`, `BuildSystemPrompt` update, unit tests. Smallest diff, biggest cost win. Solves ~90% of the problem on its own.
2. **Session timer** — frontend timer + warning + cutoff UI from existing `startedAt`; backend backstop in `SendMessage` (using defaults until step 3 lands).
3. **Configurable limits** — preferences fields, store keys, Settings UI, validation.
4. **Eviction-based summary** — `Summarize`, `MaxHistoryMsgs` export, splice logic, `activeSession` state. Optional; defer if steps 1–3 prove sufficient in real sessions.

Each step is revertable. Steps 1 and 2 can land in either order.

## Verification

### Unit tests
- `stripPastImages_test.go`:
  - 5 user messages each with an image. Assert the returned slice has image parts on only the last one; assert original input is unmodified (no aliasing).
  - User message that has plain string content (no image): assert unchanged.
  - Empty history: assert no panic.
- Step 4: eviction-batch trigger arithmetic (history length vs. `summarizedThroughIdx` vs. window size).

### Manual end-to-end
1. Start a session with a screenshot region configured. Use `OPENROUTER_KEY` from `.env` so auth is trivial.
2. Send 12+ messages. Add a temporary log in `Complete` and confirm the outbound JSON has `image_url` only on the most recent user message.
3. Watch the header timer advance. Set a 1-min warning / 2-min limit in Settings for quick testing: confirm the warning banner appears, then at the limit the chat input is disabled, the cutoff banner is visible, the End button still works, and clicking End persists the session normally.
4. Confirm a fresh session resets the timer, and that limit 0 disables both warning and cutoff.
5. Step 4 only: lower the eviction batch threshold for one run, send 12+ exchanges, and confirm a second `system` message containing the summary appears in the next outbound payload. Restore the threshold afterwards.

### Cost sanity check
- Before: a 12-turn session sends ~17k input tokens per call by the last turn (~14k images + ~3k text + system).
- After step 1: a 12-turn session sends ~1.4k image tokens + text per call regardless of turn number.
- Confirm via the OpenRouter dashboard before/after step 1.

## Out of scope (flagged for later)

- Streaming responses (perceived-latency win, orthogonal to cost).
- OpenRouter / Anthropic prompt caching (`cache_control: ephemeral`) — image stripping makes history prefix-stable, so this becomes an easy follow-up, but it adds complexity now.
- Cost meter in the UI (pricing tables shift; sub-dollar sessions don't need it).
- Retry / backoff on OpenRouter failures.
- Blocking `StartSession` until the first screenshot is available (pre-existing issue: first message ships with no image).
- Replacing already-superseded screenshots inside the in-memory `activeSession.history` to bound memory — unnecessary under the 30-minute cap.
