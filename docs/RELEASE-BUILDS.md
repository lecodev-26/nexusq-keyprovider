# Release build and verification

## Scope

Prebuilt release binaries are not currently published. CI creates release candidates, checks repeatability, rebuilds them on a separate GitHub-hosted runner, and retains verification evidence. This is stronger cross-runner evidence, **not an independent operator or third-party reproducible-build audit**.

## Pinned build environment

- Go toolchain: **go1.27.2** (exact version required by the release script).
- Module language/toolchain floor: Go 1.26.0 as declared in `go.mod`.
- `GOTOOLCHAIN=local`, `GOWORK=off`, `GOFLAGS=-mod=readonly`, CGO disabled, `-trimpath`, `-buildvcs=false`, and `-ldflags="-buildid= -s -w"`.
- Each target is built twice using separate, initially empty Go build-cache directories.
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

The script refuses a dirty Git worktree or a Go version other than go1.27.2. It builds each target twice with independent empty `GOCACHE` directories and compares files byte-for-byte. Outputs, `SHA256SUMS`, and `BUILD-INFO.txt` are written to ignored `dist/`.

## SBOMs and build-environment inventory

The release-build CI job produces three distinct SPDX 2.3 JSON documents:

1. `sbom-source.spdx.json` — the checked-out source workspace and detected project dependencies.
2. `sbom-release-artifacts.spdx.json` — the four compiled release binaries, scanned separately by Syft.
3. `sbom-ci-environment.spdx.json` — direct actions declared across the CI workflow, integration input versions, and directly invoked release-build tools, with versions/resolved references.

Syft is pinned to `v1.55.0`. `BUILD-ENVIRONMENT.txt` records the runner image metadata, OS, Go environment/module graph, and direct action/tool versions. The CI environment SPDX inventory records runner labels across the workflow and the actual image/tool versions of the release-build runner. It deliberately does **not** enumerate every preinstalled package on every runner, transitive dependencies bundled inside each action, the exact Rust stable patch selected at runtime, GitHub service internals, or the external NEXUS-Q deployment. Those remain outside its scope.

The release candidate tarball includes all three SBOMs, the environment manifest, checksums, build metadata, and four binaries. The workflow separately attests the source, release-binary, and CI-environment SBOMs and the tarball's SLSA build provenance. The tarball and Sigstore bundles are retained as workflow artifacts for 30 days; no GitHub Release is published automatically.

## Verify checksums and attestations

After downloading and extracting the workflow artifact:

```sh
cd extracted-bundle
sha256sum --check SHA256SUMS
gh attestation trusted-root > trusted_root.jsonl
gh attestation verify nexusq-keyprovider-release-candidates.tar.gz --repo lecodev-26/nexusq-keyprovider --predicate-type https://slsa.dev/provenance/v1 --bundle provenance.sigstore.json --custom-trusted-root trusted_root.jsonl
gh attestation verify nexusq-keyprovider-release-candidates.tar.gz --repo lecodev-26/nexusq-keyprovider --predicate-type https://spdx.dev/Document/v2.3 --bundle source-sbom-attestation.sigstore.json --custom-trusted-root trusted_root.jsonl
gh attestation verify nexusq-keyprovider-release-candidates.tar.gz --repo lecodev-26/nexusq-keyprovider --predicate-type https://spdx.dev/Document/v2.3 --bundle release-sbom-attestation.sigstore.json --custom-trusted-root trusted_root.jsonl
gh attestation verify nexusq-keyprovider-release-candidates.tar.gz --repo lecodev-26/nexusq-keyprovider --predicate-type https://spdx.dev/Document/v2.3 --bundle ci-sbom-attestation.sigstore.json --custom-trusted-root trusted_root.jsonl
```

The second GitHub-hosted runner verifies all four attestations, verifies the bundle checksums, rebuilds the same source commit with fresh caches, and compares the four binaries byte-for-byte. It uploads an independent rebuild report for 30 days. This demonstrates cross-runner agreement inside GitHub Actions, not independence from GitHub, the repository maintainer, or the CI workflow itself.

## Runtime validation

Cross-compilation is not runtime testing. The CI workflow also runs the provider's OpenTofu 1.13.2 protocol, local-state/saved-plan, DEK recovery/rotation, and HTTP remote-state recovery E2E suite on native macOS 15 ARM64 and Windows Server 2025 AMD64 runners. The Linux compatibility matrix remains separately documented in [OpenTofu compatibility](OPENTOFU-COMPATIBILITY.md). These native jobs use a test-only NEXUS-Q API fixture; they do not test a production NEXUS-Q deployment or AWS S3.

## Artifact names

- `nexusq-keyprovider-linux-amd64`
- `nexusq-keyprovider-linux-arm64`
- `nexusq-keyprovider-darwin-arm64`
- `nexusq-keyprovider-windows-amd64.exe`

## Compatibility boundary

The core Linux E2E matrix tests **1.12.11 on Linux AMD64** and **1.13.2 on Linux AMD64/ARM64**. Native runtime jobs test **1.13.2 on macOS 15 ARM64 and Windows Server 2025 AMD64**. S3Mock and real NEXUS-Q service E2E remain on Linux with OpenTofu 1.13.2. Other patches/releases and other operating-system versions must not be claimed until dedicated tests pass.

## Remaining external validation

- A separate operator must reproduce the build outside this repository's GitHub Actions workflow and record the result.
- An independent security auditor must review the design, implementation, CI and operational controls before any production-readiness claim.
- Real AWS S3, IAM, versioning, locking, Object Lock, SSE-KMS, and recovery behavior require an authorized AWS test account; S3Mock is not equivalent.
- A full inventory of every preinstalled runner package and every transitive dependency inside GitHub Actions is not currently produced. The current CI SBOM covers direct workflow actions, declared integration inputs, and directly invoked release-build tools only.

SHA-256 checksums detect artifact changes only when the checksum file is obtained through a trusted channel; they do not authenticate the publisher by themselves. Attestations authenticate workflow provenance and SBOM associations, not the correctness or security of the implementation. Keep release publication and any production-readiness claim subject to independent review.
