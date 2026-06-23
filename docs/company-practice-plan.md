# Company Practice Mode — Implementation Plan

> An **opt-in** mode where the user practices for a specific company. The AI greets them in
> character, **assigns a real interview-frequency LeetCode problem**, asks them to open it, and
> then runs the normal screen-driven interview — flavored by that company's interviewing style.
> Roadmap entry: [roadmap.md](roadmap.md) → Phase 6. **Status: planned (not started).**

## Context

Today every session is **screen-driven**: the candidate brings their own problem and the AI reads
it from the screenshot. This feature adds a second way in — pick **Google** (or Amazon, Meta, …),
and the AI says *"Hi, I'm your interviewer at Google today. Let's work on Two Sum — open it on
LeetCode and walk me through your first thoughts."* That's where the interview properly starts;
from there the existing capture + Socratic loop takes over, but the interviewer behaves the way
that company's interviewers do.

**This is additive, never a replacement.** The default Hub flow ("interview on whatever I have
up") stays the default and is untouched. Company Practice is a separate tab. Difficulty filtering
(Easy/Medium/Hard) and a randomize ("surprise me") pick are included.

## Two design tensions, resolved (the soundness core)

- **Screen-driven invariant** ([CLAUDE.md](../CLAUDE.md): *"never send a written problem statement
  — the screenshot carries it"*). Company mode assigns a problem **by reference only** — title +
  difficulty + LeetCode link — **never** the problem statement text. The candidate opens the real
  problem; the AI still reads the actual prompt and their code from the screenshot, and uses its
  own knowledge of the named (well-known) problem to interview. We bundle **metadata only** — which
  also sidesteps LeetCode-content copyright.
- **"AI speaks first"** ([CLAUDE.md](../CLAUDE.md): *"the AI never speaks unless the user
  typed/spoke first"*). The opener is a **template-derived greeting** shown in the transcript and
  spoken aloud (if voice mode is on). To avoid models that reject a leading non-`user` turn, the
  opener is **not** inserted as an assistant message — instead the company **system prompt encodes**
  *"you have just greeted the candidate and assigned {problem}; continue from there."* Model history
  stays `system → user → …`. This is a deliberate, scoped exception that fires only at the start of
  a company session the user explicitly initiated.

## Data source & curation

[github.com/liquidslr/interview-company-wise-problems](https://github.com/liquidslr/interview-company-wise-problems)
— one folder per company (100+), each with CSVs by time window (`5. All.csv` is the full list).
Columns:

```
Difficulty,Title,Frequency,Acceptance Rate,Link,Topics
EASY,Two Sum,100.0,0.557…,https://leetcode.com/problems/two-sum,"Array, Hash Table"
```

`Frequency` (0–100) is exactly the "asked often" signal we sort by. The repo has **no explicit
license file**, so rather than vendoring its CSVs wholesale we **curate a focused subset into our
own data file** — top-by-frequency, biased to well-known problems the AI reliably knows — and
**attribute the source**. This is licensing-safe (factual metadata, no problem text) and higher
quality than dumping 50+ obscure entries.

---

## Phase 0 — Data & company profiles

1. **Curate `internal/problems/data/<Company>.csv`** for ~10–15 companies (Google, Amazon, Meta,
   Microsoft, Apple, Netflix, Bloomberg, Uber, Adobe, LinkedIn, …): ~20–30 top-frequency,
   well-known problems each (`Difficulty,Title,Frequency,Link,Topics`). Add `data/SOURCE.md`
   crediting the upstream repo.
2. **Author `companyProfiles`** — a map in `internal/ai` (company → short *authored* style
   guidance): Google = algorithmic depth + complexity rigor + clean code; Amazon = DSA **plus**
   Leadership-Principles behavioral probing; Meta = fast pace, two problems; generic fallback for
   any company without a specific profile. This is authored guidance, **not** model recall — that's
   what makes it reliable.

## Phase 1 — Backend

3. **`internal/problems` package** — embed and serve the metadata:
   - `//go:embed data/*.csv` → parse once with `encoding/csv`, cache (mirror the lazy-cache pattern
     in `internal/voice`).
   - `Problem` lives in `internal/models`:
     `{ Company, Title, Difficulty, Frequency, Link, Topics []string }` (json tags; normalize
     difficulty `EASY`→`Easy`).
   - API: `Companies() []string`, `Problems(company) []models.Problem`. Filtering / sorting /
     randomize live **frontend-side** over `Problems` (responsive); no backend random needed.
4. **Company-mode prompt, kept DRY** — in `internal/ai/prompts.go`, extract the shared rules +
   spoken-style block into one base string. `BuildSystemPrompt` = base (default, unchanged
   behavior). `BuildCompanySystemPrompt(profile, p models.Problem)` = base + a company header that
   injects the persona and the assignment, instructing the AI to *begin as if it has just greeted
   the candidate and assigned {Title} ({Link}); if the problem isn't visible on their screen yet,
   ask them to open it.* Reuses, never duplicates, the rules.
5. **Opening turn (no AI call, no leading assistant turn)** — a deterministic template:
   *"Hi, I'm your interviewer at {Company} today. We'll work on {Title}, a {Difficulty} problem.
   Open it on LeetCode and walk me through your initial thoughts when you're ready."* Returned to
   the frontend to display + speak; the system prompt (step 4) carries the same assignment so the
   model is consistent when the candidate replies.
6. **Bound methods** (`app.go`, kept thin):
   - `ListCompanies() []string`, `ListCompanyProblems(company) ([]models.Problem, error)`.
   - `StartCompanySession(company string, problem models.Problem) (models.CompanySessionStart, error)`,
     where `CompanySessionStart { Session models.Session; Opening string }` — a **struct return**
     (not a 3-value return) for clean Wails binding. It seeds history with
     `BuildCompanySystemPrompt`, starts capture (reusing `StartSession`'s body), and returns the
     templated opener. v1 keeps company/problem context **in-memory** on the active session (no
     SQLite migration yet).
   - `OpenURL(url string) error` wrapping `runtime.BrowserOpenURL(a.ctx, url)` so the LeetCode link
     opens in the real browser, not the frameless webview.
   - `StartSession(model)` (default mode) stays **unchanged**. Run `wails generate module`; export
     via `lib/wailsBridge.ts`.

## Phase 2 — Frontend (new tab + flow)

7. **New "Company Practice" tab** — add `"company"` to the pill-nav `view` union in
   [App.tsx](../frontend/src/App.tsx) (currently `"hub" | "history" | "settings"`) with its own
   pill button; new `CompanyPractice` component (+ CSS). Starting a company session sets the
   **existing** active-session state, so the active Chat/overlay UI (which renders independent of
   `view`) is reused as-is.
8. **Picker UI** — reuse the `ModelPicker`/`VoicePicker` list idiom:
   - Searchable company selector.
   - **Difficulty filter** chips: All / Easy / Medium / Hard.
   - **Sort**: by Frequency (default — "asked often"), with Title/Difficulty options.
   - **Randomize** ("Surprise me") — pick a random problem from the current filtered set.
   - Problem rows: title, difficulty badge, topics, frequency, and an "Open on LeetCode" button
     (→ `OpenURL`).
9. **Start flow** — selecting a problem → "Start interview" → `StartCompanySession` → enter the
   **existing** active-session UI with the AI's opening message displayed as the first interviewer
   turn, spoken aloud if voice mode is on (`speak()` — the opener is clean text, TTS-safe). Show a
   small banner during the session: "{Company} · {Title} · {Difficulty} · Open on LeetCode". The
   default Hub start path is unchanged.

## Phase 3 — Polish & stretch

10. Persist last company + difficulty filter in `Preferences` (existing plumbing); persist
    company/problem on the `Session` row for history (the SQLite migration deferred from v1).
11. Optional **AI-generated opener** (one model call) behind the templated default, for a more
    natural, persona-rich greeting.
12. Tests: `internal/problems` (CSV parse, difficulty normalization, filter, random) and
    `internal/ai` (company prompt contains the company persona + assigned problem). Update
    [architecture.md](architecture.md) and [CLAUDE.md](../CLAUDE.md) (codebase map + the
    screen-driven invariant nuance: company mode assigns by reference, never ships problem text).

## Verification

- **Go:** `go build ./...`, `go test ./...` (new `internal/problems` + prompt tests), `gofmt -l .`.
- **Bindings:** `ListCompanies` / `ListCompanyProblems` / `StartCompanySession` / `OpenURL`
  present under `frontend/wailsjs/`.
- **Types/bundle:** `cd frontend && npx tsc --noEmit` && `npm run build`.
- **End-to-end (`wails dev`):**
  1. Company Practice tab → pick Google → filter Medium → Randomize → a problem is chosen.
  2. Start → the AI **speaks first**, introduces itself as the Google interviewer, and names the
     problem; "Open on LeetCode" opens the real page in the browser.
  3. Solve in your editor → screenshots flow → the interview runs screen-driven, flavored by the
     Google profile (complexity rigor, etc.), never revealing the answer.
  4. **Regression:** the default Hub flow still starts a normal screen-driven session with no
     company context and no unprompted opener.
