# Managed Test Accounts — Implementation Plan & Status

> The **step-by-step, phase-by-phase** build plan for the managed test-account
> tier. Design rationale (the *why*) lives in
> [managed-keys-plan.md](managed-keys-plan.md); this is the *how*, with a
> **Checkpoint** after each step that re-examines it for simplicity, efficiency,
> and consistency with earlier steps.
>
> **Status legend:** ✅ done · ◑ in progress · ☐ not started
>
> **Current status (2026-07-16):** **Phases 0–2 ✅ complete** — the access
> service is built and verified end-to-end locally (Phase 0); the app backend
> (store → access client → account service → mode-aware AuthStatus → bindings +
> launch refresh) is implemented and unit-tested against fakes (Phase 1); the
> full frontend (app-shell plumbing, InviteActivation, SetupPage two-door fork,
> Settings account card + mode fork + invite entry, pinned-model lock card +
> managed voice guards) is built and verified (2.1–2.5), and the **2.6 full
> local e2e battery passed** against a live memory/log service under
> `wails dev` — redeem, pinning, voice resolution, kill switch, BYOK
> regression, sign-out, offline grace, Clear-All — which also closed 1.10's
> deferred interactive gate and 2.3's live drive. **Phase 3 started
> (2026-07-18): 3.1 ✅ complete** — both shared keys minted and verified live
> (`access-service/ops/verify-providers.sh` **5/5**, incl. the named
> checkpoint: EL TTS refused with 401 "missing the permission
> text_to_speech"). **3.2 ✅ complete** — the Firestore store is implemented
> and proven against the emulator (transactional invite race → exactly
> MaxUses wins; live service smoke incl. the doc-edited kill switch flipping
> `/keys` to 403 with no restart). **3.3 ◑ prepared** — `resend.go` verified
> against current docs (no drift), full-stack `ops/smoke-resend.sh` ready;
> the live smoke + domain decision need the owner-created Resend account.
> **3.4 ✅ deployed** — live on Cloud Run at
> `https://mogi-access-zdz7y265mq-uc.a.run.app` (Firestore + Secret Manager +
> real OTP mail), full prod smoke green, kill-switch drill passed, and the
> service left **dormant** until a real OpenRouter provisioning key replaces
> the stub minter. **3.5 ✅** — the app now ships pointing at that URL
> (`wails build` green, verified in the binary, live client probe passed).
> Remaining: 3.6 (prod e2e + rotation drill), 3.7 (docs), 3.8 (cohort launch)
> — all gated on the OpenRouter Management key.

## Context

Testers redeem an **invite code + email OTP** and the app auto-inserts
developer-funded API keys (per-tester $3-capped OpenRouter key, shared
TTS/STT-restricted Google key, shared STT-scoped ElevenLabs key) fetched from a
small **access service**; **BYOK stays a first-class equal mode** via a
`KeyMode` preference over two key namespaces. Pinned model server-side, voice
prefs open but catalog-filtered, launch-time key refresh for rotation/enforcement,
~$20/mo budget. Dev-mode mailer first (log OTPs); the service lives in this repo
as its own Go module at `access-service/`.

Simplifying decisions (validated in design review):
1. **Extend `models.AuthStatus`** — the three existing `*Configured` bools become
   mode-aware, so existing gates work in managed mode unchanged. New fields:
   `keyMode`, `managedActive`, `managedEmail`, `pinnedModel`. Token never crosses
   the boundary.
2. **KeyMode is a Preferences field** switched via existing `UpdatePreferences`;
   `Settings.Update` re-resolves providers only when KeyMode actually changed.
3. **Only 3 new bindings:** `RequestTestCode`, `ActivateTestAccount`,
   `SignOutTestAccount`.
4. Sign-out is **device-local** in v1 (tester-doc revocation is the server lever).

## Wire contract (as built in Phase 0; Phase 1's client mirrors it)

| Endpoint | Request | Success | Failure |
|---|---|---|---|
| `POST /activate` | `{"email","inviteCode"}` | `204` | `400` invalid/exhausted invite (**no mail sent**); `403` phase off; `429` rate-limited |
| `POST /verify` | `{"email","code"}` | `200 {"token","keys":{openrouter,google,elevenlabs,pinnedModel}}` | `400` bad/expired code or ≥5 attempts |
| `GET /keys` | `Authorization: Bearer <token>` | `200 {openrouter,google,elevenlabs,pinnedModel}` | `401` bad token; `403` revoked / phase off |
| `GET /healthz` | — | `200 ok` | — |

Invite validation runs **before** any mail (spam-relay defense); the invite use
is committed once, on successful `/verify`.

---

## Phase 0 — Access service, standalone and local-first — ✅ Done

Runs with `STORE=memory MAILER=log` (OTPs to the log, stub key minter) — zero
GCP/Resend/OpenRouter setup. Files under `access-service/` (`module mogi-access`).

- ✅ **0.1 Verify OpenRouter provisioning.** Resolved from OpenRouter's public
  docs (a live curl needs the owner's provisioning key). **Findings:** the raw
  key is returned **only at mint** (`POST /api/v1/keys/`; later reads mask it as
  `label`+`hash`); there is **no model-restriction field**; delete is
  `DELETE /api/v1/keys/{hash}`. → `Tester` stores `ORKey`+`ORKeyHash`; `Mint`
  takes no model param. **Residual (non-blocking):** confirm `limit` is USD on
  the first real mint (OpenRouter credits are 1:1 USD).
  > **Checkpoint ✓** — decisions locked before code; the store/minter shapes
  > below already reflect them.
- ✅ **0.2 Module scaffold.** `access-service/go.mod` (`module mogi-access`,
  `go 1.25.0`, zero external deps) + `main.go` (env `config`, `loadConfig`,
  graceful-shutdown `run`, `buildHandler`, `getenv*` helpers).
  > **Checkpoint ✓** — root `go build ./... && go test ./...` unchanged/green
  > (nested module doesn't descend into or pollute `mogi` — premise proven);
  > service builds; gofmt clean. All 13 env vars map to a real consumer.
- ✅ **0.3 Store interface + memory impl.** `internal/store/store.go`
  (`Invite`/`OTP`/`Tester`/`Session`/`Config`, `ErrNotFound`, **added
  `ErrInviteUnavailable`** for `ConsumeInvite`'s inactive/exhausted case, `Store`
  interface) + `memory.go` (mutex-guarded maps, `NewMemory` seeds a 100-use
  invite + config).
  > **Checkpoint ✓** — walked all 3 handlers against the interface: every method
  > has a caller, nothing missing. `ConsumeInvite` = single-doc read-modify-write
  > → maps cleanly to a Firestore transaction later.
- ✅ **0.4 Mailer.** `internal/mailer/`: `Mailer` interface, `LogMailer` (dev),
  `Resend` (prod, updater-style request idiom — **built now, exercised in 3.3**).
- ✅ **0.5 Provisioning client + stub.** `internal/openrouter/`: `Client`
  (`Mint`/`Delete`, `ai/client.go` idiom), `KeyMinter` interface, `StubMinter`
  (`sk-or-fake-…`) for keyless local runs.
  > **Checkpoint ✓ (0.4+0.5)** — both HTTP clients share the
  > `NewRequestWithContext` + wrapped-error idiom; both `Client` and `StubMinter`
  > satisfy `KeyMinter` (server stays oblivious). `Delete` kept as the documented
  > rotation-ops hook (implements the 3.6 revoke-by-hash requirement, justifies
  > storing `ORKeyHash`); revisit if no admin surface materializes.
- ✅ **0.6 HTTP server.** `internal/server/server.go` (`New`, `handleActivate`
  with the load-bearing **invite→rate-limit→mail** order, `handleVerify`,
  `handleKeys`, `GET /healthz`; helpers `normalizeEmail`/`hashString`/`genOTP`
  (unbiased crypto/rand)/`genToken`/`bearerToken`/`clientIP`; one `keysPayload`
  reused flat by `/keys` and nested by `/verify`) + `ratelimit.go` (sliding
  window). **Refinement discovered here:** the `/verify` request carries only
  `{email, code}`, so `ConsumeInvite` needs the invite code from elsewhere →
  **added `InviteCode` to the `OTP` struct** (the OTP *is* "a pending activation
  for this email via this invite").
  > **Checkpoint ✓** — all 10 store methods called, none unused; identical keys
  > JSON in both endpoints; `TestPhaseActive` enforced in both `/activate` and
  > `/keys`. `main.go` wiring updated (placeholder health check folded into the
  > server).
- ✅ **0.7 Handler tests.** `server_test.go` (white-box `package server`) over
  `NewMemory` + recording fake mailer + fake minter: bad-invite-no-mail, OTP
  round trip (+ email normalization), expiry, attempts-exhausted, invite-consumed-
  once (+ key reused on re-verify), rate-limit, keys valid/invalid token, revoked
  →403, phase-off→403 (activate **and** keys).
  > **Checkpoint ✓** — `go test ./... && go vet ./...` green; every wire-contract
  > row asserts **status and body shape** (these tests are the contract the
  > Phase 1 client is written against).
- ✅ **0.8 Local smoke + README.** Live run confirmed: activate→`204` (OTP
  logged) · bad invite→`400` (no mail for stranger) · verify→`200` (token +
  stub key + pinned model) · keys→`200` (identical payload) · bad token→`401`.
  `access-service/README.md` written (run command, endpoint table, env table,
  curl flow, deploy command, ops-notes stub).
  > **Phase gate ✓** — root suite unchanged/green; service build+test+vet+gofmt
  > green; no premature one-file packages (`ratelimit.go` stayed in `server/`).
  > **As-built notes:** `main.go` is 201 lines (over the ~100 estimate) but is
  > all config/wiring/plumbing, no logic. `mailer/resend.go`, `openrouter.Delete`,
  > and the Firestore seam are deliberately built-but-unexercised Phase 3 hooks.

**Built layout:**
```
access-service/
  go.mod  main.go  README.md
  internal/store/     store.go  memory.go
  internal/mailer/    mailer.go  log.go  resend.go
  internal/openrouter/ provisioning.go  stub.go
  internal/server/    server.go  ratelimit.go  server_test.go
```

---

## Phase 1 — App backend (store → client → service → bindings) — ✅ Done

Dependency-ordered; no step references a later artifact. Built exactly as
planned except for the small as-built refinements noted per step.

- ✅ **1.1 Managed store rows.** New `internal/store/managed.go`: consts
  (`managed_openrouter_api_key`, …, `managed_session_token`, `managed_email`,
  `managed_pinned_model`), `managedProviderKey()` mirroring `providerKey()`,
  methods on the existing `getPref/setPref/deletePref` primitives (**no schema
  change**): `GetManagedKey/SetManagedKey`, `GetManagedSession/SetManagedSession`,
  `GetManagedPinnedModel/SetManagedPinnedModel`, `DeleteManagedData()`. Extended
  `ClearAll`'s doc comment (wipe includes managed rows → full reset signs out).
  > **Checkpoint ✓** — build+gofmt clean; error-wrapping matches
  > `preferences.go`; `DeleteManagedData` loops `deletePref` (no raw SQL).
  > **As-built:** also added `GetManagedEmail/SetManagedEmail` — the plan
  > declared the `managed_email` const but omitted its accessor; it's read by
  > `AuthStatus` (1.3) and written by `Activate` (1.5), so a getter/setter pair
  > mirroring the session/pinned-model ones was required.
- ✅ **1.2 `Preferences.KeyMode`.** `models/session.go`: `KeyMode string
  \`json:"keyMode"\`` (`"byok"` default | `"managed"`); `preferences.go`: const
  `keyKeyMode`, default in `GetPreferences`, overlay read (guarded `v != ""`, so
  a stray empty write still reads back `"byok"`), one `setPref` in
  `SavePreferences`.
- ✅ **1.3 Mode-aware AuthStatus.** models: added `keyMode/managedActive/
  managedEmail/pinnedModel`. `settings.go`: `SettingsStore` gained the managed
  getters; `AuthStatus()` picks `GetAPIKey` vs `GetManagedKey` by mode and fills
  the new fields. Extended `fakeStore`; added `TestAuthStatusManagedMode`.
  > **Checkpoint (1.2+1.3) ✓** — build+test+gofmt green; `ListModels` guards on
  > the registry (unaffected); `KeyMode` defaulted in exactly one place.
  > **As-built:** `managedActive/managedEmail/pinnedModel` are reported
  > **regardless of mode** (they read the managed rows, not the active
  > namespace) — deliberately, so a user who flipped to BYOK without signing out
  > still shows as managed-active for the Phase 2 "switch back" affordance.
- ✅ **1.4 `internal/access` client.** `client.go`: `const DefaultURL`
  (placeholder → real in 3.5); `Client{baseURL, httpClient(15s)}`, `NewClient`
  (trims trailing `/`); `KeySet{OpenRouter,Google,ElevenLabs,PinnedModel}`;
  `RequestCode`/`Verify`/`Keys` over one private `do` helper; non-2xx decodes
  `{"error"}`; 401/403 wrap sentinel `ErrUnauthorized` preserving the server
  message. Added `client_test.go` (httptest) asserting the three bodies incl.
  `/verify`'s `"keys"` nesting and the 401/403→`ErrUnauthorized` wrap.
  > **Checkpoint ✓** — field-by-field diff of bodies vs the Phase 0 server; one
  > sentinel suffices. **As-built:** the sentinel is wrapped as
  > `fmt.Errorf("%w: %s", ErrUnauthorized, msg)`; the account service recovers
  > the server message for the sign-out notice by trimming that prefix
  > (`signOutNotice`).
- ✅ **1.5 Account service.** New `internal/service/account.go`: `AccountStore`,
  `AccessClient` interface (`*access.Client` satisfies), `Account{store,
  providers, client, status func() models.AuthStatus}` (status borrowed from
  `Settings.AuthStatus`, like `NewHistory` borrows `interview.ActiveID`).
  `RequestCode`; `Activate` (Verify→store keys+pin+session+email→KeyMode="managed"→
  `ApplyMode`→status); `SignOut` (local: `DeleteManagedData`, KeyMode="byok",
  re-resolve); `Refresh(ctx) (changed, notice, err)` — no-token no-op / 200 upsert
  + re-resolve / `ErrUnauthorized` purge+notice / other-error keep cached keys.
  Package-level `applyKeyMode(st, providers, prefs)` (over a tiny `keyResolver`
  interface) = the single resolution rule. Tests via `accountWith(...)` +
  `fakeAccess` over a stateful store (`statefulStore`).
  > **Checkpoint ✓** — no status logic duplicated; service→access coupling
  > matches the `AI`→`ai.ChatMessage` precedent; `ApplyMode` = `applyKeyMode` +
  > prefs-read + logging, and every mutating path (Activate/SignOut/Refresh) ends
  > by calling it. **As-built:** `Activate`/`SignOut` **return `models.AuthStatus`**
  > (not `void`) — the borrowed `status` func is right there, so the bindings hand
  > the fresh status straight back (Phase 2 can use it directly instead of a
  > follow-up `GetAuthStatus`). `Refresh`'s `changed` is precise: `true` on the
  > sign-out path, and on a 200 only when the **pinned model actually moved** (the
  > sole refresh-visible field), so launches don't emit a no-op event.
- ✅ **1.6 Settings chokepoints.** `settings.go`: (a) `Update` compares old vs new
  prefs; **only on KeyMode change** calls `applyKeyMode`. (b) `SetAPIKey`/
  `DeleteAPIKey`: always write the store, touch `providers.SetKey` only when
  `!managedMode()`. (c) `ClearAllData` comment extended (managed sign-out).
  Tests: `TestUpdateKeyModeFlipReresolves` (incl. a non-flip Update leaving the
  registry untouched), `TestSetAPIKeyInManagedModeLeavesRegistry`,
  `TestClearAllSignsOutManaged`.
  > **Checkpoint ✓** — every write path into keys/KeyMode (`SetAPIKey`,
  > `DeleteAPIKey`, `Update`, `ClearAllData`, `Account.Activate/SignOut/Refresh`,
  > `ApplyMode`) ends with registry ≡ store+mode; that invariant is
  > `applyKeyMode`'s doc comment; a non-flip `Update` touches providers zero
  > times (asserted).
- ✅ **1.7 Model pinning.** `interview.go`: `InterviewStore` gained
  `GetManagedPinnedModel`; one helper `resolveModel(requested, prefs)` (managed→
  pinned; else requested; else `prefs.Model`) at the three chokepoints (`Start`,
  `startCompanyInterview`, `extractSessionMeta` fallback). Debrief unchanged
  (frozen session model). `TestResolveModelPinning` (table): pinned wins managed
  (over a request), empty pin degrades to request, explicit wins byok, saved
  default otherwise.
  > **Checkpoint ✓** — every session-creating `prefs.Model` read flows through
  > `resolveModel`; no inline `if prefs.KeyMode` at call sites.
- ✅ **1.8 Voice guards.** `voice.go`: `managedGoogleVoiceAllowed(id)` (contains
  "neural2"/"wavenet"); `activeTTS` — managed forces Google (STT-scoped EL key
  would 4xx) and falls back to `defaultGoogleVoiceID` when the saved voice fails
  the allowlist; `Voices()` — managed filters the catalog
  (`filterManagedGoogleVoices`). `activeSTT` untouched. Tests: managed forces
  Google despite `TTSProvider=elevenlabs`, errors (no EL fallback) with no Google
  key, premium saved voice falls back, catalog filter drops Chirp/Studio.
  > **Checkpoint ✓** — `activeTTS` reads mode from its existing `GetPreferences`
  > call; default `en-US-Neural2-F` passes; backend guard is the source of truth
  > (Phase 2 tile-hiding is mirror-only). **As-built:** `Voices()` does one extra
  > (cheap, local) `GetPreferences` read for the filter branch — `activeTTS`
  > doesn't surface the mode, and `GetPreferences` is already called liberally
  > across the service layer.
- ✅ **1.9 Wiring, bindings, launch refresh.** `app.go`: `App.account`; in
  `NewApp`, `MOGI_ACCESS_URL` env (default `access.DefaultURL`) →
  `service.NewAccount(...)`; **replaced the 3-provider load loop with
  `app.account.ApplyMode()`** (loop deleted). `startup`: `go
  a.refreshManagedAccount()` (20s ctx, log, emit on change). Three 1–3-line
  bindings — `RequestTestCode`/`ActivateTestAccount`/`SignOutTestAccount` (the
  latter two return `models.AuthStatus`). `window.go`: `emitManagedChanged(notice)`
  → `EventsEmit(a.ctx, "managed:changed", notice)`.
  > **Checkpoint ✓** — `go build/vet/gofmt` green; `grep -l "pkg/runtime" *.go`
  > → only `window.go`; old loop confirmed gone. **As-built:** with no managed
  > state, `ApplyMode` resolves the BYOK namespace — byte-identical to the old
  > loop's effect (the loop's per-key read-error warning is the only dropped
  > behaviour; a bad read now just leaves that slot empty, same as before).
- ✅ **1.10 Regenerate + bridge.** Ran `wails generate module` (regenerated
  `frontend/wailsjs`, incl. the 3 bindings + the new `AuthStatus`/`Preferences`
  fields); added the 3 methods to `wailsBridge.ts`.
  > **Phase gate — automated ✓:** root `go build/test/vet` + `gofmt -l` clean,
  > access-service suite green, `npx tsc --noEmit` clean. **Interactive e2e ☐
  > (deferred):** the local-service + `MOGI_ACCESS_URL=… wails dev` devtools drive
  > (`RequestTestCode`/`ActivateTestAccount` → `GetAuthStatus()` shows managed)
  > needs a running access service + native window and has **not** been run yet.
  > It can run standalone before Phase 2, or fold into the 2.6 full local e2e.

## Phase 2 — Frontend — ✅ Done

- ✅ **2.1 App shell plumbing.** `App.tsx`: `handleAuthChange` (setAuthStatus +
  `loadPrefs`) passed to SetupPage/Settings (stale-prefs guard); mount-scoped
  `EventsOn("managed:changed", …)` → refetch AuthStatus + prefs, surface `notice`.
  Built per the 1.9 as-built note: `handleAuthChange(status?)` takes an optional
  status — button handlers pass the value Activate/SignOut return (only
  `loadPrefs()` then needs a round trip); the `managed:changed` listener (no
  status in hand) refetches. One setter, no redundant fetch.
  > **Checkpoint ✓** — tsc clean; `handleAuthChange` is the only status setter
  > passed down; listener drives the existing dismissible error banner and was
  > verified in the stubbed preview: a notice surfaces + exactly
  > `GetAuthStatus`/`GetPreferences` refetch; an empty notice (rotation/re-pin)
  > refetches silently; the subscription survives StrictMode's
  > mount→cleanup→remount (unsubscribe path exercised).
  > **As-built:** the subscribe is wrapped in try/catch — unlike bound Go calls
  > (which *reject*), `EventsOn` **throws synchronously** when the Wails runtime
  > is absent, and an unguarded mount-scoped subscribe crashed the whole tree in
  > the plain-browser preview. (The `ptt:down` listener never hit this: it's
  > gated behind prefs, which never load unstubbed.)
- ✅ **2.2 InviteActivation component.** New `components/setup/InviteActivation.tsx`
  + CSS: phase machine `request | verify | done`, email/invite/OTP fields,
  privacy-notice checkbox gating the request, `RequestTestCode`/
  `ActivateTestAccount`, `onActivated`/`onBack`. Reuses `.btn*`.
  > **Checkpoint ✓** — tsc clean; full flow driven in a stubbed browser preview
  > (both themes): consent checkbox gates the request even with both fields
  > filled; invalid-invite and invalid-OTP rejections render inline (staying on
  > their phase); the OTP field strips non-digits and caps at 6, with Activate
  > enabled only at exactly 6; success hands the fresh `AuthStatus` to
  > `onActivated`. Host-agnostic as required for 2.4.
  > **As-built:** `onBack` is optional — the request step renders a Back button
  > only when the host provides one; the verify step's Back is internal (returns
  > to request keeping the typed email/invite, which also serves as the resend
  > path). On success the component calls `onActivated(status)` right after
  > entering `done` — the terminal success card is just what shows until the
  > host re-renders. Fields follow the SetupPage credential idiom (uppercase
  > label + underlined mono input) via self-contained `invite-*` classes.
- ✅ **2.3 SetupPage two-door fork.** Door state `choose | invite | byok`; managed
  signed-in pre-pass card; invite door hosts `InviteActivation`; byok door =
  existing form untouched + back link; gate "Already configured" hints on
  `keyMode !== "managed"`.
  > **Checkpoint ✓ (stubbed preview)** — every UI flow driven with auth-scenario
  > stubs: fresh → chooser (two doors) → invite door hosts `InviteActivation`
  > (Back roundtrips) → activate → pre-pass card supersedes the component's
  > `done` state → Continue → Hub with Start enabled; relaunch-as-managed lands
  > on the card directly (no doors, no form); relaunch-as-configured-BYOK lands
  > **directly on the byok form, byte-identical** (original header, all three
  > "Already configured" hints, "Keys Configured" check state, Continue enabled)
  > plus the back link; byok ↔ chooser roundtrip. tsc clean. **The live
  > `wails dev` drive (fresh DB → OTP from the service log) folds into the 2.6
  > battery, alongside 1.10's still-open interactive gate.**
  > **As-built:** setup lands by state — BYOK users with saved keys go straight
  > to the byok door (today's relaunch UX, no extra click), fresh installs get
  > the chooser; the pre-pass card gates on `keyMode === "managed"` **computed
  > per render**, so a launch-refresh sign-out mid-setup drops back to the
  > doors. `InviteActivation.onActivated` wires straight to `onAuthChange`
  > (2.1's single setter). The card reuses `setup-card--success` for the glow
  > and points account management at Settings → API Keys (2.4). Mode-switching
  > is deliberately *not* offered from the card — that's Settings' job.
- ✅ **2.4 Settings account card + fork + invite entry.** New
  `components/settings/ManagedAccountCard.tsx` + CSS (email, badge, pinned-model,
  **Switch to my own keys** via `savePrefs({keyMode:"byok"})`, **Sign out** via
  `SignOutTestAccount`). `Settings.tsx`: `api-keys` pane renders the card when
  managed, else `ApiKeysSection`. `ApiKeysSection` (byok view): "Switch back"
  banner when `managedActive`; "Have an invite?" footer hosting inline
  `<InviteActivation>`.
  > **Checkpoint ✓ (stubbed preview, stateful auth stub)** — activate (from the
  > inline footer) → card, signed in as the new email; switch to byok → the
  > three key cards + banner ("Signed in as …, using your own keys"), with the
  > pre-existing BYOK OpenRouter key **Configured** and EL/Google bare (namespace
  > separation visible); switch back → card, same email (managed rows intact);
  > sign out → banner gone, **BYOK key still Configured**, invite footer
  > reappears. Instrumented the mode switch: exactly
  > `UpdatePreferences{keyMode}` → `GetAuthStatus` → `GetPreferences` — no other
  > mutation path. Both themes; tsc clean. (True relaunch persistence is
  > backend behavior — 1.6's unit tests — and lands in the 2.6 battery.)
  > **As-built:** the Settings **shell** owns the three account flows
  > (`switchKeyMode`, `handleSignOut`, `handleActivated`) because sign-out and
  > activation change the store-side KeyMode *under* the shell's local prefs
  > mirror — each flow re-seeds it (`seedFromPrefs`, the `handleDataCleared`
  > recovery), otherwise the next unrelated `savePrefs` would write a stale
  > KeyMode back. `Settings`' `onAuthChange` prop widened to
  > `(status?) => void` to match 2.1's setter (bare call = refetch after a mode
  > flip). The invite footer is hidden while `managedActive` (the banner is the
  > affordance then) and collapses to a single row until opened. Sign-out has
  > no confirm step: it's device-local and recoverable — re-verifying reuses
  > the tester and mints no new invite use (0.7). New `ApiKeysSection.css`
  > (the section previously had no stylesheet of its own).
- ✅ **2.5 Pinned model + voice in Settings.** `ModelsSection`: `authStatus`
  prop; managed → static locked card (lock icon + `pinnedModel`) instead of
  `ModelPicker`. `VoiceSection`: managed → google-only tiles + `resolveProvider()
  → "google"` + BYOK-only note. `VoicePicker` unchanged (backend filters).
  > **Checkpoint ✓** — tsc clean; stubbed preview, both modes and themes:
  > managed → lock card (lock icon + pinned-model chip + hint deep-linking to
  > API Keys) with the picker gone, single Google tile **selected even with
  > `ttsProvider: "elevenlabs"` saved** (the forced resolution), BYOK-only note,
  > filtered 3-voice catalog; byok → full picker + both tiles + 5-voice catalog
  > incl. Chirp/Studio. Both branch on `authStatus.keyMode`, not prefs. The
  > lock card was re-confirmed **live** in the 2.6 battery with the real
  > server pin. (The no-Chirp/Studio managed catalog is backend behavior —
  > 1.8's unit tests; with stub keys the live `ListVoices` can't succeed.)
  > **As-built:** the lock card reuses `.settings-card-placeholder`; only the
  > pinned-id chip needed CSS (new `ModelsSection.css`). ModelsSection gained
  > `onOpenApiKeys` (same deep-link idiom as VoiceSection's placeholder) since
  > API Keys owns mode switching. VoiceSection filters `VOICE_PROVIDERS` down
  > to a `providerTiles` list when managed — the premium tile is dropped, not
  > rendered dead — and the note reuses `settings-hint settings-hint-muted`.
  > `ListVoices()` takes **no provider arg** (the backend resolves + filters);
  > the picker's `provider` prop is only its refetch trigger.
- ✅ **2.6 Full local e2e (pre-GCP).** memory/log service + `MOGI_ACCESS_URL=…
  wails dev`: redeem → keys land, model pinned, Scribe STT + forced-Google TTS;
  kill switch (`TEST_PHASE_ACTIVE=false` restart → graceful sign-out); BYOK
  regression; sign-out leaves BYOK keys; offline grace (stop service → cached
  keys work); Clear All Data → signed out.
  > **Phase gate ✓ — full battery passed (2026-07-16)**, driven through the
  > `wails dev` dev server (`:34115`, bindings over the dev bridge) against
  > `STORE=memory MAILER=log` on `:8787` with a distinctive
  > `PINNED_MODEL=anthropic/claude-sonnet-4.5`:
  > - **Redeem (2.3's live drive):** fresh DB → chooser → invite door → OTP
  >   from the service log → activate → signed-in card; all three managed rows
  >   + session token + email + pin landed in SQLite; AuthStatus reported
  >   managed/3×configured/pinned.
  > - **Voice resolution:** with deliberately fake keys, error provenance
  >   proves routing — `SynthesizeSpeech` failed **at Google** (`googletts:
  >   … 400`, despite an installed EL key) and `TranscribeAudio` failed **at
  >   ElevenLabs Scribe** (`voice: ElevenLabs STT … 401`) = forced-Google TTS +
  >   Scribe-preferred STT. (Real-key success is 3.1's checkpoint.)
  > - **Kill switch:** phase-off proven live on the wire (`POST /activate` →
  >   `403 "the test phase is not currently active"`); app relaunch → launch
  >   refresh rejected → managed rows purged, `key_mode` flipped to byok,
  >   **BYOK key untouched**. *Memory-store caveat:* restarting the service
  >   also wipes sessions, so the refresh rejection observed live is the 401
  >   invalid-token flavor — same `ErrUnauthorized` purge path; the
  >   403-phase-off `/keys` row is asserted by 0.7's handler tests.
  > - **BYOK regression + namespace separation:** switch-to-own-keys → three
  >   bare key cards + banner; saved a BYOK OpenRouter key (Configured); both
  >   `openrouter_api_key` and `managed_openrouter_api_key` side by side in
  >   the DB; switch-back round trip; explicit sign-out removed every
  >   `managed_*` row, kept the BYOK key, invite footer returned.
  > - **Offline grace:** service stopped → relaunch logged `account: launch
  >   key refresh failed, using cached keys: … connection refused`, every
  >   managed row intact, app landed on the signed-in card.
  > - **Clear All Data:** CONFIRM modal → AuthStatus fully reset, tables empty.
  > - **1.10's deferred gate closed** in its original form: console-driven
  >   `RequestTestCode` → OTP from log → `ActivateTestAccount` → returned
  >   AuthStatus showed managed.
  >
  > Whole-feature recount: 3 bindings, 2 new components; only the deliberate
  > Phase 3 hooks (Resend mailer, provisioning `Delete`, Firestore seam)
  > remain unexercised.
  > **As-built notes:** (1) a fresh-DB e2e must move **both** data dirs aside —
  > with `~/Library/Application Support/mogi` absent, `appDataDir`'s legacy
  > migration silently renames an old `ai-interviewer/` dir into place and its
  > keys read as configured. (2) In an external browser the Wails dev page
  > needs real typed keystrokes; synthetic `input.value` writes don't reach
  > React state. (3) `wails dev` + the OS keyboard hook logs an
  > Accessibility-permission warning in headless-ish drives — harmless here.

## Phase 3 — Deploy, ops, docs — ☐ Not started

- ✅ **3.1 Provider verification + shared keys** (verify-items 2+3): EL key
  STT-only + cap (**confirm the cap binds Scribe's dollar billing**); GCP key
  restricted to TTS+STT, pin quota knobs, budget alerts $10/$20.
  > **Checkpoint:** TTS with Google key succeeds; TTS with EL key **fails**
  > (STT-scoping proven — what makes the 1.8 guard correctness).
  > **GCP half ✓ (2026-07-18), verified live. Findings:**
  > - **Verify-item 3 resolved.** TTS exposes **only request-rate knobs** (per-
  >   project + per-voice-family RPM, default 1000/min) — there is **no
  >   character/day or spend quota**, so a low RPM cap is TTS's only real-time
  >   dollar brake. STT v1 does have a true daily knob:
  >   `AudiosecondsRequestsPerDayPerProject`, default 1,728,000 audio-s/day
  >   (480 h!). Budget alerts lag billing by hours — they are detection, not
  >   enforcement; enforcement = the quota caps + kill switch + rotation.
  > - **Quotas pinned** (Cloud Quotas preferences, all granted immediately):
  >   TTS `RequestsPerMinutePerProject` 1000→**10** (5 testers × 2 replies/min
  >   fits; throttles a leaked key to ~$48/h even at max-size Neural2 requests),
  >   STT `AudiosecondsRequestsPerDayPerProject` 1,728,000→**7,200** (2 h of
  >   audio/day cohort-wide ≈ single-digit $/day worst case), STT
  >   `DefaultRequestsPerMinutePerProject` 900→**60**.
  > - **Shared key minted:** `mogi-managed-voice` in `ai-interviewer-500220`,
  >   API-restricted to `texttospeech`+`speech`; key string lives only in
  >   gitignored `access-service/.env` (`GOOGLE_SHARED_KEY`; → Secret Manager
  >   in 3.4). Deliberately separate from the personal dev key ("API key 1")
  >   so either rotates/revokes without touching the other.
  > - **Budget:** `mogi-ai-interviewer-monthly` — $20/mo on the project's
  >   billing account filtered to this project, thresholds 50% ($10) + 100%
  >   ($20), default email recipients (billing admins).
  > - **Checkpoint, Google half PASSED** via `access-service/ops/
  >   verify-providers.sh` (mirrors the app's exact request bodies from
  >   `internal/googletts` / `internal/voice`): Neural2 synthesize → 200;
  >   LINEAR16 round-trip through STT → 200 with the synthesized phrase back
  >   verbatim; Translate with the same key → **403 blocked** (restriction
  >   binds).
  > - **Verify-item 2 resolved (docs level).** `speech_to_text` is a standalone
  >   per-key permission in the create-key API's enum → an STT-only key is
  >   constructible. Whether the per-key cap binds Scribe depends on the
  >   workspace's billing regime: on **subscription (credit) plans**, credits
  >   are shared across every product and Scribe draws them (≈330 credits per
  >   audio-minute), so a per-key monthly credit cap **does bind STT**; on
  >   **usage-based API billing**, Scribe bills $0.22/h in dollars and the
  >   guard is keeping the prepaid balance small — exactly the fallback the
  >   plan anticipated. The dashboard's key-create modal shows which regime
  >   applies.
  > **EL half ✓ (2026-07-18).** STT-only key minted in the dashboard and
  > dropped into `access-service/.env`; `ops/verify-providers.sh` now passes
  > **5/5**. The named checkpoint closed with the ideal proof: EL TTS → **401**
  > `"The API key you used is missing the permission text_to_speech to execute
  > this operation"` — a *scope* refusal, not a balance/auth accident — and
  > Scribe STT returned the round-trip phrase verbatim. The 1.8
  > forced-Google-TTS guard is thereby a correctness rule, not a preference.
- ✅ **3.2 Firestore impl.** `access-service/internal/store/firestore.go` (deps in
  the service module only); `ConsumeInvite` via `RunTransaction`; config from a
  `config/config` doc; wire `STORE=firestore`.
  > **Checkpoint ✓ (2026-07-18)** — memory suite green with **zero interface
  > change** (`store.go` untouched); root module untouched (`go.mod`/`go.sum`
  > clean, root build+tests green). Proven beyond the gate against the
  > **Firestore emulator**: 5 integration tests in `firestore_test.go` (they
  > skip without `FIRESTORE_EMULATOR_HOST`, keeping the normal suite
  > cloud-free), incl. a 10-goroutine race on a 3-use invite → **exactly 3
  > wins, final Uses 3** (the transaction guarantee 0.3 predicted),
  > microsecond timestamp round-trip fidelity, idempotent OTP delete, and
  > missing `config/config` → loud error. Then a **live service smoke**
  > (`STORE=firestore GCP_PROJECT=… go run .` against the emulator):
  > activate→verify→keys green with the pinned model coming back **from the
  > seeded doc** (a distinctive value, not the env default — config provably
  > reads Firestore), invite `Uses` incremented to exactly 1, and the kill
  > switch flipped **by editing the doc while the service ran** → `/keys` 403
  > immediately, no restart — the console-editability the design promised.
  > **As-built:** (1) one collection per record type (`invites`/`otps`/
  > `testers`/`sessions`, keyed by the memory maps' natural keys) plus
  > `config/config`; structs stored under their Go field names (no tags) so
  > the Firestore console matches `store.go` one-to-one. (2) Missing
  > `config/config` fails loudly — and effectively closed — rather than
  > inventing defaults. (3) No `Close`: the client lives for the process
  > lifetime (the Store interface has no teardown; Cloud Run reaps it).
  > (4) **The swap surfaced a latent handler conflation:** with an infallible
  > memory store, "not found" and "backend down" were indistinguishable — on
  > Firestore, an infra blip would have 401'd `/keys` (the app purges managed
  > keys on 401/403 ⇒ spurious device sign-out) or fallen into `/verify`'s
  > first-timer path (burning an invite use and overwriting the tester's
  > minted key). Handlers now branch on `errors.Is(err, store.ErrNotFound)`
  > at every read site and answer infra errors with a logged 500 (new
  > `internalError` helper — 5xx paths previously logged nothing, which on
  > Cloud Run means no trace at all); the wire contract is unchanged and the
  > app already treats 5xx as "keep cached keys" (the offline-grace path).
  > Pinned by two new handler tests over an error-injecting store wrapper:
  > outage ≠ sign-out (same token works after recovery), outage ≠ mint
  > (minter uncalled, invite unburned).
- ✅ **3.3 Resend** (verify-item 4): smoke via `onboarding@resend.dev`; verify
  domain (SPF/DKIM), set `MAIL_FROM`, test spam placement. GitHub device flow is
  the documented fallback.
  > **Done (2026-07-18):** `trymogi.dev` bought + verified in Resend, real OTP
  > mail from `otp@trymogi.dev` lands in the Gmail inbox. Two optional polish
  > items (DMARC TXT, corporate-inbox spot-check) are noted below but don't
  > gate deploy.
  > **Prepared (2026-07-18); the live half is blocked on the Resend account
  > (owner-only, I cannot create accounts).** `resend.go` verified against the
  > current API docs — endpoint, Bearer auth, `"Name <addr>"` from,
  > `to`-as-array, `text` body all match: no drift, zero code change. Facts
  > that shape the rest (verify-item 4, docs level): the sandbox sender
  > (`onboarding@resend.dev`) needs no domain but delivers **only to the
  > account owner's own address** — smoke-only, useless for testers; real
  > cohort mail needs a verified domain (SPF TXT + feedback MX on a `send.`
  > subdomain + DKIM TXT — the dashboard lists the exact records; subdomain
  > sending recommended over apex) or the device-flow fallback; the free tier
  > (100/day, 3,000/mo, 1 domain, 10 req/s) sits far above the service's own
  > /activate rate limits. New `ops/smoke-resend.sh` runs the smoke
  > **full-stack** — boots the service with `MAILER=resend` and drives
  > `/activate`, so the mail goes through the production mailer path (204 ⇒
  > Resend accepted our message); the same script re-runs for the
  > verified-domain and spam-placement checks.
  > **Sandbox smoke ✓ (2026-07-18):** with the owner-created key,
  > `ops/smoke-resend.sh zyang3104@gmail.com` → `/activate` **204** — Resend
  > accepted the OTP mail built by the production mailer path. The first run
  > was accidentally instructive: its recipient defaulted to `git config
  > user.email` (a different address), and Resend answered 403 *"You can only
  > send testing emails to your own email address"* — the sandbox restriction
  > **proven on the wire** (and surfaced verbatim by 3.2's `internalError`
  > logging). The script now prints its recipient and hints on that 403.
  > **Placement ✓ (2026-07-18):** the sandbox OTP landed in the Gmail
  > **inbox**, not spam (owner-confirmed) — good baseline deliverability.
  > The bare smoke also now defaults its recipient to `RESEND_ACCOUNT_EMAIL`
  > from `.env` (the git identity was a different address — the cause of two
  > confusing sandbox 403s).
  > **Decision (2026-07-18): buy a dedicated domain** (owner-chosen over the
  > device-flow fallback). Availability verified via RDAP (whois was
  > unreliable — the macOS client stopped at the IANA root): `mogi.dev`,
  > `mogi.app`, `getmogi.com`, `mogiapp.com` taken; **`trymogi.dev`,
  > `trymogi.app`, `trymogi.com`, `getmogi.dev`, `usemogi.com` available**
  > (recommendation: `trymogi.dev`). Because the domain will exist solely for
  > the app, sending from the root (`otp@<domain>`) is fine — Resend's
  > "use a subdomain" advice protects domains that also carry other mail, and
  > Resend puts the SPF/bounce records on `send.<domain>` regardless.
  > **Domain live (2026-07-18):** `trymogi.dev` bought (Cloudflare DNS),
  > added + **verified in Resend**; SPF TXT + feedback MX on
  > `send.trymogi.dev` and the DKIM key all resolve publicly (checked
  > authoritative + 1.1.1.1 + 8.8.8.8 — an early empty read was propagation
  > racing, minutes-scale). `MAIL_FROM="Mogi <otp@trymogi.dev>"` set in
  > `.env`; real-domain smokes **accepted (204) to both Gmail and the UNC
  > corporate address** — the latter doubles as the verification proof, since
  > the sandbox had refused that exact recipient. (Also fixed a smoke-script
  > env quirk: the `${MAIL_FROM:+…}` inline-prefix word-split a
  > `"Name <addr>"` value; the sourced env is exported, so the prefix was
  > removed.)
  > **Placement ✓ (2026-07-18):** real-domain OTP landed in the **Gmail
  > inbox** (owner-confirmed) from `otp@trymogi.dev` — a verified domain
  > reaching a major provider's inbox is the deliverability bar 3.3 exists to
  > clear. The functional requirement (testers receive OTPs) is met.
  > **Optional polish (non-blocking, does not gate 3.4):** (1) add the
  > `_dmarc` TXT (`v=DMARC1; p=none`) in Cloudflare — still absent in DNS as
  > of this write; recommended hygiene, and some strict corporate filters
  > weight it. (2) Spot-check the UNC/corporate inbox + Gmail "Show original"
  > SPF/DKIM verdicts if convenient. Neither blocks deploy.
- ✅ **3.4 Deploy.** Secrets → Secret Manager; `gcloud run deploy mogi-access
  --source access-service --max-instances 1 …`; `roles/datastore.user`; seed
  `config/config` + first invite; re-run the smoke curl against prod.
  > **Checkpoint (3.3+3.4) ✓ (2026-07-19)** — all three conditions met: prod
  > flow green **including real OTP mail** from `otp@trymogi.dev`; no key
  > material in tracked files; `--max-instances 1` confirmed on the live
  > revision. **Live URL:** `https://mogi-access-zdz7y265mq-uc.a.run.app`
  > (3.5 sets `access.DefaultURL` to this).
  > **As-built:**
  > - **Everything in `us-central1`** — Firestore Native DB co-located with
  >   Cloud Run (Firestore's location is permanent; single-region is cheaper
  >   than multi-region and this service is low-QPS).
  > - **A `Dockerfile` rather than buildpacks** (multi-stage → static
  >   `CGO_ENABLED=0` binary on `distroless/static:nonroot`): deterministic,
  >   tiny, and immune to buildpack Go-version drift (the module is `go
  >   1.25.0`). Added `.dockerignore` + `.gcloudignore` that **exclude `.env`**
  >   so the real keys can never enter the build context or image — the
  >   secret-hygiene-critical detail of this step.
  > - **Env vars deliberately omit `TEST_PHASE_ACTIVE` and `PINNED_MODEL`.**
  >   In `STORE=firestore` those are read from the `config/config` doc; setting
  >   them as env would imply an authority they don't have. Only
  >   `STORE`/`MAILER`/`GCP_PROJECT`/`MAIL_FROM` are env; the three provider
  >   keys mount from Secret Manager (`mogi-google-shared-key`,
  >   `mogi-elevenlabs-shared-key`, `mogi-resend-api-key`).
  > - **IAM** on the runtime (compute default) SA: `roles/datastore.user`,
  >   `roles/secretmanager.secretAccessor`, `roles/cloudbuild.builds.builder`.
  > - **New `ops/seed-firestore.sh`** — `show` / `config <bool> [model]` /
  >   `invite <CODE> [max-uses]`, the scriptable form of the console-editable
  >   state (reused by 3.8's invite minting).
  > **Prod smoke results:** bad invite → 400 (no mail); real invite → 204 **+
  > real email delivered**; OTP persisted to Firestore as a *hash* keyed by
  > email hash carrying its invite code (the 0.6 privacy design, visible in
  > prod); verify → 200 with token + keys, the **Google/ElevenLabs values
  > arriving from Secret Manager** and `pinnedModel` from the config doc;
  > `/keys` → 200 with the identical payload shape; garbage token → 401;
  > invite consumed **exactly once** (1/5), 1 tester + 1 session written, OTP
  > doc deleted after use.
  > **Kill-switch drill ✓ (prod, live):** flipping `TestPhaseActive=false` in
  > the config doc — **no redeploy** — immediately turned `/keys` into 403
  > ("test phase has ended", the app's graceful sign-out) and `/activate` into
  > 403. **The service is deliberately left in this dormant state** until the
  > OpenRouter key lands (below), so the deployed stack cannot hand anyone
  > stub keys.
  > **Known quirk (not a service defect):** `/healthz` is unreachable *from
  > the dev machine* — Cloud Run's request log proves those requests never
  > arrive, while `/` and `/activate` do (and return our Go mux's 404 and our
  > handler's JSON 400 respectively). Something on the local network hijacks
  > that path; `ops/smoke-resend.sh` is unaffected because it probes its own
  > localhost service.
  > **Remaining ☐ before testers (the 3.6 gate): a real
  > `OPENROUTER_PROVISIONING_KEY`.** The service is running the **stub
  > minter**, so activation currently returns a fake `sk-or-…` key. Fix is one
  > secret + one redeploy (see the README's "Going live" note), then flip the
  > kill switch back on.
- ✅ **3.5 Point the app at prod.** Set `access.DefaultURL` to the Cloud Run URL;
  `go build ./... && wails build`.
  > **Done (2026-07-19).** `access.DefaultURL` →
  > `https://mogi-access-zdz7y265mq-uc.a.run.app` (placeholder retired). Root
  > `go build`/`vet`/`gofmt`/`go test` green; **full `wails build` green** —
  > `Mogi.app` packaged and self-signed. Verified the constant survives into
  > the shipped artifact: `strings` on the built binary finds the Cloud Run URL
  > and **zero** occurrences of the old `mogi-access.example.com`.
  > **Live probe:** a throwaway `internal/access` test drove the **real client
  > against the real deployed service** (then was deleted): a bogus token came
  > back as the `ErrUnauthorized` sentinel — precisely the signal
  > `Account.Refresh` keys its purge-and-sign-out path off — and a bad invite
  > surfaced the server's own message. So the client↔service contract holds
  > over the network, not just against `httptest`.
  > **As-built:** `DefaultURL`'s comment now records *why* the runtime knobs
  > live in Firestore rather than beside it — changing this constant needs a
  > new app release, whereas the kill switch and pinned model must be
  > changeable in seconds.
- ☐ **3.6 Prod e2e + drills.** Repeat 2.6 against prod. Drills: kill switch,
  rotation, per-tester revocation (`revoked:true` + delete OR key by hash).
  > **🐞 3.6 caught a production-breaking bug before any tester could.**
  > `internal/openrouter`'s `keysURL` carried a **trailing slash** (0.1 recorded
  > it as `POST /api/v1/keys/`). Against the live API that path returns **404**
  > on POST — so *every* activation would have failed at mint time, after the
  > OTP was already spent and the invite use already consumed. (GET on the same
  > path 301-redirects, which Go silently downgrades to a GET — a mint would
  > have become a key *list*.) Fixed: `keysURL` has no trailing slash, `Delete`
  > appends `"/"+hash`. `baseURL` became a `Client` field purely so tests can
  > point at an `httptest` server; new `provisioning_test.go` pins the path,
  > method, auth header, and `{name, limit}` body for both calls, plus the
  > error path. **This also closes 0.1's open residual:** a live mint returned
  > `limit` exactly as sent, confirming it is USD and a *ceiling* — the probe
  > key was deleted immediately (`DELETE → 200`, proving the revocation hook
  > works too).
  > **Real minting verified end-to-end in prod ✓ (2026-07-19).** After the fix
  > shipped (rev `00003-5k7`), a fresh activation on the genuine first-time
  > path produced: a real `sk-or-v1-…` key (73 chars, **not** the stub's
  > `sk-or-fake-…`); OpenRouter showing it **named for the tester's email with
  > a $3 limit, $0 used**; invite `Uses` 1→2; the tester doc carrying the real
  > `ORKeyHash`. Decisive check — **a live inference call with that tester key
  > on the pinned model returned "managed key works"**, so the artefact a
  > tester actually receives is proven functional, not merely well-formed.
  > **Two of the three drills are already done** (out of order, while building
  > the 3.4 ops tooling): the **kill switch** was proven live in 3.4, and
  > **per-tester revocation** was proven live on 2026-07-19 —
  > `ops/seed-firestore.sh revoke <email>` → `/keys` returns 403 *"this test
  > account has been deactivated"*, with the tester's `ORKeyHash` preserved for
  > the paired OpenRouter delete. Remaining for 3.6: the full app-side e2e and
  > the **shared-key rotation** drill.
  > **App-level prod e2e ✓ (2026-07-19)** — the real app (`wails dev`, fresh
  > DB, both data dirs moved aside) against the **live Cloud Run service**:
  > fresh install → two-door chooser → invite door → real OTP email → activate
  > → **"You're signed in"** → Hub showing **SYSTEM READY / Start Session
  > enabled with zero key setup** (the managed tier's whole point). Local
  > SQLite held all six `managed_*` rows incl. the **real 73-char
  > `sk-or-v1-` key**, `key_mode=managed`, and an **empty BYOK namespace**
  > (separation visible). Settings → API Keys rendered the account card
  > (email, ACTIVE, pinned model, switch/sign-out); Models showed the **locked
  > pinned-model card** carrying the value from the prod config doc; Voice
  > showed **Google-only tiles + the BYOK-only note and 51 voices** — a live
  > `ListVoices` through the shared Google key, so that key is proven working
  > *from inside the app*, not just from curl.
  > **Kill switch, app-level ✓ (the flagship drill):** flipping
  > `TestPhaseActive=false` and relaunching drove the launch refresh to a 403
  > → **every `managed_*` row purged, `key_mode` back to `byok`**, and the UI
  > landed on the setup chooser — a graceful sign-out, no error state, no
  > crash.
  > **☐ Not covered by this run** (deferred, lower value): BYOK-regression
  > round trip, offline grace, and Clear-All-Data — all three passed in the
  > 2.6 local battery against identical backend code paths, and the
  > namespace separation they protect was re-observed above.
  > **🐞 Unrelated crash found:** `wails dev` **SIGSEGV**s at startup inside
  > the global hotkey listener — `gohook.End()` under cgo,
  > `internal/hotkey/listener.go:212` — on Go 1.25.5 / gohook v0.42.3 without
  > Accessibility permission. 2.6 had recorded this as a harmless *warning*;
  > it is now fatal, so it is a regression in its own right (it would hit any
  > dev running `wails dev` unprivileged). Worked around for this e2e by
  > pre-setting `push_to_talk_enabled=0` in the fresh DB. **Not a
  > managed-keys defect — needs its own fix.**
  > **Ops tooling as-built:** `seed-firestore.sh` was rewritten to
  > verb-per-command (`on`/`off`/`model`/`invite`/`disable-invite`/`testers`/
  > **Rotation drill ◑ (2026-07-19).** Google shared key rotated: replacement
  > `mogi-managed-voice-v2` minted with the same two API restrictions, added
  > as **secret version 2**, and independently proven by
  > `ops/verify-providers.sh` **5/5** (TTS, STT round-trip, and a
  > non-allowlisted API still 403 — same posture as v1). **Key operational
  > finding: a new secret version does NOT reach a running instance.** Cloud
  > Run resolves `:latest` at *container start*, so the warm instance kept
  > serving v1 throughout (re-checked twice). Rotating a **leaked** key
  > therefore requires forcing a new revision — never assume the rotation is
  > live. Remaining: redeploy (or let the instance idle out), confirm `/keys`
  > serves v2, then delete the v1 key (`gcloud services api-keys delete`).
  > `revoke`/`unrevoke`/`show`) and every mutation now carries an
  > `updateMask`. The first cut used bare Firestore PATCHes, which **replace
  > the whole document** — `config false` silently rewrote `PinnedModel` to the
  > script's default, and a naive `revoke` would have wiped the tester's minted
  > `ORKey`. New `ops/openrouter-keys.sh` (`list`/`delete <hash>`) exposes the
  > provisioning `Delete` that 0.5 built as an ops hook and 3.6 needs.
- ☐ **3.7 Docs.** `roadmap.md` (status), `architecture.md` (3 bindings,
  `managed:changed` event, access-service data-flow note), `CLAUDE.md` (map
  rows), flip `managed-keys-plan.md` status to implemented.
- ☐ **3.8 Cohort launch.** Mint invites, confirm budget alerts, weekly-ops glance
  list into the service README.

## Hard ordering dependencies

1. Phase 0 before 1.4 (handler tests are the executable contract the client
   mirrors). **[0 done]**
2. In Phase 1: 1.1→1.2→1.3; 1.4→1.5→1.6; everything before 1.10
   (`wails generate module` once). **[1 done]**
3. 1.10 before all of Phase 2 (bindings + regenerated models gate TSX). **[1.10
   fully done — its interactive `wails dev` smoke ran inside the 2.6 battery.]**
4. 2.1 before 2.2–2.5 (`handleAuthChange` is the staleness guard).
5. 3.1/3.2/3.3 → 3.4 → 3.5/3.6; testers only after 3.6.

## Verification

- Per step: named checkpoint commands (root `go build ./... && go test ./... &&
  gofmt -l .`; `cd access-service && go test ./...`; `cd frontend && npx tsc
  --noEmit`; `wails generate module` after Go surface changes).
- Phase gates: **0.8 curl smoke ✓**; **1.10 ✓ (automated: Go
  build/test/vet/gofmt, access-service suite, `npx tsc --noEmit`; its
  console-driven backend e2e ran inside 2.6)**; **2.6 full local e2e ✓ (kill
  switch, offline grace, BYOK regression, ClearAll — see the 2.6 gate notes)**;
  **3.1 checkpoint ✓ (2026-07-18, `access-service/ops/verify-providers.sh`
  5/5 — Google TTS + STT round trip + restriction 403; EL TTS 401
  missing-permission, Scribe 200; rerun it after minting or rotating either
  shared key)**; **3.2 ✓ (memory suite green, zero interface change, root
  module untouched; plus emulator integration tests + a live
  `STORE=firestore` service smoke with a doc-driven kill-switch flip)**;
  3.6 prod e2e + revocation/rotation drills ☐.

## Critical files

- `access-service/internal/server/server.go` ✅ + `internal/access/client.go` ✅
  — the two sides of the wire contract
- `access-service/internal/store/firestore.go` ✅ — the production store
  (transactional invite consume, console-editable `config/config` doc)
- `internal/service/account.go` ✅ — activation/refresh/sign-out + the
  `applyKeyMode` invariant
- `internal/service/settings.go` ✅ — mode-aware AuthStatus + the three chokepoints
- `internal/store/managed.go` ✅ — managed namespace on existing pref primitives
- `internal/service/interview.go` / `voice.go` ✅ — `resolveModel` + TTS/catalog guards
- `app.go` / `window.go` ✅ — wiring, 3 bindings, startup refresh, `managed:changed`
- `frontend/src/…` — `lib/wailsBridge.ts` ✅ (3 methods exported); `App.tsx` ✅
  (`handleAuthChange` + `managed:changed` listener); `InviteActivation` ✅;
  SetupPage fork ✅; ManagedAccountCard + Settings fork + ApiKeysSection
  banner/invite-footer ✅; ModelsSection lock card + VoiceSection managed
  guards ✅ (2.5)
