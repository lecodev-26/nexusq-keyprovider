# Contributing

Thanks for helping build NEXUS-Q Key Provider Integrations.

This is one repository for multiple external adapters. Every adapter belongs in `adapters/<project>/` and owns its source, native protocol implementation, tests, fixtures, documentation, compatibility evidence, scripts, and release artifacts. Do not create a new repository for each integration.

## Before contributing

- Read the README and the target adapter's README, contribution notes, and threat model.
- Keep changes focused and explain security, compatibility, and operational impact.
- Never commit credentials, real keys, state files, plans, or private infrastructure details.
- Do not claim support until dedicated tests and documented validation exist.
- Do not duplicate or modify the NEXUS-Q cryptographic engine as part of adapter work.

## Development

The root Makefile delegates to the current Go adapter:

    make test-opentofu
    make vet-opentofu
    make fmt-check-opentofu
    make build-opentofu

Run adapter-specific commands from that adapter's directory. Each adapter may use a different language, runtime, formatter, linter, and test framework; its README must document the exact commands.

## Adapter boundaries

Each adapter owns its native external protocol or plugin contract, NEXUS-Q API mapping, configuration, context binding, metadata format, key lifecycle, error semantics, tests, compatibility matrix, release scripts, and operator documentation.

Do not force a common protocol or abstraction onto unrelated external projects. Extract shared components only after a concrete common requirement is demonstrated and the security boundary remains clear.

## Validation and claims

Mark an adapter implemented only after code, automated tests, integration tests, documentation, and compatibility evidence exist. Distinguish CI fixtures from tests against a real NEXUS-Q service or real external backend. Passing CI is not an independent security audit or production certification.

## Releases

Use adapter-scoped Git tags and GitHub Releases. Follow the target adapter's release documentation and keep downloadable assets prefixed with nexusq-keyprovider-<adapter>-.
