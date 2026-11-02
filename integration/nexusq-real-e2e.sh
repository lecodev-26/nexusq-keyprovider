#!/usr/bin/env bash
set -euo pipefail
umask 077

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
NEXUSQ_ROOT="${NEXUSQ_SOURCE_DIR:-$ROOT/../nexusq}"
TMP="$(mktemp -d)"
SERVER_PID=""
BACKEND_PID=""
SERVER_BIN="$NEXUSQ_ROOT/target/release/nexusq-server"
CLI_BIN="$NEXUSQ_ROOT/target/release/nexusq"
PROVIDER_BIN="$TMP/nexusq-keyprovider"
SERVER_URL="http://127.0.0.1:18445"
BACKEND_URL="http://127.0.0.1:18444"

cleanup() {
  if [[ -n "$BACKEND_PID" ]]; then
    kill "$BACKEND_PID" 2>/dev/null || true
    wait "$BACKEND_PID" 2>/dev/null || true
  fi
  if [[ -n "$SERVER_PID" ]]; then
    kill "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
  fi
  rm -rf "$TMP"
}
trap cleanup EXIT

fail_if_contains() {
  local file="$1" value="$2"
  if grep -aFq -- "$value" "$file"; then
    echo "FAIL: plaintext marker found in $file" >&2
    exit 1
  fi
}

wait_for_url() {
  local url="$1"
  for _ in $(seq 1 80); do
    if curl --fail --silent --output /dev/null "$url"; then return 0; fi
    sleep 0.25
  done
  echo "Timed out waiting for $url" >&2
  [[ ! -f "$TMP/nexusq-server.log" ]] || tail -80 "$TMP/nexusq-server.log" >&2
  return 1
}

start_server() {
  NEXUSQ_VAULT_PATH="$TMP/vault.nqv" \
  NEXUSQ_SERVER_TOKEN="$SERVER_TOKEN" \
  NEXUSQ_SERVER_ADDR="127.0.0.1:18445" \
  NEXUSQ_SERVER_RATE_LIMIT="600" \
  RUST_LOG="nexusq_server=warn" \
    "$SERVER_BIN" >"$TMP/nexusq-server.log" 2>&1 &
  SERVER_PID=$!
  wait_for_url "$SERVER_URL/health"
}

unlock_server() {
  python3 -c 'import json,sys; print(json.dumps({"password":open(sys.argv[1], encoding="utf-8").read()}))' "$TMP/vault-password" >"$TMP/unlock.json"
  chmod 600 "$TMP/unlock.json"
  curl --config "$TMP/curl-auth.conf" --fail --silent --show-error -o /dev/null \
    -X POST "$SERVER_URL/v1/vault/unlock" \
    -H "Content-Type: application/json" \
    --data-binary "@$TMP/unlock.json"
  local state
  state="$(curl --config "$TMP/curl-auth.conf" --fail --silent --show-error "$SERVER_URL/v1/vault/status")"
  python3 -c 'import json,sys; assert json.load(sys.stdin)["state"] == "unlocked"' <<<"$state"
}

echo "== Build provider against the real NEXUS-Q service binaries =="
test -x "$CLI_BIN"
test -x "$SERVER_BIN"
(
  cd "$ROOT"
  go build -trimpath -o "$PROVIDER_BIN" .
)
chmod 700 "$PROVIDER_BIN"

echo "== Create an ephemeral NEXUS-Q vault and two wrapping keys =="
# Keep generated secrets newline-free: the CLI strips one trailing newline
# from password files, while the HTTP unlock API receives the exact bytes.
printf '%s' "$(openssl rand -hex 32)" >"$TMP/vault-password"
printf '%s' "$(openssl rand -hex 32)" >"$TMP/server-token"
chmod 600 "$TMP/vault-password" "$TMP/server-token"
SERVER_TOKEN="$(cat "$TMP/server-token")"
printf 'header = "Authorization: Bearer %s"\n' "$SERVER_TOKEN" >"$TMP/curl-auth.conf"
chmod 600 "$TMP/curl-auth.conf"

"$CLI_BIN" vault create "$TMP/vault.nqv" \
  --label "nexusq-keyprovider-ci" \
  --password-file "$TMP/vault-password"
mkdir "$TMP/audit"
"$CLI_BIN" vault attach-audit "$TMP/vault.nqv" "$TMP/audit" \
  --password-file "$TMP/vault-password"

KEY1_JSON="$("$CLI_BIN" --output json key generate "$TMP/vault.nqv" \
  --algorithm aes256gcm --purpose wrap --activate \
  --password-file "$TMP/vault-password")"
KEY1="$(python3 -c 'import json,sys; print(json.load(sys.stdin)["key_id"])' <<<"$KEY1_JSON")"

KEY2_JSON="$("$CLI_BIN" --output json key generate "$TMP/vault.nqv" \
  --algorithm aes256gcm --purpose wrap --activate \
  --password-file "$TMP/vault-password")"
KEY2="$(python3 -c 'import json,sys; print(json.load(sys.stdin)["key_id"])' <<<"$KEY2_JSON")"
test -n "$KEY1"
test -n "$KEY2"
test "$KEY1" != "$KEY2"

