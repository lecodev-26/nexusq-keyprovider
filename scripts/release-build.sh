#!/usr/bin/env bash
set -euo pipefail

readonly REQUIRED_GO_VERSION="go1.27.2"
readonly OUTPUT_DIR="${1:-dist}"
if ! command -v go >/dev/null 2>&1; then echo "error: Go is required" >&2; exit 1; fi
actual_go="$(go version | awk '{print $3}')"
if [[ "$actual_go" != "$REQUIRED_GO_VERSION" ]]; then
  echo "error: release builds require $REQUIRED_GO_VERSION (found $actual_go)" >&2; exit 1
fi
if ! git rev-parse --show-toplevel >/dev/null 2>&1; then echo "error: run inside a Git checkout" >&2; exit 1; fi
if [[ -n "$(git status --porcelain)" ]]; then echo "error: release build requires a clean Git worktree" >&2; exit 1; fi
commit="$(git rev-parse HEAD)"
mkdir -p "$OUTPUT_DIR"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
targets=("linux amd64" "linux arm64" "darwin arm64" "windows amd64")
build_one() {
  local goos="$1" goarch="$2" destination="$3"
  local filename="nexusq-keyprovider-${goos}-${goarch}"
  [[ "$goos" == "windows" ]] && filename+=".exe"
  GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags="-buildid= -s -w" -o "$destination/$filename" .
}
for target in "${targets[@]}"; do
  read -r goos goarch <<<"$target"
  echo "Building $goos/$goarch (first pass)"
  build_one "$goos" "$goarch" "$OUTPUT_DIR"
  echo "Building $goos/$goarch (reproducibility pass)"
  build_one "$goos" "$goarch" "$tmp"
  filename="nexusq-keyprovider-${goos}-${goarch}"
  [[ "$goos" == "windows" ]] && filename+=".exe"
  cmp -s "$OUTPUT_DIR/$filename" "$tmp/$filename" || { echo "error: repeated builds differ for $goos/$goarch" >&2; exit 1; }
done
(cd "$OUTPUT_DIR" && sha256sum nexusq-keyprovider-* > SHA256SUMS)
cat > "$OUTPUT_DIR/BUILD-INFO.txt" <<INFO
Project: github.com/lecodev-26/nexusq-keyprovider
Source commit: $commit
Go toolchain: $actual_go
Build command: GOOS=<target> GOARCH=<target> CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags="-buildid= -s -w"
Targets: linux/amd64 linux/arm64 darwin/arm64 windows/amd64
Reproducibility check: each target built twice in this environment; byte-for-byte comparison passed
Checksums: SHA256SUMS (SHA-256)
INFO
echo "Builds and repeated-build comparisons passed. Artifacts: $OUTPUT_DIR/"
echo "This is local build evidence, not a signed attestation or independent reproducible-build verification."
