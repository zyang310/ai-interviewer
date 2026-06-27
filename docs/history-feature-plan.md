# Session History — Feature Plan

> Status: ✅ implemented (Phase 3). The phases below document how it was built; see [architecture.md](architecture.md#persistence-sqlite) for the persistence/data-flow reference.

## Context

**Why now:** `docs/roadmap.md` Phase 3 lists "Session history view" as *partial* — the backend bindings `ListSessions` / `GetSessionTranscript` already exist and are exported, but the History tab in `App.tsx` is just a placeholder ("coming soon"). This feature replaces that placeholder with a real, designed History page that matches the provided Stitch mockup (ported to the app's plain‑CSS MD3 tokens — **no Tailwind**), and fills the three backend gaps the mockup exposes.

**Outcome:** From the History tab the user sees a reverse‑chronological list of past sessions — each row showing an AI‑derived **problem title + difficulty badge**, **date**, **duration**, and **model** — can **expand** any row to read the full interviewer/user transcript inline, and can **delete** a session.

**Decisions confirmed with the user:**
- **Title + difficulty:** the app is screen‑driven and stores neither today, so they're derived by a **best‑effort async AI call at session end** (not manual, not omitted). Falls back to a generic label if extraction fails or hasn't completed.
- **"Go to Debrief" button:** rendered but **disabled / no‑op placeholder** — debrief is a later feedback feature.
- **Screenshots are not persisted;** the transcript is text‑only (matches the mockup).

## Current state (what already exists — reuse, don't rebuild)

- **Bound + exported already:** `ListSessions() []SessionSummary` and `GetSessionTranscript(id) []Message` (`app.go` ~359–368; re‑exported in `frontend/src/lib/wailsBridge.ts`).
- **Storage** (`internal/store/db.go`): `sessions(id, problem_id, model, started_at, ended_at)` + `messages(id, session_id, role, content, has_image, created_at)`.
- **Store fns** (`internal/store/sessions.go`): `CreateSession`, `EndSession`, `ListSessions`, `AddMessage`, `GetMessages`.
- **Frontend:** view state `useState<"hub"|"history"|"settings">` (`App.tsx:57`); history branch is the placeholder at `App.tsx` ~423–430. `MessageBubble` (`role`/`content`) is directly reusable for transcript bubbles. `Settings.tsx` `savePrefs` (~152–168) is the canonical loading/error async pattern. Design tokens in `style.css :root`; shared `.btn` / `.btn-primary` / `.btn-ghost` / `.btn-danger` in `App.css`; Material Symbols loaded via CDN in `index.html`.

## Gaps this plan closes

1. No problem **title / difficulty** captured anywhere (screen‑driven).
2. `SessionSummary` has no `endedAt` → client can't compute **duration**.
3. No **`DeleteSession`** method.

---

## Phase 1 — Backend data layer
Files: `internal/store/db.go`, `internal/store/sessions.go`, `internal/models/session.go`

1. **Migration** (`db.go` `migrate()`): add two nullable columns to `sessions`: `problem_title TEXT`, `difficulty TEXT`. SQLite has no `ADD COLUMN IF NOT EXISTS`, so add a small idempotent helper that reads `PRAGMA table_info(sessions)` and runs `ALTER TABLE sessions ADD COLUMN …` only when the column is absent. Also update the `CREATE TABLE` definition so fresh DBs get the columns directly.
2. **`models.Session`**: add `ProblemTitle string` (`json:"problemTitle"`), `Difficulty string` (`json:"difficulty"`).
3. **`models.SessionSummary`**: add `Difficulty string` (`json:"difficulty"`) and `EndedAt *time.Time` (`json:"endedAt,omitempty"`). (`ProblemTitle` already exists — it's just always empty today.)
4. **`sessions.go`**:
   - `ListSessions`: extend the SELECT to include `ended_at, problem_title, difficulty`; populate the new summary fields (COALESCE NULLs → `""`/nil; use `sql.NullString` for the nullable columns, matching the existing dual RFC3339 / `2006-01-02 15:04:05` time parsing). Keep the existing `ORDER BY started_at DESC` + message‑count grouping.
   - New `UpdateSessionMeta(id, title, difficulty string) error` → `UPDATE sessions SET problem_title=?, difficulty=? WHERE id=?`.
   - New `DeleteSession(id string) error` → single transaction: delete `messages WHERE session_id=?`, then the `sessions` row. Manual cascade is **required** — foreign keys are enabled (`db.go:29` `?_foreign_keys=on`) and the `messages` FK has no `ON DELETE CASCADE`, so deleting a session that still has messages would otherwise fail.

Verify: `go build ./...`, `go test ./...`, `gofmt -l`.

## Phase 2 — AI title/difficulty extraction
Files: `internal/ai/prompts.go`, `internal/ai/client.go`, `app.go`

1. **`prompts.go`**: add a `ProblemMetaPrompt` constant instructing the model to read a transcript and return **strict JSON** `{"title": "...", "difficulty": "Easy|Medium|Hard"}` — short title (≤ 4 words, e.g. "LRU Cache"); empty `title`/`difficulty` when unclear.
2. **`client.go`**: add `ExtractProblemMeta(ctx, model, transcript string) (title, difficulty string, err error)` — a **text‑only** chat call reusing the existing OpenRouter request path, with a small `max_tokens` (per the rules, always cap), tolerant JSON parse (strip code fences/whitespace).
3. **`app.go` `EndSession`** (currently `app.go:215–223`): **before** the existing `a.active = nil`, capture the session's `model` from `a.active.session.Model` into a local (it's unrecoverable afterward without a new query, and `EndSession` only receives `sessionID`). After `db.EndSession(id)`, launch a **goroutine** (`context.Background()` + timeout, since the request context ends) that: reads `db.GetMessages(id)`, builds a transcript string (skip the system message), calls `ExtractProblemMeta(ctx, model, transcript)`, then `db.UpdateSessionMeta`. **Best‑effort:** log and return on error — never block or fail `EndSession`. Skip when there are too few messages to label.

> Screen‑driven invariant preserved: extraction only *labels history after the fact*; the live interview is still driven solely by the screenshot.

Verify: `go build ./...` (runtime check happens in Phase 6).

## Phase 3 — Regenerate bindings
1. `wails generate module` — regenerates `frontend/wailsjs` (new `SessionSummary` fields + `DeleteSession`).
2. `wailsBridge.ts`: export `DeleteSession`. (`ListSessions` / `GetSessionTranscript` are already exported.)

Verify: `cd frontend && npx tsc --noEmit`.

## Phase 4 — History UI components
New files (one component + its own CSS each, per the rules):
- `frontend/src/components/History.tsx` + `History.css`
- `frontend/src/components/SessionHistoryCard.tsx` + `SessionHistoryCard.css`
- `frontend/src/lib/format.ts` — small pure helpers: `formatSessionDate` ("Oct 24, 2023"), `formatDuration(startedAt, endedAt)` ("45m 12s"), `prettyModel(modelId)` ("anthropic/claude-sonnet-4" → "Claude Sonnet 4").

**`History.tsx`** (self‑contained, no props):
- On mount, call `ListSessions()` following the `Settings.savePrefs` loading/error pattern; render header ("Session History" + subtitle) and a vertical list of `SessionHistoryCard`.
- Owns `expandedId` (one open at a time) + a transcript cache `{ [id]: Message[] }`; on expand, lazy‑call `GetSessionTranscript(id)` with a per‑card loading state.
- **Delete:** inline two‑step confirm on the row (click delete → row shows "Delete? ✓ ✕") to avoid building a new modal; on confirm, `DeleteSession(id)` then drop the row from local state (no full refetch).
- **Empty state:** reuse the existing `.history-placeholder` look already in `App.css` ("No sessions yet").

**`SessionHistoryCard.tsx`** (props: `summary`, `expanded`, `transcript`, `loadingTranscript`, `onToggle`, `onDelete`):
- **Collapsed:** title (fallback "Interview session" when empty) · neutral difficulty badge (hidden when empty) · date (`calendar_today`) · duration (`timer`) · model via `prettyModel` (`smart_toy`); trailing delete icon‑button + `expand_more`/`expand_less` chevron.
- **Expanded:** "Transcript" subheader → scrollable list of reused `MessageBubble`s → footer with a **disabled** "Go to Debrief" `.btn` placeholder.

**Styling (port mockup → tokens):** cards = `var(--surface-container)` + `1px var(--outline-variant)` + 12px radius (mirrors `.settings-card`); the mockup's `#1A1D23` badge background maps exactly to `var(--level-1-bg)`, badge text → `var(--outline)`; reuse `.btn`/`.btn-ghost` and the app's existing scrollbar treatment for the transcript area; all icons via `material-symbols-outlined`. Difficulty color‑coding is left out (mockup is neutral) — note in code as an easy future tweak.

## Phase 5 — Integrate into the shell
- `App.tsx`: import `History`; replace the placeholder branch (~423–430) with `<History />`. The pill nav already has the History tab and the `"history"` view state — no nav changes needed.

## Phase 6 — Verify end‑to‑end
- `go build ./...`, `go test ./...`, `gofmt -l`.
- `cd frontend && npx tsc --noEmit`.
- `wails dev`: run a short session → **End** → open **History**: row appears immediately; title/difficulty fill in shortly after (async extraction); **expand** → transcript renders via `MessageBubble`; **delete** → row disappears and stays gone after an app restart. Confirm old/pre‑migration sessions show the fallback title and a correctly computed duration.
- Optional UI‑only pass: `cd frontend && npm run dev` with the Wails calls stubbed (they no‑op in a plain browser).

## Debrief (implemented — extends this feature)
The expanded history card has a **Transcript / Debrief tab toggle**; selecting **Debrief** opens an AI
**scorecard** for the finished session, rendered by `components/Debrief.tsx`.

- **Cheap by design.** Generated lazily the first time the Debrief tab is opened by `App.GetDebrief(id)`
  and **cached** in a new `sessions.debrief` column — re-opening reads from SQLite (zero tokens). One
  text call, capped via the shared `max_tokens`, on the **session's own model**. The card defaults to
  the **Transcript** tab, so merely expanding a card never spends tokens.
- **Real code context.** Transcripts never stored screenshots, so to judge the actual solution the
  end-of-session call was upgraded: `ExtractProblemMeta` → `ExtractSessionMeta` now takes the final
  screenshot (one vision call, folded into the existing labeling work) and also returns the
  candidate's final code, persisted to a new `sessions.final_code` column. `GenerateDebrief` reads
  transcript + final code.
- **Shape.** `models.Debrief{ verdict, summary, rubric{problemSolving,coding,communication,complexity,pace}, strengths[], improvements[] }`;
  verdict is the 5-point hire scale, the rubric scores **five** dimensions 1-5 (0 = insufficient
  evidence) — `pace` (time management) was added alongside the UI redesign. Parsing reuses the
  brace-extraction tolerance from the label parser; scores are clamped, verdict normalised. Old cached
  debriefs predate `pace`, so it deserialises to 0 (shown as "—") until regenerated — no migration.
- **UI.** Two-column scorecard ported from the Stitch mockup: verdict chip + summary + Strengths /
  To Improve lists on the left; a "Performance metrics" five-bar list and a `RadarChart.tsx` pentagon
  (self-contained SVG, MD3 tokens, no chart dependency) on the right.
- **Flow.** `History.tsx` lazy-loads/caches per card via the idempotent `ensureDebrief` (mirrors the
  transcript cache); `SessionHistoryCard.tsx` owns the active tab and triggers the load on first
  Debrief-tab open; binding `GetDebrief` in `lib/wailsBridge.ts`.

## Out of scope (future)
- Persisting screenshots / showing the problem image inside the transcript.
- Search, filtering, pagination, and difficulty color‑coding.
- A "Regenerate debrief" action (intentionally omitted — the result is cached to avoid re-spend).
