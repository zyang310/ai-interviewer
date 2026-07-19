# mogi-access

The Mogi **test-phase access service**. Testers redeem an **invite code + email
OTP**; in return the app receives developer-funded API keys and inserts them
itself, so nobody has to obtain their own OpenRouter / Google / ElevenLabs keys.

It **hands out keys**; it does **not** proxy interview traffic — screenshots and
audio still go straight from the app to the providers. Design rationale:
[../docs/managed-keys-plan.md](../docs/managed-keys-plan.md).

This is a **separate Go module** (`module mogi-access`) so its dependencies
never touch the root `mogi` app module.

## Run locally (zero cloud setup)

```bash
cd access-service
STORE=memory MAILER=log DEV_INVITE_CODE=MOGI-DEV go run .
```

- `STORE=memory` — in-process store, seeded with one 100-use invite (`MOGI-DEV`).
- `MAILER=log` — the OTP is **printed to the service log** instead of emailed,
  so the whole flow works with no domain and no Resend account.
- No `OPENROUTER_PROVISIONING_KEY` — a **stub minter** returns fake keys, so
  activation runs without spending real credits.

Point the desktop app at it with `MOGI_ACCESS_URL=http://localhost:8787` (added
in Phase 1).

## Endpoints (wire contract)

| Endpoint | Request | Success | Failure |
|---|---|---|---|
| `POST /activate` | `{"email","inviteCode"}` | `204` | `400` invalid/exhausted invite (no mail sent); `403` phase off; `429` rate-limited |
| `POST /verify` | `{"email","code"}` | `200 {"token","keys":{openrouter,google,elevenlabs,pinnedModel}}` | `400` bad/expired code or ≥5 attempts |
| `GET /keys` | `Authorization: Bearer <token>` | `200 {openrouter,google,elevenlabs,pinnedModel}` | `401` bad token; `403` revoked / phase off |
| `GET /healthz` | — | `200 ok` | — |

Invite validation happens **before** any mail is sent, so the endpoint can't be
used as a spam relay. The invite use is committed once, on successful `/verify`.

## Curl the flow

```bash
BASE=http://localhost:8787
# 1. Request a code (OTP appears in the service log with MAILER=log)
curl -sS -X POST "$BASE/activate" -H 'Content-Type: application/json' \
  -d '{"email":"me@example.com","inviteCode":"MOGI-DEV"}'
# 2. Verify (copy the code from the log) → returns a token + keys
curl -sS -X POST "$BASE/verify" -H 'Content-Type: application/json' \
  -d '{"email":"me@example.com","code":"123456"}'
# 3. Re-fetch keys with the token (the app does this on every launch)
curl -sS "$BASE/keys" -H "Authorization: Bearer <token-from-step-2>"
```

## Environment variables

| Var | Default | Purpose |
|---|---|---|
| `PORT` | `8787` | Listen port |
| `STORE` | `memory` | `memory` (dev) or `firestore` (prod, Phase 3.2) |
| `MAILER` | `log` | `log` (dev) or `resend` (prod) |
| `DEV_INVITE_CODE` | `MOGI-DEV` | Invite seeded in the memory store only |
| `TEST_PHASE_ACTIVE` | `true` | Kill switch default for the memory store |
| `PINNED_MODEL` | `google/gemini-2.5-flash` | Model id served to managed clients |
| `OPENROUTER_PROVISIONING_KEY` | — | Provisioning key; **unset ⇒ stub minter** |
| `OR_KEY_LIMIT_USD` | `3` | Per-tester OpenRouter credit cap |
| `GOOGLE_SHARED_KEY` | — | Shared TTS/STT-restricted Google key |
| `ELEVENLABS_SHARED_KEY` | — | Shared STT-scoped ElevenLabs key |
| `RESEND_API_KEY` | — | Resend key (when `MAILER=resend`) |
| `MAIL_FROM` | `Mogi <onboarding@resend.dev>` | OTP `From:` |
| `GCP_PROJECT` | — | GCP project (when `STORE=firestore`) |

In production, `TEST_PHASE_ACTIVE` and `PINNED_MODEL` are read from a Firestore
`config/config` doc (console-editable, no redeploy); the env values above seed
only the in-memory store.

