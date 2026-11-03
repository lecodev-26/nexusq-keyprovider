# NEXUS-Q Key Provider Integrations

**One repository. Multiple independent adapters. One existing NEXUS-Q cryptographic engine.**

This repository is the home for integrations that let external tools use NEXUS-Q for key management and envelope encryption. OpenTofu was the first integration, not the limit of the project. New targets are added as isolated adapters under adapters/, not as separate repositories.

NEXUS-Q itself is a separate cryptographic engine. This repository consumes its APIs; it does not duplicate or restructure the engine.

## Adapter catalog

| Adapter | Implementation | Status | Everything for this adapter |
| --- | --- | --- | --- |
| OpenTofu | Go · External Key Provider protocol v1 | Implemented · experimental | [adapters/opentofu/](adapters/opentofu/) |
| fnox | Lua 5.4 · custom provider plugin | Implemented · experimental (mock suite and real-service GitHub Actions integration passed on Ubuntu; see adapter validation notes) | [adapters/fnox/](adapters/fnox/) |

Each adapter directory is self-contained: source, tests, fixtures, documentation, compatibility information, build/release scripts, and generated artifacts all live with that adapter. The root contains only repository-wide governance, the adapter catalog, GitHub-required workflow entrypoints (one per adapter), and cross-adapter planning.

## Repository layout

```text
nexusq-keyprovider/
├── adapters/
│   ├── opentofu/
│   │   ├── docs/          # OpenTofu-only documentation and threat model
│   │   ├── tests/         # Unit tests and E2E fixtures
│   │   ├── scripts/       # OpenTofu build and SBOM helpers
│   │   ├── dist/          # Generated OpenTofu release artifacts (ignored)
│   │   ├── main.go
│   │   ├── main_test.go
│   │   ├── go.mod
│   │   ├── README.md
│   │   ├── CHANGELOG.md
│   │   └── ROADMAP.md
│   └── fnox/
│       ├── provider.lua
│       ├── tests/
│       ├── docs/
│       └── README.md
├── .github/workflows/    # GitHub-required adapter workflow entrypoints
├── CONTRIBUTING.md       # Repository-wide contribution rules
├── SECURITY.md           # Repository-wide security reporting
├── ROADMAP.md            # Cross-adapter portfolio
├── Makefile              # Root commands delegating to adapters
└── README.md
```

## Adapter boundaries

Adapters may use different languages, runtimes, protocols, test frameworks, and release formats. They do not need to share a generic provider protocol. Each adapter owns its own NEXUS-Q API mapping, metadata format, context binding, key lifecycle, error handling, tests, compatibility evidence, and operational guidance.

Only genuinely common, security-reviewed components should be extracted into shared code. Avoid premature frameworks and never move project-specific files back to the repository root.

## Working on an adapter

Start in the adapter's own README:

- [OpenTofu adapter guide](adapters/opentofu/README.md)
- [fnox adapter guide and implementation](adapters/fnox/README.md)

From the repository root, the Makefile provides convenience commands for each adapter. The fnox integration suite requires fnox 1.47.0+ and curl 8.4.0+ on PATH:

```sh
make test-opentofu
make vet-opentofu
make fmt-check-opentofu
make build-opentofu
make test-fnox
```

Adapter-specific tests, scripts, docs, and release artifacts remain inside the adapter folder. GitHub requires workflow entrypoints to live in .github/workflows/. Each entrypoint is adapter-scoped: opentofu.yml and fnox.yml run automatically on relevant push and pull-request changes through path filters, and both retain workflow_dispatch. The fnox workflow runs the mock CLI suite and a real encrypt/decrypt round trip against NEXUS-Q v4.1.2 built from pinned source.

## Releases and artifacts

New releases use adapter-scoped Git tags and separate GitHub Releases:

- OpenTofu: opentofu/vX.Y.Z (for example, opentofu/v0.1.0-rc.2)
- fnox: fnox/vX.Y.Z (for example, fnox/v0.1.0-rc.1)

Downloadable asset names are prefixed with nexusq-keyprovider-<adapter>-. Thus OpenTofu and fnox assets cannot overwrite or be confused with each other. The historical OpenTofu tag v0.1.0-rc.1 and its release assets remain unchanged; the next OpenTofu release should use the scoped tag convention.

See the [integration portfolio roadmap](ROADMAP.md) for the repository-level plan. Each adapter's own roadmap records its implementation and validation status.

## Security and maturity

Both OpenTofu and fnox adapters are experimental. Their CI results and compatibility claims apply only to the versions and environments recorded in their own documentation. No adapter should be described as independently audited, certified, or production-approved without published evidence.

See [SECURITY.md](SECURITY.md) and the adapter-specific threat model before using an integration with valuable secrets or infrastructure state.

## License

Apache License 2.0. See [LICENSE](LICENSE).
