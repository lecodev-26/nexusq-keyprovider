# Release build and verification

## Scope

Prebuilt release binaries are not currently published. This procedure creates local cross-platform candidates and checks that two builds made in the same environment are byte-for-byte identical. This is a determinism check, **not** an independent reproducible-build attestation; another operator must rebuild the same commit and compare hashes.

## Pinned build environment

- Go toolchain: **go1.27.2** (exact version required by the release script).
- Module language/toolchain floor: Go 1.26.0 as declared in `go.mod`.
- CGO disabled; build flags: `-trimpath -buildvcs=false -ldflags="-buildid= -s -w"`.
- Targets: Linux AMD64, Linux ARM64, macOS ARM64, Windows AMD64.

Update `scripts/release-build.sh`, CI, and this document together when changing the exact toolchain pin.

## Build

```sh
git clone https://github.com/lecodev-26/nexusq-keyprovider.git
cd nexusq-keyprovider
git checkout <reviewed-commit>
go version
go test -count=1 ./...
go vet ./...
bash scripts/release-build.sh
```

The script refuses a dirty Git worktree or a Go version other than go1.27.2. It builds each target twice and compares files byte-for-byte. Outputs, `SHA256SUMS`, and `BUILD-INFO.txt` are written to ignored `dist/`.

## Verify checksums

```sh
cd dist
sha256sum --check SHA256SUMS
```

For independent verification, a second operator should check out the exact source commit in `BUILD-INFO.txt`, use the same Go toolchain and build command in a separate clean environment, run the script, and compare SHA-256 hashes. Record OS/container image and toolchain provenance.

## Artifact names

- `nexusq-keyprovider-linux-amd64`
- `nexusq-keyprovider-linux-arm64`
- `nexusq-keyprovider-darwin-arm64`
- `nexusq-keyprovider-windows-amd64.exe`

Cross-compilation is not runtime testing. OpenTofu E2E runtime tests currently run only on Linux AMD64 and ARM64.

## Compatibility boundary

Automated OpenTofu integration jobs currently pin **OpenTofu 1.13.2**. This is the currently exercised baseline, not a broad compatibility matrix. Older/newer releases and runtime behavior on macOS/Windows must not be claimed until dedicated CI/runtime tests pass.

## Checksums and provenance limitations

SHA-256 checksums detect artifact changes only if the checksum file itself comes through a trusted channel; they do not authenticate the publisher. This repository does not yet sign binaries, publish GitHub Releases, create signed release attestations, or provide an SBOM for release artifacts. These remain future release-engineering work.
