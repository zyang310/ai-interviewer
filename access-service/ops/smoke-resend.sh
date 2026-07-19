#!/usr/bin/env bash
# smoke-resend.sh — Phase 3.3 OTP-mail smoke, full-stack on purpose.
#
# Boots the REAL service with MAILER=resend (memory store) and drives
# POST /activate, so the OTP email goes through internal/mailer/resend.go —
# the same code path production uses. A 204 from /activate therefore means
# Resend accepted the message our mailer built; the human half of the check is
# reading the inbox (and the spam folder — note where it landed).
#
#   ./smoke-resend.sh [to-address]     # default: git config user.email
#
# Reads RESEND_API_KEY (required) and MAIL_FROM (optional) from
# access-service/.env or the environment. Three intended runs:
#   1. Sandbox smoke: MAIL_FROM unset ⇒ "Mogi <onboarding@resend.dev>" —
#      Resend delivers this ONLY to the account owner's own address.
#   2. Verified-domain smoke: set MAIL_FROM="Mogi <otp@send.yourdomain>" after
#      DNS verification and re-run.
#   3. Spam placement: run against a Gmail and an Outlook address you control
#      and check headers (SPF/DKIM/DMARC pass) + folder placement.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
env_file="$here/../.env"
if [[ -f "$env_file" ]]; then
  # shellcheck disable=SC1090
  set -a; source "$env_file"; set +a
fi

if [[ -z "${RESEND_API_KEY:-}" ]]; then
  echo "RESEND_API_KEY is not set (add it to access-service/.env)." >&2
  echo "Create one at resend.com → API Keys, scope 'Sending access'." >&2
  exit 1
fi

# Recipient: explicit arg > RESEND_ACCOUNT_EMAIL (.env) > git email. The .env
# entry exists because the sandbox sender only delivers to the account owner,
# and the git identity is often a different address.
to="${1:-${RESEND_ACCOUNT_EMAIL:-$(git config user.email || true)}}"
if [[ -z "$to" ]]; then
  echo "usage: $0 <to-address>" >&2
  exit 1
fi
echo "recipient: $to (with the sandbox sender this must be the Resend account owner's address)"

port=8793
log="$(mktemp)"
cleanup() {
  pids="$(lsof -ti ":$port" 2>/dev/null || true)"
  [[ -n "$pids" ]] && kill $pids 2>/dev/null
  rm -f "$log"
}
trap cleanup EXIT

echo "starting service (MAILER=resend, from: ${MAIL_FROM:-Mogi <onboarding@resend.dev>}) ..."
# MAIL_FROM and RESEND_API_KEY are already exported (set -a + source above, or
# the caller's environment) — the service inherits them.
(
  cd "$here/.." &&
    PORT=$port STORE=memory MAILER=resend DEV_INVITE_CODE=MOGI-DEV \
    go run . >"$log" 2>&1
) &

if ! curl -s --retry 30 --retry-connrefused --retry-delay 1 --max-time 60 \
  "http://127.0.0.1:$port/healthz" >/dev/null 2>&1; then
  echo "FAIL  service did not come up on :$port. Log:" >&2
  tail -5 "$log" >&2
  exit 1
fi

code=$(curl -sS -o /dev/null -w "%{http_code}" -X POST "http://127.0.0.1:$port/activate" \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"$to\",\"inviteCode\":\"MOGI-DEV\"}")

if [[ "$code" == 204 ]]; then
  echo "PASS  /activate -> 204: Resend accepted the OTP mail for $to"
  echo "      Now check that inbox (AND the spam folder — note where it landed)."
  if [[ "${MAIL_FROM:-onboarding@resend.dev}" == *resend.dev* ]]; then
    echo "      Sandbox sender: Resend only delivers this to the account owner's address."
  fi
else
  echo "FAIL  /activate -> $code. Service log:"
  tail -5 "$log"
  if grep -q "only send testing emails to your own email address" "$log"; then
    echo "hint: with the sandbox sender, re-run as: $0 <your-resend-account-email>"
  fi
  exit 1
fi
