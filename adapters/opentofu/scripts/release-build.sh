#!/usr/bin/env bash
set -euo pipefail

readonly REQUIRED_GO_VERSION="go1.27.2"
readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly ADAPTER_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
readonly REPO_ROOT="$(cd "$ADAPTER_DIR/../.." && pwd)"
readonly OUTPUT_DIR="${1:-$ADAPTER_DIR/dist}"

if ! command -v go >/dev/null 2>&1; then echo "error: Go is required" >&2; exit 1; fi
actual_go="$(go version | awk '{print $3}')"
if [[ "$actual_go" != "$REQUIRED_GO_VERSION" ]]; then
  echo "error: release builds require $REQUIRED_GO_VERSION (found $actual_go)" >&2; exit 1
fi
if ! git -C "$REPO_ROOT" rev-parse --show-toplevel >/dev/null 2>&1; then echo "error: run inside a Git checkout" >&2; exit 1; fi
if [[ -n "$(git -C "$REPO_ROOT" status --porcelain)" ]]; then echo "error: release build requires a clean Git worktree" >&2; exit 1; fi
commit="$(git -C "$REPO_ROOT" rev-parse HEAD)"
mkdir -p "$OUTPUT_DIR"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/gocache-first" "$tmp/gocache-second"
targets=("linux amd64" "linux arm64" "darwin arm64" "windows amd64")
build_one() {
  local goos="$1" goarch="$2" destination="$3" cache_dir="$4"
  local filename="nexusq-keyprovider-opentofu-${goos}-${goarch}"
  [[ "$goos" == "windows" ]] && filename+=".exe"
  (
    cd "$ADAPTER_DIR"
    GOTOOLCHAIN=local GOWORK=off GOFLAGS=-mod=readonly GOCACHE="$cache_dir" \
      GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 \
      go build -trimpath -buildvcs=false -ldflags="-buildid= -s -w" -o "$destination/$filename" .
  )
}
for target in "${targets[@]}"; do
  read -r goos goarch <<<"$target"
  echo "Building OpenTofu $goos/$goarch (isolated cache, first pass)"
  build_one "$goos" "$goarch" "$OUTPUT_DIR" "$tmp/gocache-first"
  echo "Building OpenTofu $goos/$goarch (independent cache, second pass)"
  build_one "$goos" "$goarch" "$tmp" "$tmp/gocache-second"
  filename="nexusq-keyprovider-opentofu-${goos}-${goarch}"
  [[ "$goos" == "windows" ]] && filename+=".exe"
  cmp -s "$OUTPUT_DIR/$filename" "$tmp/$filename" || { echo "error: independent-cache builds differ for $goos/$goarch" >&2; exit 1; }
done
(cd "$OUTPUT_DIR" && sha256sum nexusq-keyprovider-opentofu-* > SHA256SUMS)
cat > "$OUTPUT_DIR/BUILD-INFO.txt" <<INFO
Project: github.com/lecodev-26/nexusq-keyprovider
Module: github.com/lecodev-26/nexusq-keyprovider/adapters/opentofu
Adapter: OpenTofu
Source commit: $commit
Go toolchain: $actual_go (GOTOOLCHAIN=local)
Build command: GOWORK=off GOFLAGS=-mod=readonly GOOS=<target> GOARCH=<target> CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags="-buildid= -s -w"
Targets: linux/amd64 linux/arm64 darwin/arm64 windows/amd64
Reproducibility check: each target built twice using separate, initially empty GOCACHE directories; byte-for-byte comparison passed
Checksums: SHA256SUMS (SHA-256)
INFO
echo "OpenTofu builds and independent-cache comparisons passed. Artifacts: $OUTPUT_DIR/"
echo "This is local build evidence, not a signed attestation or independent reproducible-build verification."
