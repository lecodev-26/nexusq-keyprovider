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
- GitHub Actions workflow: references external actions by release tags (`actions/checkout`, `actions/setup-go`, and `opentofu/setup-opentofu`). These are CI-time dependencies, not runtime dependencies. The NEXUS-Q source checkout and the Rust toolchain setup action are pinned to full commit SHAs. For a hardened release pipeline, pin each action to a reviewed full commit SHA and periodically review updates.

## Release build evidence and limitations

The repository includes `scripts/release-build.sh`, which requires Go 1.27.2, a clean Git worktree, disables CGO, uses `-trimpath` and `-buildvcs=false`, and builds Linux AMD64/ARM64, macOS ARM64, and Windows AMD64 outputs twice. It compares each pair byte-for-byte and emits SHA-256 checksums plus a build-info file. CI runs the same procedure on Ubuntu 24.04.

This establishes repeated-build determinism within that CI runner, not independent reproducibility across machines. Checksums are not signatures; they do not authenticate the publisher. There are no published release binaries, signed attestations, signed executables, or release SBOM yet. See [Release build and verification](RELEASE-BUILDS.md).

## Maintainer checks before a release

1. Re-run `go list -m -json all` and review any newly introduced module.
2. Review each dependency's license and provenance before accepting it; retain required notices and license texts.
3. Review changes to workflow actions and pin them to immutable commit SHAs for release builds.
4. Check that no binary, credential, generated secret, or unrelated source has entered the repository.
5. Re-run tests, vet, race detection on a supported host, and cross-builds.

This inventory is a snapshot of the repository, not a legal opinion, a complete software bill of materials, or a guarantee that every upstream tool used during development has been audited.
