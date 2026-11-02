#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP="$(mktemp -d)"
MOCK_PID=""
BACKEND_PID=""
PYTHON_BIN="${PYTHON:-python3}"
PROVIDER_NAME="nexusq-keyprovider"
MOCK_NAME="mock-nexusq"
if [[ "${OS:-}" == "Windows_NT" || "${OSTYPE:-}" == msys* || "${OSTYPE:-}" == cygwin* ]]; then
  PROVIDER_NAME+=".exe"
  MOCK_NAME+=".exe"
fi

cleanup() {
  [[ -z "$BACKEND_PID" ]] || kill "$BACKEND_PID" 2>/dev/null || true
  [[ -z "$MOCK_PID" ]] || kill "$MOCK_PID" 2>/dev/null || true
  rm -rf "$TMP"
}
trap cleanup EXIT

fail_if_contains() {
  local file="$1" value="$2"
  if grep -aFq -- "$value" "$file"; then
    echo "FAIL: plaintext marker '$value' found in $file" >&2
    exit 1
  fi
}

wait_for_url() {
  local url="$1"
  for _ in $(seq 1 40); do
    if curl --fail --silent --output /dev/null "$url"; then return 0; fi
    sleep 0.25
  done
  echo "Timed out waiting for $url" >&2
  return 1
}

wait_for_mock() {
  for _ in $(seq 1 40); do
    if [[ "$(curl --fail --silent http://127.0.0.1:18443/health 2>/dev/null || true)" == "nexusq-mock-e2e" ]]; then
      return 0
    fi
    sleep 0.25
  done
  echo "Timed out waiting for the test-only NEXUS-Q mock" >&2
  return 1
}

echo "== Build provider and test-only NEXUS-Q API fixture =="
(
  cd "$ROOT"
  go build -trimpath -o "$TMP/$PROVIDER_NAME" .
  go build -trimpath -o "$TMP/$MOCK_NAME" ./integration/mock-nexusq
)
if [[ "$PROVIDER_NAME" != *.exe ]]; then chmod +x "$TMP/$PROVIDER_NAME"; fi

"$TMP/$MOCK_NAME" >"$TMP/mock-nexusq.log" 2>&1 &
MOCK_PID=$!
wait_for_mock

PROVIDER_PATH="$TMP/$PROVIDER_NAME"
TMP_NATIVE="$TMP"
if [[ "$PROVIDER_NAME" == *.exe ]]; then
  PROVIDER_PATH="$(cygpath -m "$PROVIDER_PATH")"
  TMP_NATIVE="$(cygpath -m "$TMP")"
fi

export NEXUSQ_URL="http://127.0.0.1:18443"
export NEXUSQ_TOKEN="integration-test-token"
export NEXUSQ_KEY_ID="integration-key-v1"
export NEXUSQ_CONTEXT_ID="integration-e2e-state"
export NEXUSQ_HTTP_BACKEND_STATE="$TMP_NATIVE/remote.tfstate"
export NEXUSQ_HTTP_BACKEND_LOCK="$TMP_NATIVE/remote.lock"

echo "== Local state and saved-plan encryption =="
mkdir "$TMP/local"
sed "s|/absolute/path/to/nexusq-keyprovider|$PROVIDER_PATH|g" \
  "$ROOT/integration/main.tf" > "$TMP/local/main.tf"
(
  cd "$TMP/local"
  tofu init -input=false
  tofu plan -input=false -out="$TMP/saved.tfplan"
  fail_if_contains "$TMP/saved.tfplan" "terraform_data.example"
  fail_if_contains "$TMP/saved.tfplan" "replace-me"
  tofu show -json "$TMP/saved.tfplan" > "$TMP/saved-plan.json"
  grep -q '"terraform_data.example"' "$TMP/saved-plan.json"
  tofu apply -input=false -auto-approve "$TMP/saved.tfplan"
  test -s terraform.tfstate
  fail_if_contains terraform.tfstate "terraform_data.example"
  fail_if_contains terraform.tfstate "replace-me"
  tofu show -json > "$TMP/state.json"
  grep -q '"terraform_data.example"' "$TMP/state.json"
)

echo "== Previous-key recovery and wrapping-key rotation =="
export NEXUSQ_KEY_ID="integration-key-v2"
(
  cd "$TMP/local"
  tofu plan -input=false -out="$TMP/rotation.tfplan"
  tofu apply -input=false -auto-approve "$TMP/rotation.tfplan"
  tofu show -json > "$TMP/rotated-state.json"
  grep -q '"terraform_data.example"' "$TMP/rotated-state.json"
)
curl --fail --silent http://127.0.0.1:18443/stats > "$TMP/mock-stats.json"
"$PYTHON_BIN" - "$TMP/mock-stats.json" <<'PY'
import json, sys
stats = json.load(open(sys.argv[1], encoding="utf-8"))
if stats["encrypt_calls"] < 2 or stats["decrypt_calls"] < 1:
    raise SystemExit(f"expected fresh wrapping and prior-key recovery, got {stats}")
print(f"mock API calls: encrypt={stats['encrypt_calls']} decrypt={stats['decrypt_calls']}")
PY

echo "== HTTP remote-state persistence and clean-workdir recovery =="
start_backend() {
  "$PYTHON_BIN" -u "$ROOT/integration/fixtures/http-backend.py" >"$TMP/http-backend.log" 2>&1 &
  BACKEND_PID=$!
  wait_for_url http://127.0.0.1:18444/health
}
stop_backend() {
  kill "$BACKEND_PID" 2>/dev/null || true
  wait "$BACKEND_PID" 2>/dev/null || true
  BACKEND_PID=""
}
start_backend
mkdir "$TMP/remote-write"
sed "s|/absolute/path/to/nexusq-keyprovider|$PROVIDER_PATH|g" \
  "$ROOT/integration/remote-http.tf" > "$TMP/remote-write/main.tf"
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
sed "s|/absolute/path/to/nexusq-keyprovider|$PROVIDER_PATH|g" \
  "$ROOT/integration/remote-http.tf" > "$TMP/remote-read/main.tf"
(
  cd "$TMP/remote-read"
  tofu init -input=false -reconfigure
  tofu show -json > "$TMP/remote-state.json"
  grep -q '"terraform_data.remote_demo"' "$TMP/remote-state.json"
)
echo "PASS: OpenTofu state, saved plans, DEK recovery/rotation, and remote HTTP state recovery"