echo "== Start real NEXUS-Q and verify authentication boundary =="
start_server
if curl --silent --output /dev/null --write-out '%{http_code}' \
  "$SERVER_URL/v1/vault/status" | grep -q '^200$'; then
  echo "FAIL: protected endpoint accepted an unauthenticated request" >&2
  exit 1
fi
unlock_server

export NEXUSQ_URL="$SERVER_URL"
export NEXUSQ_TOKEN="$SERVER_TOKEN"
export NEXUSQ_KEY_ID="$KEY1"
export NEXUSQ_CONTEXT_ID="ci-real-nexusq-state"
export NEXUSQ_HTTP_BACKEND_STATE="$TMP/remote.tfstate"
export NEXUSQ_HTTP_BACKEND_LOCK="$TMP/remote.lock"


echo "== Real NEXUS-Q: explicit legacy v1 migration to context-bound v2 =="
python3 - "$KEY1" "$TMP/legacy-dek" "$TMP/legacy-encrypt.json" <<'PYLEGACY'
import base64, json, secrets, sys
dek = secrets.token_bytes(32)
with open(sys.argv[2], "wb") as handle:
    handle.write(dek)
request = {
    "key_id": sys.argv[1],
    "plaintext": base64.b64encode(dek).decode("ascii"),
    "metadata": "opentofu-external-key-provider-v1",
}
with open(sys.argv[3], "w", encoding="utf-8") as handle:
    json.dump(request, handle)
PYLEGACY
LEGACY_ENVELOPE="$(curl --config "$TMP/curl-auth.conf" --fail --silent --show-error \
  -H "Content-Type: application/json" --data-binary "@$TMP/legacy-encrypt.json" \
  "$SERVER_URL/v1/encrypt" | python3 -c 'import json,sys; print(json.load(sys.stdin)["data"])')"
python3 - "$LEGACY_ENVELOPE" "$KEY1" "$TMP/legacy-input.json" <<'PYLEGACY'
import json, sys
metadata = {
    "format_version": 1,
    "envelope": sys.argv[1],
    "wrapping_key_id": sys.argv[2],
}
with open(sys.argv[3], "w", encoding="utf-8") as handle:
    json.dump({"external_data": metadata}, handle)
PYLEGACY
if env -u NEXUSQ_ALLOW_LEGACY_V1 "$PROVIDER_BIN" \
  <"$TMP/legacy-input.json" >"$TMP/legacy-disabled.out" 2>"$TMP/legacy-disabled.err"; then
  echo "FAIL: legacy v1 metadata was accepted without the migration opt-in" >&2
  exit 1
fi
grep -q "legacy metadata v1 is disabled" "$TMP/legacy-disabled.err"
NEXUSQ_ALLOW_LEGACY_V1=true "$PROVIDER_BIN" \
  <"$TMP/legacy-input.json" >"$TMP/legacy-migrated.jsonl"
python3 - "$TMP/legacy-dek" "$TMP/legacy-migrated.jsonl" "$TMP/legacy-v2-input.json" <<'PYLEGACY'
import base64, json, sys
expected = open(sys.argv[1], "rb").read()
lines = open(sys.argv[2], encoding="utf-8").read().splitlines()
assert len(lines) == 2
result = json.loads(lines[1])
assert base64.b64decode(result["keys"]["decryption_key"]) == expected
external = result["meta"]["external_data"]
assert external["format_version"] == 2
assert external["context_id"] == "ci-real-nexusq-state"
with open(sys.argv[3], "w", encoding="utf-8") as handle:
    json.dump({"external_data": external}, handle)
PYLEGACY

echo "== Real NEXUS-Q: reject a v2 envelope used with another configured context =="
if NEXUSQ_CONTEXT_ID="ci-other-context" "$PROVIDER_BIN" \
  <"$TMP/legacy-v2-input.json" >"$TMP/wrong-context.out" 2>"$TMP/wrong-context.err"; then
  echo "FAIL: v2 metadata was accepted with a different configured context" >&2
  exit 1
fi
grep -q "metadata context_id does not match" "$TMP/wrong-context.err"

echo "== Real NEXUS-Q: reject tampered external context using encrypted payload binding =="
python3 - "$TMP/legacy-migrated.jsonl" "$TMP/tampered-context-input.json" <<'PYLEGACY'
import json, sys
lines = open(sys.argv[1], encoding="utf-8").read().splitlines()
external = json.loads(lines[1])["meta"]["external_data"]
external["context_id"] = "ci-tampered-context"
with open(sys.argv[2], "w", encoding="utf-8") as handle:
    json.dump({"external_data": external}, handle)
PYLEGACY
if NEXUSQ_CONTEXT_ID="ci-tampered-context" "$PROVIDER_BIN" \
  <"$TMP/tampered-context-input.json" >"$TMP/tampered-context.out" 2>"$TMP/tampered-context.err"; then
  echo "FAIL: tampered external context bypassed encrypted payload binding" >&2
  exit 1
