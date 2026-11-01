# Source and dependency provenance

## Current inventory

The current Go module has no third-party Go module dependencies. `go list -m all` reports only this module:

`github.com/lecodev-26/nexusq-keyprovider`

The Go implementation and tests use the Go standard library. The HTTP backend fixture uses Python's standard library. The Terraform files are integration-test fixtures/configuration, not vendored provider code.

The project does not currently vendor source code, bundle external executables, or include generated third-party source.

## Licensing

- Project source and original project documentation: Apache License 2.0; see the repository's `LICENSE`.
- Go standard library: distributed under the Go project's applicable open-source license terms; it is part of the Go toolchain and is not copied into this repository.
- Python standard library: distributed under the applicable Python Software Foundation license terms; it is not copied into this repository.
- GitHub Actions workflow: references external actions by release tags (`actions/checkout` and `actions/setup-go`). These are CI-time dependencies, not runtime dependencies. For a hardened release pipeline, pin each action to a reviewed full commit SHA and periodically review updates.

## Maintainer checks before a release

1. Re-run `go list -m -json all` and review any newly introduced module.
2. Review each dependency's license and provenance before accepting it; retain required notices and license texts.
3. Review changes to workflow actions and pin them to immutable commit SHAs for release builds.
4. Check that no binary, credential, generated secret, or unrelated source has entered the repository.
5. Re-run tests, vet, race detection on a supported host, and cross-builds.

This inventory is a snapshot of the repository, not a legal opinion, a complete software bill of materials, or a guarantee that every upstream tool used during development has been audited.