## Test

```bash
go test ./...
```

Handler tests (`internal/server/server_test.go`) run against the memory store
with a recording fake mailer and a fake minter — no network, no cloud.

The Firestore store has its own integration tests
(`internal/store/firestore_test.go`) that run against the local emulator and
**skip automatically** when no emulator is configured, so the plain suite
stays cloud-free:

```bash
gcloud emulators firestore start --host-port=127.0.0.1:8790   # separate terminal
FIRESTORE_EMULATOR_HOST=127.0.0.1:8790 go test ./internal/store/
```

They cover the transactional invite consume (a concurrent race wins exactly
`MaxUses` times), round-trip fidelity for every record type, and the loud
failure on a missing `config/config` doc.

## Deploy (live)

Deployed to Cloud Run in `ai-interviewer-500220`, region `us-central1`:
**https://mogi-access-zdz7y265mq-uc.a.run.app**

```bash
# From the repo root. Re-run to ship changes (rolls out a new revision).
gcloud run deploy mogi-access \
  --source access-service --project ai-interviewer-500220 \
  --region us-central1 --allow-unauthenticated --max-instances 1 \
  --set-env-vars "^##^STORE=firestore##MAILER=resend##GCP_PROJECT=ai-interviewer-500220##MAIL_FROM=Mogi <otp@trymogi.dev>" \
  --set-secrets "GOOGLE_SHARED_KEY=mogi-google-shared-key:latest,ELEVENLABS_SHARED_KEY=mogi-elevenlabs-shared-key:latest,RESEND_API_KEY=mogi-resend-api-key:latest"
```

`--max-instances 1` keeps the in-memory rate limiter globally coherent (free at
this scale). Secrets live in Secret Manager, never in this repo (it is public);
`.gcloudignore` and `.dockerignore` exclude `.env` from the build context and
image. The custom `^##^` delimiter is required because `MAIL_FROM` contains a
space and an `@`.

**`TEST_PHASE_ACTIVE` and `PINNED_MODEL` are intentionally not env vars here** —
with `STORE=firestore` they come from the `config/config` doc so they can change
without a redeploy.

Runtime service account (`PROJECT_NUMBER-compute@developer.gserviceaccount.com`)
needs `roles/datastore.user`, `roles/secretmanager.secretAccessor`, and
`roles/cloudbuild.builds.builder`.

### Runbook — operating the live service

All commands run from `access-service/`. Everything here takes effect
**immediately, with no redeploy** (the service reads this state per request).

| Goal | Command |
|---|---|
| See everything at a glance | `ops/seed-firestore.sh show` |
| **Emergency stop — cut off everyone** | `ops/seed-firestore.sh off` |
| Turn the service back on | `ops/seed-firestore.sh on` |
| Change the model for all testers | `ops/seed-firestore.sh model google/gemini-2.5-flash` |
| Mint an invite | `ops/seed-firestore.sh invite MOGI-ABC123 5` |
| Stop new redemptions of a code | `ops/seed-firestore.sh disable-invite MOGI-ABC123` |
| List testers + their key hashes | `ops/seed-firestore.sh testers` |
| **Cut off one tester** | `ops/seed-firestore.sh revoke someone@example.com` |
| Reinstate a tester | `ops/seed-firestore.sh unrevoke someone@example.com` |
| See every minted key + its spend | `ops/openrouter-keys.sh list` |
| **Destroy one key right now** | `ops/openrouter-keys.sh delete <hash>` |