fi
grep -q "decrypted payload context_id does not match" "$TMP/tampered-context.err"

echo "== Real NEXUS-Q: local state and saved-plan encryption =="
mkdir "$TMP/local"
sed "s|/absolute/path/to/nexusq-keyprovider|$PROVIDER_BIN|g" \
  "$ROOT/integration/main.tf" >"$TMP/local/main.tf"
(
  cd "$TMP/local"
  tofu init -input=false
  tofu plan -input=false -out="$TMP/saved.tfplan"
  fail_if_contains "$TMP/saved.tfplan" "terraform_data.example"
  fail_if_contains "$TMP/saved.tfplan" "replace-me"
  tofu show -json "$TMP/saved.tfplan" >"$TMP/saved-plan.json"
  grep -q '"terraform_data.example"' "$TMP/saved-plan.json"
  tofu apply -input=false -auto-approve "$TMP/saved.tfplan"
  test -s terraform.tfstate
  fail_if_contains terraform.tfstate "terraform_data.example"
  fail_if_contains terraform.tfstate "replace-me"
  tofu show -json >"$TMP/state.json"
  grep -q '"terraform_data.example"' "$TMP/state.json"
)

echo "== Retire the previous NEXUS-Q wrapping key through its rotation API =="
ROTATION="$(curl --config "$TMP/curl-auth.conf" --fail --silent --show-error -X POST \
  "$SERVER_URL/v1/keys/$KEY1/rotate")"
python3 -c 'import json,sys; assert json.load(sys.stdin)["data"]' <<<"$ROTATION"
curl --config "$TMP/curl-auth.conf" --fail --silent --show-error -o /dev/null -X POST \
  "$SERVER_URL/v1/vault/lock"
unlock_server

echo "== Real NEXUS-Q: previous-key recovery and provider key-ID rotation =="
export NEXUSQ_KEY_ID="$KEY2"
(
  cd "$TMP/local"
  tofu plan -input=false -out="$TMP/rotation.tfplan"
  tofu apply -input=false -auto-approve "$TMP/rotation.tfplan"
  tofu show -json >"$TMP/rotated-state.json"
  grep -q '"terraform_data.example"' "$TMP/rotated-state.json"
  tofu show -json "$TMP/saved.tfplan" >"$TMP/old-key-plan.json"
  grep -q '"terraform_data.example"' "$TMP/old-key-plan.json"
)

echo "== Restart real NEXUS-Q and recover encrypted state =="
kill "$SERVER_PID"
wait "$SERVER_PID" 2>/dev/null || true
SERVER_PID=""
start_server
unlock_server
(
  cd "$TMP/local"
  tofu show -json >"$TMP/restarted-state.json"
  grep -q '"terraform_data.example"' "$TMP/restarted-state.json"
  tofu show -json "$TMP/saved.tfplan" >"$TMP/restarted-old-key-plan.json"
  grep -q '"terraform_data.example"' "$TMP/restarted-old-key-plan.json"
)

echo "== Real NEXUS-Q: HTTP remote-state persistence and recovery =="
start_backend() {
  python3 -u "$ROOT/integration/fixtures/http-backend.py" >"$TMP/http-backend.log" 2>&1 &
  BACKEND_PID=$!
  wait_for_url "$BACKEND_URL/health"
}
stop_backend() {
  kill "$BACKEND_PID" 2>/dev/null || true
  wait "$BACKEND_PID" 2>/dev/null || true
  BACKEND_PID=""
}
start_backend
mkdir "$TMP/remote-write"
sed "s|/absolute/path/to/nexusq-keyprovider|$PROVIDER_BIN|g" \
  "$ROOT/integration/remote-http.tf" >"$TMP/remote-write/main.tf"
(
  cd "$TMP/remote-write"
  tofu init -input=false
  tofu apply -input=false -auto-approve
)
test -s "$NEXUSQ_HTTP_BACKEND_STATE"
fail_if_contains "$NEXUSQ_HTTP_BACKEND_STATE" "terraform_data.remote_demo"
fail_if_contains "$NEXUSQ_HTTP_BACKEND_STATE" "replace-me"
stop_backend
start_backend
mkdir "$TMP/remote-read"
sed "s|/absolute/path/to/nexusq-keyprovider|$PROVIDER_BIN|g" \
  "$ROOT/integration/remote-http.tf" >"$TMP/remote-read/main.tf"
(
  cd "$TMP/remote-read"
  tofu init -input=false -reconfigure
  tofu show -json >"$TMP/remote-state.json"
  grep -q '"terraform_data.remote_demo"' "$TMP/remote-state.json"
)

echo "== Verify NEXUS-Q audit endpoint reports a valid chain =="
AUDIT="$(curl --config "$TMP/curl-auth.conf" --fail --silent --show-error "$SERVER_URL/v1/audit")"
python3 -c 'import json,sys; assert json.load(sys.stdin)["verified"] is True' <<<"$AUDIT"

echo "PASS: real NEXUS-Q vault/API, authenticated state and saved-plan encryption, key rotation, server restart recovery, HTTP backend persistence, and audit verification"
