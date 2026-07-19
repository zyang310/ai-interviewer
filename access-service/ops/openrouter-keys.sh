#!/usr/bin/env bash
# openrouter-keys.sh — inspect and revoke the per-tester OpenRouter keys the
# service mints.
#
#   ./openrouter-keys.sh list            # every minted key: name, limit, usage
#   ./openrouter-keys.sh delete <hash>   # revoke ONE key immediately
#
# This is the half of revocation that stops *spending*. Marking a tester
# Revoked in Firestore stops the service handing their key out again, but their
# device still holds the copy it already fetched — deleting the key here kills
# it at the source, instantly, wherever it is.
#
# Uses the Management (provisioning) key: from OPENROUTER_PROVISIONING_KEY in
# the environment or access-service/.env, else read from Secret Manager.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
env_file="$here/../.env"
if [[ -f "$env_file" ]]; then
  # shellcheck disable=SC1090
  set -a; source "$env_file"; set +a
fi

KEY="${OPENROUTER_PROVISIONING_KEY:-}"
if [[ -z "$KEY" ]]; then
  KEY=$(gcloud secrets versions access latest \
    --secret=mogi-openrouter-provisioning-key \
    --project="${GCP_PROJECT:-ai-interviewer-500220}" 2>/dev/null || true)
fi
if [[ -z "$KEY" ]]; then
  echo "No Management key found. Set OPENROUTER_PROVISIONING_KEY in access-service/.env," >&2
  echo "or create the mogi-openrouter-provisioning-key secret (see README 'Going live')." >&2
  exit 1
fi

API=https://openrouter.ai/api/v1/keys

case "${1:-list}" in
  list)
    curl -sS "$API" -H "Authorization: Bearer $KEY" | python3 -c '
import json,sys
d = json.load(sys.stdin)
rows = d.get("data") or []
if not rows: print("  (no keys minted yet)"); raise SystemExit
print("  %-28s %-8s %-8s %s" % ("NAME (tester email)", "LIMIT", "USED", "HASH"))
for k in rows:
    print("  %-28s $%-7s $%-7s %s" % (
        (k.get("name") or "?")[:28],
        k.get("limit", "-"),
        round(k.get("usage", 0), 4),
        k.get("hash", "")))'
    ;;
  delete)
    hash="${2:?usage: delete <hash>   (get it from: $0 list)}"
    code=$(curl -sS -o /tmp/or-del.txt -w "%{http_code}" -X DELETE "$API/$hash" -H "Authorization: Bearer $KEY")
    if [[ "$code" == 2* ]]; then
      echo "revoked key $hash — it stops working immediately, everywhere"
    else
      echo "FAILED ($code): $(head -c 200 /tmp/or-del.txt)" >&2; exit 1
    fi
    ;;
  *)
    echo "usage: $0 {list|delete <hash>}" >&2; exit 1
    ;;
esac
