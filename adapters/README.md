# Adapters

Each directory under `adapters/` contains one integration for an external project.

## Adapter requirements

Every adapter owns its:

- Upstream protocol or plugin implementation.
- NEXUS-Q API mapping and configuration.
- Context binding, metadata format, and key lifecycle decisions.
- Unit and integration tests, including invalid-input and failure cases.
- Supported upstream versions and compatibility evidence.
- Installation, operation, and troubleshooting documentation.

Different adapters may use different languages and runtime models. Do not couple them through a shared protocol unless the external projects genuinely share one. Shared libraries are appropriate only for stable, security-reviewed behavior used by more than one adapter.

The NEXUS-Q engine remains outside this repository and is not copied into adapter directories.

## Current adapters

- [OpenTofu](opentofu/README.md) — first adapter; experimental Go implementation.
- [fnox](fnox/README.md) — second adapter; experimental Lua 5.4 provider with mock CLI tests and a dedicated real-service GitHub Actions workflow.
