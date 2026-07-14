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

## Deploy (Phase 3 — not yet wired)

```bash
gcloud run deploy mogi-access --source . --region us-central1 \
  --allow-unauthenticated --max-instances 1 \
  --set-env-vars STORE=firestore,MAILER=resend,PINNED_MODEL=... \
  --set-secrets RESEND_API_KEY=...,OPENROUTER_PROVISIONING_KEY=...,GOOGLE_SHARED_KEY=...,ELEVENLABS_SHARED_KEY=...
```

`--max-instances 1` keeps the in-memory rate limiter globally coherent (free at
this scale). Secrets live in Secret Manager, never in this repo (it is public).

## Ops notes

Filled in during Phase 3 (provider verification, quota knobs, revocation and
rotation drills, the weekly usage-glance list). Revocation of one tester is
`revoked: true` on their Firestore doc plus deleting their OpenRouter key by its
stored hash (`openrouter.Client.Delete`, or the OpenRouter dashboard).
