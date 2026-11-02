#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP="$(mktemp -d)"
MOCK_PID=""
S3MOCK_CONTAINER="nexusq-s3mock-e2e-$$"
S3MOCK_VOLUME="${S3MOCK_CONTAINER}-data"
S3MOCK_IMAGE="adobe/s3mock:5.2.3"
S3_ACCESS_KEY="nexusq-e2e"
S3_SECRET_KEY="nexusq-e2e-local-only-secret"
S3_ENDPOINT="http://127.0.0.1:9090"

cleanup() {
  [[ -z "$MOCK_PID" ]] || kill "$MOCK_PID" 2>/dev/null || true
  docker rm -f "$S3MOCK_CONTAINER" >/dev/null 2>&1 || true
  docker volume rm "$S3MOCK_VOLUME" >/dev/null 2>&1 || true
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
  for _ in $(seq 1 60); do
    if curl --fail --silent --output /dev/null "$url"; then return 0; fi
    sleep 0.5
  done
  echo "Timed out waiting for $url" >&2
  docker logs "$S3MOCK_CONTAINER" >&2 || true
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

start_s3mock() {
  docker rm -f "$S3MOCK_CONTAINER" >/dev/null 2>&1 || true
  docker run --detach --name "$S3MOCK_CONTAINER" \
    --publish 9090:9090 \
    --env COM_ADOBE_TESTING_S3MOCK_STORE_ROOT=/s3mockroot \
    --env COM_ADOBE_TESTING_S3MOCK_STORE_RETAIN_FILES_ON_EXIT=true \
    --volume "$S3MOCK_VOLUME:/s3mockroot" \
    "$S3MOCK_IMAGE"
  wait_for_url "$S3_ENDPOINT/favicon.ico"
}

echo "== Build provider and test-only NEXUS-Q API fixture =="
(
  cd "$ROOT"
  go build -trimpath -o "$TMP/nexusq-keyprovider" .
  go build -trimpath -o "$TMP/mock-nexusq" ./integration/mock-nexusq
)
chmod +x "$TMP/nexusq-keyprovider"

"$TMP/mock-nexusq" >"$TMP/mock-nexusq.log" 2>&1 &
MOCK_PID=$!
wait_for_mock

export NEXUSQ_URL="http://127.0.0.1:18443"
export NEXUSQ_TOKEN="integration-test-token"
export NEXUSQ_KEY_ID="integration-key-v1"
export AWS_ACCESS_KEY_ID="$S3_ACCESS_KEY"
export AWS_SECRET_ACCESS_KEY="$S3_SECRET_KEY"
export AWS_DEFAULT_REGION="us-east-1"
export AWS_EC2_METADATA_DISABLED="true"

echo "== Start isolated S3Mock service =="
docker volume create "$S3MOCK_VOLUME" >/dev/null
start_s3mock
curl --fail --silent --show-error --request PUT --output /dev/null "$S3_ENDPOINT/nexusq-e2e-state/"

echo "== Apply encrypted state and saved plan to S3 backend =="
mkdir "$TMP/s3-write"
sed "s|/absolute/path/to/nexusq-keyprovider|$TMP/nexusq-keyprovider|g" \
  "$ROOT/integration/remote-s3.tf" > "$TMP/s3-write/main.tf"
(
  cd "$TMP/s3-write"
  tofu init -input=false
  tofu plan -input=false -out="$TMP/s3-saved.tfplan"
  tofu show -json "$TMP/s3-saved.tfplan" > "$TMP/s3-plan.json"
  grep -q '"terraform_data.s3_demo"' "$TMP/s3-plan.json"
  fail_if_contains "$TMP/s3-saved.tfplan" "nexusq-s3-plaintext-marker"
  tofu apply -input=false -auto-approve "$TMP/s3-saved.tfplan"
  tofu show -json > "$TMP/s3-state.json"
  grep -q '"terraform_data.s3_demo"' "$TMP/s3-state.json"
)

curl --fail --silent --show-error \
  --output "$TMP/s3-remote.tfstate" \
  "$S3_ENDPOINT/nexusq-e2e-state/state/terraform.tfstate"
test -s "$TMP/s3-remote.tfstate"
fail_if_contains "$TMP/s3-remote.tfstate" "nexusq-s3-plaintext-marker"
fail_if_contains "$TMP/s3-remote.tfstate" "integration-test-token"

echo "== Restart S3Mock and recover from a clean OpenTofu directory =="
docker stop --time 10 "$S3MOCK_CONTAINER" >/dev/null
docker rm "$S3MOCK_CONTAINER" >/dev/null
start_s3mock
curl --fail --silent --show-error --head \
  "$S3_ENDPOINT/nexusq-e2e-state/state/terraform.tfstate" >/dev/null

mkdir "$TMP/s3-read"
sed "s|/absolute/path/to/nexusq-keyprovider|$TMP/nexusq-keyprovider|g" \
  "$ROOT/integration/remote-s3.tf" > "$TMP/s3-read/main.tf"
(
  cd "$TMP/s3-read"
  tofu init -input=false
  tofu show -json > "$TMP/recovered-state.json"
  grep -q '"terraform_data.s3_demo"' "$TMP/recovered-state.json"
  grep -Fq "nexusq-s3-plaintext-marker" "$TMP/recovered-state.json"
)

echo "== Rotate wrapping-key ID and persist a new encrypted S3 state =="
export NEXUSQ_KEY_ID="integration-key-v2"
(
  cd "$TMP/s3-read"
  tofu plan -input=false -out="$TMP/s3-rotation.tfplan"
  tofu apply -input=false -auto-approve "$TMP/s3-rotation.tfplan"
  tofu show -json > "$TMP/rotated-state.json"
  grep -q '"terraform_data.s3_demo"' "$TMP/rotated-state.json"
)
curl --fail --silent --show-error \
  --output "$TMP/s3-rotated.tfstate" \
  "$S3_ENDPOINT/nexusq-e2e-state/state/terraform.tfstate"
fail_if_contains "$TMP/s3-rotated.tfstate" "nexusq-s3-plaintext-marker"
curl --fail --silent http://127.0.0.1:18443/stats > "$TMP/mock-stats.json"
python3 - "$TMP/mock-stats.json" <<'PY'
import json, sys
with open(sys.argv[1], encoding="utf-8") as handle:
    stats = json.load(handle)
if stats["encrypt_calls"] < 2 or stats["decrypt_calls"] < 1:
    raise SystemExit(f"expected fresh wrapping and prior-DEK recovery, got {stats}")
print(f"mock API calls: encrypt={stats['encrypt_calls']} decrypt={stats['decrypt_calls']}")
PY

echo "PASS: OpenTofu encrypted state and saved plan on S3Mock, object confidentiality scan, service restart recovery, and wrapping-key rotation"