No laptop? The kill switch is one field in one document, editable from a phone:
[console.cloud.google.com → config/config](https://console.cloud.google.com/firestore/databases/-default-/data/panel/config/config?project=ai-interviewer-500220)
→ toggle `TestPhaseActive`.

### Killing access — three scopes

Pick the smallest one that solves the problem.

**1. One key (stops spending instantly).**
```bash
ops/seed-firestore.sh testers          # find the tester's orKeyHash
ops/openrouter-keys.sh delete <hash>   # the key dies at OpenRouter
```
Use when a key leaks or one tester is burning credits. Immediate and absolute:
the key stops working wherever it is, including on their machine.

**2. One tester (stops them re-fetching).**
```bash
ops/seed-firestore.sh revoke someone@example.com
```
`/keys` starts returning 403, so their app signs out on its next launch
refresh — managed keys purged, any personal BYOK keys untouched.

> **These two are different, and you usually want both.** Revoking flips a flag
> the *service* honours, but the tester's device already downloaded the key and
> keeps using it until its next launch refresh. Revoke = "no more service";
> delete = "the key itself is dead." To cut someone off *and* stop the spend,
> do both. Verified in prod: revoke → `/keys` 403 *"this test account has been
> deactivated"*, with the tester's stored key hash left intact for the delete.

**3. Everyone (the kill switch).**
```bash
ops/seed-firestore.sh off
```
`/activate` and `/keys` both 403 — no new signups, and every existing tester
signs out gracefully on next launch. Nothing is deleted, so `on` restores the
whole cohort. This is the "something is wrong, stop the bleeding" lever; the
per-key and per-tester tools are the scalpels.

**Rotating a shared key** (Google / ElevenLabs / Resend) is a different job —
mint a replacement, add a new secret version, redeploy. See *Shared managed
keys* below.

**Invites** are shared codes, not per-person tokens: one code admits up to
`MaxUses` distinct testers. `Uses` increments once per *new* tester on a
successful `/verify` — re-verifying an existing tester (a reinstall, a second
device) reuses their record and does **not** burn another use. To retire a
code, set `Active=false` on its doc in the console (or overwrite it with
`invite <CODE> 0`); existing testers keep working, since the gate is only
checked at activation.

Cost model worth remembering: minting a tester key costs **nothing** upfront.
Every key draws from the one OpenRouter account balance, and `OR_KEY_LIMIT_USD`
(default `3`) is a per-key *ceiling* — it caps the blast radius of any single
leaked or runaway key, it does not reserve or spend $3.

The kill switch is verified in prod: flipping `TestPhaseActive` turns `/keys`
into a 403 (the app signs out gracefully on its next launch refresh) and
`/activate` into a 403 — with no redeploy.

> **Note:** `curl`ing the deployed `/healthz` from the primary dev machine
> returns a Google 404 — those requests never reach Cloud Run (its request log
> shows `/` and `/activate` arriving but never `/healthz`), i.e. something on
> that local network hijacks the path. Use `/` (expect Go's
> `404 page not found`) as a liveness probe from there instead.

### Going live (remaining gate)

The service currently runs the **stub minter** — activation returns a fake
`sk-or-…` key — and is deliberately left with `TestPhaseActive=false`. Before
real testers:

```bash
# 1. Create a *Management API key* at openrouter.ai/settings/management-keys
#    (it can only manage other keys — it cannot make inference calls itself).
printf '%s' "$OR_PROVISIONING_KEY" | \
  gcloud secrets create mogi-openrouter-provisioning-key \
    --project=ai-interviewer-500220 --replication-policy=automatic --data-file=-
# 2. Add it to the deploy's --set-secrets as OPENROUTER_PROVISIONING_KEY=... and redeploy.
# 3. ops/seed-firestore.sh config true
```

## Ops notes

### Shared managed keys (Phase 3.1)

- **Google:** API key `mogi-managed-voice` in project `ai-interviewer-500220`,
  API-restricted to `texttospeech.googleapis.com` + `speech.googleapis.com`.
  Locally it lives in the gitignored `access-service/.env` as
  `GOOGLE_SHARED_KEY`; in prod it moves to Secret Manager (3.4). Deliberately a
  *separate key* from the developer's personal one so either can be rotated or
  revoked without touching the other.
- **ElevenLabs:** a key carrying **only** the `speech_to_text` permission
  (`mogi-managed-stt`), stored as `ELEVENLABS_SHARED_KEY`. Cap semantics depend
  on the workspace's billing regime: on subscription (credit) plans, credits
  are shared across every product and Scribe draws them (≈330 credits per
  audio-minute), so the per-key monthly credit cap **binds STT**; on
  usage-based API billing, Scribe bills $0.22/h in dollars and the guard is
  keeping the prepaid balance small (auto-recharge off).
- **Verify after minting or rotating either key:** `ops/verify-providers.sh` —
  asserts Google TTS 200, Google STT round-trip 200, non-restricted Google API
  403 (restriction binds), EL TTS refused (STT-only scope proven), Scribe 200.
  The same script is the 3.6 prod-drill tool.
- Local runs with real shared keys: `set -a; source .env; set +a` before
  `go run .` — the service reads plain env vars; `.env` is only a gitignored
  convenience file.

### OTP mail (Resend, Phase 3.3)

- **Client:** `internal/mailer/resend.go`, verified against the current API
  (POST `/emails`, Bearer auth, `from` as `"Name <addr>"`, `to` as array) —
  no drift, no code change needed.
- **Smoke:** `ops/smoke-resend.sh [to]` boots the real service with
  `MAILER=resend` and drives `/activate`, so the mail goes through the
  production mailer path — a 204 means Resend accepted the message we built.
  Three intended runs: sandbox now; again with `MAIL_FROM` set once the domain
  verifies; then against a Gmail **and** an Outlook address for spam placement
  (open the message headers, confirm SPF/DKIM/DMARC pass, note the folder).
- **Sandbox rule:** the default `Mogi <onboarding@resend.dev>` sender needs no
  domain but is delivered **only to the Resend account owner's own address** —
  fine for the smoke, useless for testers.
- **Domain verification:** dashboard → Domains → add the domain; it lists the
  exact records (SPF TXT + feedback MX on a `send.` subdomain, DKIM TXT at
  `resend._domainkey.`). Subdomain sending is recommended over the apex to
  isolate reputation; add a basic DMARC (`v=DMARC1; p=none`) TXT if the domain
  has none. Then set `MAIL_FROM="Mogi <otp@send.yourdomain>"` and re-run the
  smoke.
- **Limits vs need:** free tier is 100 mails/day, 3,000/mo, 1 domain, 10 req/s
  team-wide — far above the service's own `/activate` limits (3/email/hour,
  10/IP/hour) for a small cohort.
