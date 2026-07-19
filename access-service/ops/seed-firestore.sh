#!/usr/bin/env bash
# seed-firestore.sh — operate the live access service's Firestore state.
#
# The service reads its runtime config from a single config/config doc, its
# invites from the invites collection, and per-tester revocation from the
# testers collection — all editable without a redeploy. This script is the CLI
# for that (the Cloud console works too; this is scriptable and records the
# exact field shapes).
#
#   ./seed-firestore.sh show                  # everything at a glance
#   ./seed-firestore.sh on | off              # the kill switch
#   ./seed-firestore.sh model <model-id>      # change the pinned model
#   ./seed-firestore.sh invite <CODE> [uses]  # mint an invite (default 5 uses)
#   ./seed-firestore.sh disable-invite <CODE> # stop new redemptions of a code
#   ./seed-firestore.sh enable-invite <CODE>  # ...and turn it back on
#   ./seed-firestore.sh testers               # list testers + their key hashes
#   ./seed-firestore.sh revoke <email>        # cut off ONE tester
#   ./seed-firestore.sh unrevoke <email>
#   ./seed-firestore.sh forget-tester <email> # delete the record (re-activates fresh)
#
# Every mutation uses an updateMask so it touches exactly the field named —
# a bare Firestore PATCH replaces the whole document, which would silently wipe
# a tester's minted key or reset an invite's use count.
#
# Field names are the Go struct names (store.Config / store.Invite /
# store.Tester): the store persists structs without json tags, so the console
# shows exactly what store.go declares.
set -euo pipefail

PROJECT="${GCP_PROJECT:-ai-interviewer-500220}"
BASE="https://firestore.googleapis.com/v1/projects/$PROJECT/databases/(default)/documents"

api() { # method path-with-query [json-body]
  local method="$1" path="$2" body="${3:-}"
  local args=(-sS -X "$method" "$BASE/$path" -H "Authorization: Bearer $(gcloud auth print-access-token)")
  [[ -n "$body" ]] && args+=(-H 'Content-Type: application/json' -d "$body")
  curl "${args[@]}"
}

# setField DOC FIELD JSON-VALUE — update one field, leaving the rest intact.
setField() {
  api PATCH "$1?updateMask.fieldPaths=$2" "{\"fields\":{\"$2\":$3}}" \
    | python3 -c 'import json,sys; d=json.load(sys.stdin); print("  error:", d["error"]["message"][:80]) if "error" in d else print("  ok")'
}

# urlenc — escape an email for use as a document id path segment.
urlenc() { python3 -c 'import sys,urllib.parse; print(urllib.parse.quote(sys.argv[1], safe=""))' "$1"; }

case "${1:-show}" in
  on|off)
    [[ "$1" == on ]] && v=true || v=false
    echo "kill switch -> TestPhaseActive=$v"
    setField "config/config" "TestPhaseActive" "{\"booleanValue\":$v}"
    ;;
  model)
    m="${2:?usage: model <model-id>}"
    echo "pinned model -> $m"
    setField "config/config" "PinnedModel" "{\"stringValue\":\"$m\"}"
    ;;
  invite)
    code="${2:?usage: invite <CODE> [max-uses]}"; max="${3:-5}"
    # A full Set here is intentional: minting (re)initialises the whole doc.
    api PATCH "invites/$code" \
      "{\"fields\":{\"Code\":{\"stringValue\":\"$code\"},\"MaxUses\":{\"integerValue\":\"$max\"},\"Uses\":{\"integerValue\":\"0\"},\"Active\":{\"booleanValue\":true}}}" \
      >/dev/null && echo "minted invite $code ($max use(s), 0 used)"
    echo "NOTE: re-running this on an existing code RESETS its use count to 0."
    ;;
  disable-invite|enable-invite)
    code="${2:?usage: $1 <CODE>}"
    [[ "$1" == disable-invite ]] && v=false || v=true
    echo "$1 $code -> Active=$v"
    setField "invites/$code" "Active" "{\"booleanValue\":$v}"
    [[ "$v" == false ]] && echo "  No one new can redeem it. Testers who already activated keep working."
    ;;
  revoke|unrevoke)
    email="${2:?usage: $1 <email>}"
    [[ "$1" == revoke ]] && v=true || v=false
    echo "$1 $email -> Revoked=$v"
    setField "testers/$(urlenc "$email")" "Revoked" "{\"booleanValue\":$v}"
    [[ "$v" == true ]] && cat <<'EOF'
  Their app loses access on its next launch refresh (/keys -> 403).
  Their device still holds the cached OpenRouter key until then — to stop
  spending immediately, also delete the key: ops/openrouter-keys.sh delete <hash>
  (get the hash from: ops/seed-firestore.sh testers)
EOF
    ;;
  forget-tester)
    email="${2:?usage: forget-tester <email>}"
    echo "deleting tester record for $email"
    api DELETE "testers/$(urlenc "$email")" >/dev/null && echo "  ok"
    cat <<'EOF'
  They are now a first-time tester again: their next activation consumes an
  invite use and mints a NEW OpenRouter key. Their old key is NOT revoked by
  this — delete it too (ops/openrouter-keys.sh delete <hash>) or it keeps
  spending.
EOF
    ;;
  testers)
    api GET "testers" | python3 -c '
import json,sys
d = json.load(sys.stdin)
docs = d.get("documents") or []
if not docs: print("  (none)"); raise SystemExit
for doc in docs:
    f = doc["fields"]
    print("  %-32s revoked=%-5s invite=%-14s orKeyHash=%s" % (
        doc["name"].split("/")[-1],
        f.get("Revoked", {}).get("booleanValue", False),
        f.get("InviteCode", {}).get("stringValue", "?"),
        f.get("ORKeyHash", {}).get("stringValue", "") or "(stub)"))'
    ;;
  show)
    echo "=== config/config ==="
    api GET "config/config" | python3 -c '
import json,sys
d = json.load(sys.stdin)
if "error" in d: print("  (missing — seed it)")
else:
    f = d["fields"]
    on = f.get("TestPhaseActive",{}).get("booleanValue")
    print("  TestPhaseActive:", on, "" if on else "  <- KILL SWITCH IS OFF")
    print("  PinnedModel:    ", f.get("PinnedModel",{}).get("stringValue"))'
    echo "=== invites ==="
    api GET "invites" | python3 -c '
import json,sys
d = json.load(sys.stdin)
docs = d.get("documents") or []
if not docs: print("  (none)")
for doc in docs:
    f = doc["fields"]
    print("  %-20s uses %s/%s  active=%s" % (
        doc["name"].split("/")[-1],
        f.get("Uses",{}).get("integerValue","0"),
        f.get("MaxUses",{}).get("integerValue","?"),
        f.get("Active",{}).get("booleanValue")))'
    echo "=== testers ==="
    exec "$0" testers
    ;;
  *)
    sed -n '5,20p' "$0" | sed 's/^# \{0,1\}//'
    exit 1
    ;;
esac
