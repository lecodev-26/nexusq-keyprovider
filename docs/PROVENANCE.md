# Source and dependency provenance

## Current inventory

The current Go module has no third-party Go module dependencies. `go list -m all` reports only this module:

`github.com/lecodev-26/nexusq-keyprovider`

The Go implementation, tests, and test-only NEXUS-Q API substitute use the Go standard library. The mock API fixture uses AES-256-GCM with a fixed, public test key and is not a production service or a cryptographic validation of NEXUS-Q. The HTTP backend fixture uses Python's standard library. The Terraform files are integration-test fixtures/configuration, not vendored provider code.

The project does not currently vendor source code, bundle external executables, or include generated third-party source.

The real-service CI integration job checks out the public NEXUS-Q repository at commit `8735e5537bc687ee906ed68bdf119fc8956d9439` (the dereferenced v4.1.2 tag commit) and builds its CLI/server from source. NEXUS-Q is a separate project and is not copied into this repository or linked as a Go dependency. The CI job creates a disposable vault, random password, bearer token, and wrapping keys at runtime; none are stored as repository secrets or uploaded as artifacts.

## Licensing

- Project source and original project documentation: Apache License 2.0; see the repository's `LICENSE`.
- Go standard library: distributed under the Go project's applicable open-source license terms; it is part of the Go toolchain and is not copied into this repository.
- Python standard library: distributed under the applicable Python Software Foundation license terms; it is not copied into this repository.
- GitHub Actions workflow: CI-time dependencies are not runtime dependencies. The SBOM, attestation, and artifact-upload actions used by the release-build job are pinned to reviewed immutable commit SHAs; other workflow actions should continue to be reviewed and pinned before using this workflow as a production release pipeline. The NEXUS-Q source checkout and Rust toolchain setup action are also pinned to full commit SHAs.

## Release build evidence and limitations

The repository includes `scripts/release-build.sh`, which requires Go 1.27.2, a clean Git worktree, disables CGO, uses `-trimpath` and `-buildvcs=false`, and builds Linux AMD64/ARM64, macOS ARM64, and Windows AMD64 outputs twice. It compares each pair byte-for-byte and emits SHA-256 checksums plus a build-info file. CI runs the same procedure on Ubuntu 24.04.

CI now also rebuilds the exact commit on a second GitHub-hosted runner and compares all four binary outputs byte-for-byte with the primary runner. This is cross-runner evidence within GitHub Actions, but not an independent operator or third-party reproducibility audit. Checksums are not signatures; they do not authenticate the publisher. The release-build CI job now generates an SPDX JSON source SBOM with Anchore Syft, assembles a release-candidate bundle, and creates separate GitHub artifact attestations for SLSA build provenance and the SPDX SBOM using GitHub's attestation service. The workflow uploads the candidate bundle as a 30-day workflow artifact; it does not publish a GitHub Release or sign the executable files themselves. The SBOM describes the scanned source workspace and should not be mistaken for a complete inventory of the external CI toolchain or NEXUS-Q deployment environment. See [Release build and verification](RELEASE-BUILDS.md).

## Maintainer checks before a release

1. Re-run `go list -m -json all` and review any newly introduced module.
2. Review each dependency's license and provenance before accepting it; retain required notices and license texts.
3. Review changes to workflow actions and pin them to immutable commit SHAs for release builds.
4. Check that no binary, credential, generated secret, or unrelated source has entered the repository.
5. Re-run tests, vet, race detection on a supported host, and cross-builds.

This inventory is a snapshot of the repository, not a legal opinion, a complete software bill of materials, or a guarantee that every upstream tool used during development has been audited.