- **Fallback:** if no domain is available or placement is poor, the documented
  fallback is GitHub device-flow auth (no OTP mail at all) — a design change,
  not an ops tweak; decide before 3.4.

### Quota caps (GCP project `ai-interviewer-500220`)

Discovered in 3.1: **TTS has no character/day or spend quota — only
requests-per-minute knobs** — so a low RPM cap is TTS's only real-time dollar
brake. STT v1 does have a true daily-usage knob. Pinned via Cloud Quotas
preferences (decreases are self-service and granted immediately; raising back
up to the default is self-service too):

| Quota | Default | Pinned | Why |
|---|---|---|---|
| TTS `RequestsPerMinutePerProject` | 1000/min | **10/min** | ~5 testers × 2 replies/min fits; throttles a leaked key to ~$48/h even at max-size Neural2 requests |
| STT `AudiosecondsRequestsPerDayPerProject` | 1,728,000 s/day | **7,200 s/day** | 2 h of audio/day cohort-wide ≈ single-digit $/day worst case |
| STT `DefaultRequestsPerMinutePerProject` | 900/min | **60/min** | Utterance-sized sync requests; ample headroom |

View or change: `gcloud beta quotas preferences list
--project=ai-interviewer-500220`.

### Budget

`mogi-ai-interviewer-monthly`: **$20/month**, on the project's billing account
filtered to this project, email alerts at **$10 (50%)** and **$20 (100%)** to
the billing admins. Alerts lag real spend by hours (billing export) — they are
**detection, not enforcement**; enforcement is the quota caps, the kill switch,
and key rotation.

### Rotation / revocation drills

- **Google shared key:** mint a replacement with the same two API restrictions
  (`gcloud services api-keys create --api-target=service=texttospeech.googleapis.com
  --api-target=service=speech.googleapis.com …`) → update Secret Manager /
  `.env` → delete the old key (`gcloud services api-keys delete <uid>`) →
  `ops/verify-providers.sh`.
- **ElevenLabs shared key:** mint a new STT-only key in the dashboard, swap it
  in, delete the old one, re-verify.
- **One tester / one key:** see *Killing access — three scopes* above
  (`ops/seed-firestore.sh revoke` + `ops/openrouter-keys.sh delete`).
