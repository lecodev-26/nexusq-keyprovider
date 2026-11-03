# Integration portfolio roadmap

This roadmap tracks the shared repository and its independent adapters. Each adapter owns its detailed implementation plan, compatibility evidence, validation backlog, and release history.

## Repository foundations

- [x] Establish one central repository for all NEXUS-Q external key-provider integrations.
- [x] Keep adapter implementations, tests, documentation, and release tooling within each adapter directory.
- [x] Preserve the existing OpenTofu implementation and its historical release.
- [ ] Add a repeatable adapter template/checklist once a second adapter is implemented and its real common needs are known.

## Adapter portfolio

| Adapter | Status | Detailed roadmap |
| --- | --- | --- |
| OpenTofu | Implemented · experimental | [OpenTofu roadmap](adapters/opentofu/ROADMAP.md) |
| fnox | Research / planned | [fnox adapter plan](adapters/fnox/README.md) |

## Future adapter intake

Every new integration should have its own directory under `adapters/<project>/`, containing its implementation, tests, fixtures, documentation, compatibility matrix, scripts, and release artifacts. A new repository is not required for each target.

Before accepting a candidate, verify its upstream extension point, the actual NEXUS-Q API mapping, security boundaries, maintainer accessibility, and a realistic end-to-end validation route.

## Shared repository policy

The root is reserved for repository-wide governance, adapter discovery, GitHub-required adapter workflow entrypoints, and cross-adapter planning. Do not put project-specific source, tests, docs, or release bundles at the root. Do not force adapters into one language or protocol, and do not modify or duplicate the NEXUS-Q cryptographic engine as part of an adapter.

## Release naming

Use adapter-scoped Git tags and GitHub Releases for new releases:

- opentofu/vX.Y.Z (including prerelease suffixes such as opentofu/v0.1.0-rc.2)
- fnox/vX.Y.Z (including prerelease suffixes such as fnox/v0.1.0-rc.1)

Prefix downloadable assets with `nexusq-keyprovider-<adapter>-`. The historical OpenTofu tag v0.1.0-rc.1 and its published assets remain unchanged; the next OpenTofu release should use the new scoped convention. Release automation and asset naming are adapter-specific.
