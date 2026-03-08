#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:18080}"
WORKDIR="${WORKDIR:-/tmp/hexsonic-smoke}"
AUTH_TOKEN="${AUTH_TOKEN:-}"
AUTO_FETCH_TOKEN="${AUTO_FETCH_TOKEN:-true}"
KEYCLOAK_URL="${KEYCLOAK_URL:-http://localhost:18081}"
KEYCLOAK_REALM="${KEYCLOAK_REALM:-hexsonic}"
KEYCLOAK_CLIENT_ID="${KEYCLOAK_CLIENT_ID:-hexsonic-api}"
KEYCLOAK_CLIENT_SECRET="${KEYCLOAK_CLIENT_SECRET:-}"
KEYCLOAK_USER="${KEYCLOAK_USER:-}"
KEYCLOAK_PASS="${KEYCLOAK_PASS:-}"
mkdir -p "$WORKDIR"

curl -sf "$BASE_URL/healthz" > "$WORKDIR/health.json"

AUTH_ARGS=()
if [[ -z "$AUTH_TOKEN" && "$AUTO_FETCH_TOKEN" == "true" ]]; then
  if [[ -z "$KEYCLOAK_CLIENT_SECRET" || -z "$KEYCLOAK_USER" || -z "$KEYCLOAK_PASS" ]]; then
    echo "ERROR: set KEYCLOAK_CLIENT_SECRET, KEYCLOAK_USER and KEYCLOAK_PASS or provide AUTH_TOKEN"
    exit 1
  fi
  TOKEN_JSON=$(curl -sf -X POST "$KEYCLOAK_URL/realms/$KEYCLOAK_REALM/protocol/openid-connect/token" \
    -H "Host: keycloak:8080" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    -d "grant_type=password" \
    -d "client_id=$KEYCLOAK_CLIENT_ID" \
    -d "client_secret=$KEYCLOAK_CLIENT_SECRET" \
    -d "username=$KEYCLOAK_USER" \
    -d "password=$KEYCLOAK_PASS")
  AUTH_TOKEN=$(TOKEN_JSON="$TOKEN_JSON" python3 - << 'PY'
import json, os
print(json.loads(os.environ["TOKEN_JSON"]).get("access_token",""))
PY
)
fi
if [[ -n "$AUTH_TOKEN" ]]; then
  AUTH_ARGS=(-H "Authorization: Bearer $AUTH_TOKEN")
fi

ffmpeg -hide_banner -loglevel error -f lavfi -i "sine=frequency=440:duration=5" -c:a flac "$WORKDIR/test.flac" -y

IMPORT_JSON=$(curl -sf -X POST "$BASE_URL/api/v1/tracks/import" "${AUTH_ARGS[@]}" \
  -F "title=Smoke Signal" \
  -F "artist=HEXSONIC" \
  -F "album=Launch Grid" \
  -F "visibility=public" \
  -F "file=@$WORKDIR/test.flac")
echo "$IMPORT_JSON" > "$WORKDIR/import.json"

TRACK_ID=$(echo "$IMPORT_JSON" | sed -n 's/.*"track_id"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')
if [[ -z "$TRACK_ID" ]]; then
  echo "ERROR: no track_id in import response"
  exit 1
fi

SIGN_JSON=$(curl -sf -X POST "$BASE_URL/api/v1/streams/sign" "${AUTH_ARGS[@]}" \
  -H 'Content-Type: application/json' \
  -d "{\"track_id\":\"$TRACK_ID\",\"format\":\"original\"}")
echo "$SIGN_JSON" > "$WORKDIR/sign.json"
TOKEN=$(echo "$SIGN_JSON" | sed -n 's/.*"token"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')
EXPIRES=$(echo "$SIGN_JSON" | sed -n 's/.*"expires_unix"[[:space:]]*:[[:space:]]*\([0-9]*\).*/\1/p')

if [[ -z "$TOKEN" || -z "$EXPIRES" ]]; then
  echo "ERROR: invalid sign response"
  exit 1
fi

HTTP_CODE=$(curl -s -o "$WORKDIR/stream.bin" -w '%{http_code}' \
  "$BASE_URL/api/v1/stream/$TRACK_ID?format=original&token=$TOKEN&expires=$EXPIRES")
if [[ "$HTTP_CODE" != "200" ]]; then
  echo "ERROR: stream endpoint returned $HTTP_CODE"
  exit 1
fi

BYTES=$(wc -c < "$WORKDIR/stream.bin")
if [[ "$BYTES" -le 1000 ]]; then
  echo "ERROR: stream payload too small ($BYTES bytes)"
  exit 1
fi

curl -sf "$BASE_URL/api/v1/tracks" > "$WORKDIR/tracks.json"
echo "SMOKE_OK track_id=$TRACK_ID bytes=$BYTES"
