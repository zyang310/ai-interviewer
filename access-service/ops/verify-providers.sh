#!/usr/bin/env bash
# verify-providers.sh — Phase 3.1 checkpoint for the shared managed-tier keys.
#
# Proves the two shared voice keys have exactly the powers they should:
#   Google key (GOOGLE_SHARED_KEY, API-restricted to TTS+STT):
#     1. TTS synthesize succeeds (managed default voice, en-US-Neural2-F)
#     2. STT recognize succeeds — on audio round-tripped from TTS
#     3. a NON-restricted Google API is blocked (restriction is real)
#   ElevenLabs key (ELEVENLABS_SHARED_KEY, scoped speech_to_text only):
#     4. TTS FAILS (STT-only scoping proven — this is what makes the app's
#        managed forced-Google-TTS guard a correctness rule, not a preference)
#     5. Scribe STT succeeds (same round-tripped audio)
#
# Requests mirror internal/googletts/client.go and internal/voice/client.go
# byte-for-byte (same endpoints, models, encodings), so a PASS here means the
# app's own calls will behave the same. Reused for the 3.6 prod drills and
# after any key rotation.
#
# Keys are read from access-service/.env (or the environment). Nothing secret
# is ever printed. Needs: bash, curl, python3.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
env_file="$here/../.env"
if [[ -f "$env_file" ]]; then
  # shellcheck disable=SC1090
  set -a; source "$env_file"; set +a
fi

GOOGLE_SHARED_KEY="${GOOGLE_SHARED_KEY:-}"
ELEVENLABS_SHARED_KEY="${ELEVENLABS_SHARED_KEY:-}"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

pass=0 fail=0 pend=0
ok()      { echo "  PASS  $1"; pass=$((pass+1)); }
bad()     { echo "  FAIL  $1"; fail=$((fail+1)); }
pending() { echo "  PEND  $1"; pend=$((pend+1)); }

# http_code FILE URL [curl args...] — status code to stdout, body to FILE.
http_code() {
  local out="$1" url="$2"; shift 2
  curl -sS -o "$out" -w "%{http_code}" "$url" "$@"
}

phrase="the managed keys are verified"

echo "== Google shared key (restricted to TTS+STT) =="
if [[ -z "$GOOGLE_SHARED_KEY" ]]; then
  pending "GOOGLE_SHARED_KEY not set — skipping Google checks"
else
  # 1. TTS synthesize, MP3, managed default voice (same body the app sends).
  code=$(http_code "$tmp/tts.json" \
    "https://texttospeech.googleapis.com/v1/text:synthesize?key=$GOOGLE_SHARED_KEY" \
    -H 'Content-Type: application/json' -d '{
      "input": {"text": "'"$phrase"'"},
      "voice": {"languageCode": "en-US", "name": "en-US-Neural2-F"},
      "audioConfig": {"audioEncoding": "MP3"}}')
  if [[ "$code" == 200 ]] && grep -q audioContent "$tmp/tts.json"; then
    ok "Google TTS synthesize (Neural2) -> 200 + audio"
  else
    bad "Google TTS synthesize -> $code: $(head -c 300 "$tmp/tts.json")"
  fi

  # 2. Round-trip: synthesize LINEAR16 @16kHz (the app's STT input format),
  #    then recognize it with the same config the app uses.
  code=$(http_code "$tmp/wav.json" \
    "https://texttospeech.googleapis.com/v1/text:synthesize?key=$GOOGLE_SHARED_KEY" \
    -H 'Content-Type: application/json' -d '{
      "input": {"text": "'"$phrase"'"},
      "voice": {"languageCode": "en-US", "name": "en-US-Neural2-F"},
      "audioConfig": {"audioEncoding": "LINEAR16", "sampleRateHertz": 16000}}')
  if [[ "$code" != 200 ]]; then
    bad "Google TTS (LINEAR16 for round-trip) -> $code"
  else
    python3 - "$tmp/wav.json" "$tmp/audio.wav" <<'PY'
import base64, json, sys
body = json.load(open(sys.argv[1]))
open(sys.argv[2], "wb").write(base64.b64decode(body["audioContent"]))
PY
    python3 - "$tmp/audio.wav" > "$tmp/stt_req.json" <<'PY'
import base64, json, sys
print(json.dumps({
    "config": {"encoding": "LINEAR16", "sampleRateHertz": 16000,
               "languageCode": "en-US", "model": "latest_long",
               "useEnhanced": True, "enableAutomaticPunctuation": True},
    "audio": {"content": base64.b64encode(open(sys.argv[1], "rb").read()).decode()}}))
PY
    code=$(http_code "$tmp/stt.json" \
      "https://speech.googleapis.com/v1/speech:recognize?key=$GOOGLE_SHARED_KEY" \
      -H 'Content-Type: application/json' -d @"$tmp/stt_req.json")
    text=$(python3 -c 'import json,sys;d=json.load(open(sys.argv[1]));print(" ".join(a["alternatives"][0]["transcript"] for a in d.get("results",[]) if a.get("alternatives")).strip())' "$tmp/stt.json" 2>/dev/null || true)
    if [[ "$code" == 200 && -n "$text" ]]; then
      ok "Google STT recognize -> 200, transcript: \"$text\""
    else
      bad "Google STT recognize -> $code: $(head -c 300 "$tmp/stt.json")"
    fi
  fi

  # 3. Restriction proof: an API outside the allowlist must be blocked.
  code=$(http_code "$tmp/blocked.json" \
    "https://translation.googleapis.com/language/translate/v2/languages?key=$GOOGLE_SHARED_KEY&target=en")
  if [[ "$code" == 403 ]]; then
    ok "Non-restricted API (Translate) -> 403 blocked (key restriction binds)"
  else
    bad "Non-restricted API (Translate) -> $code (expected 403): $(head -c 200 "$tmp/blocked.json")"
  fi
fi

echo "== ElevenLabs shared key (scoped speech_to_text only) =="
if [[ -z "$ELEVENLABS_SHARED_KEY" ]]; then
  pending "ELEVENLABS_SHARED_KEY not set — create the STT-only key in the dashboard, paste into access-service/.env, re-run"
else
  # 4. THE 3.1 CHECKPOINT: TTS with the STT-scoped key must FAIL.
  code=$(http_code "$tmp/eltts.json" \
    "https://api.elevenlabs.io/v1/text-to-speech/21m00Tcm4TlvDq8ikWAM" \
    -H 'Content-Type: application/json' -H "xi-api-key: $ELEVENLABS_SHARED_KEY" \
    -d '{"text": "'"$phrase"'", "model_id": "eleven_flash_v2_5"}')
  if [[ "$code" == 401 || "$code" == 403 ]]; then
    ok "ElevenLabs TTS -> $code refused (STT-only scope proven): $(head -c 160 "$tmp/eltts.json")"
  else
    bad "ElevenLabs TTS -> $code (expected 401/403 — key is NOT STT-only!)"
  fi

  # 5. Scribe STT must succeed (this is the key's one job).
  if [[ -f "$tmp/audio.wav" ]]; then
    code=$(http_code "$tmp/elstt.json" \
      "https://api.elevenlabs.io/v1/speech-to-text" \
      -H "xi-api-key: $ELEVENLABS_SHARED_KEY" \
      -F "file=@$tmp/audio.wav;filename=audio.wav" \
      -F "model_id=scribe_v2" -F "tag_audio_events=false")
    text=$(python3 -c 'import json,sys;print(json.load(open(sys.argv[1])).get("text","").strip())' "$tmp/elstt.json" 2>/dev/null || true)
    if [[ "$code" == 200 && -n "$text" ]]; then
      ok "ElevenLabs Scribe STT -> 200, transcript: \"$text\""
    else
      bad "ElevenLabs Scribe STT -> $code: $(head -c 300 "$tmp/elstt.json")"
    fi
  else
    pending "Scribe STT skipped (no round-trip audio — Google TTS step must pass first)"
  fi
fi

echo
echo "verify-providers: $pass passed, $fail failed, $pend pending"
[[ $fail -eq 0 ]]
