# OpenTofu adapter

**Status: implemented · experimental**

This directory contains everything specific to the OpenTofu integration: Go implementation, unit tests, end-to-end fixtures, documentation, compatibility evidence, release scripts, and generated release artifacts.

## Integration

The adapter implements OpenTofu External Key Provider protocol v1. It generates fresh 256-bit data-encryption keys (DEKs), asks NEXUS-Q to wrap them through its envelope API, and recovers previous DEKs when OpenTofu requests them. OpenTofu remains responsible for encrypting state and saved plans using its native AES-GCM method.

## Directory layout

```text
adapters/opentofu/
├── docs/          # Design, compatibility, operations, threat model, review
├── tests/
│   ├── integration/   # OpenTofu E2E scripts, fixtures, mock NEXUS-Q API
│   └── ...            # Future adapter-local test suites
├── scripts/       # Release build and CI SBOM helper
├── dist/          # Generated release assets (gitignored)
├── main.go
├── main_test.go       # Go unit tests
├── go.mod
├── CHANGELOG.md
└── ROADMAP.md
```

## Build and test

Run from this directory:

```sh
go test -count=1 ./...
go vet ./...
test -z "$(gofmt -l .)"
mkdir -p dist
go build -trimpath -buildvcs=false -o dist/nexusq-keyprovider-opentofu .
```

Or use the repository-root convenience commands: make test-opentofu, make vet-opentofu, make fmt-check-opentofu, and make build-opentofu.

The end-to-end fixtures are in tests/integration/. They require the tools and environment described in the corresponding test documentation.

## Configuration

The adapter reads NEXUSQ_URL, NEXUSQ_TOKEN, NEXUSQ_KEY_ID, and NEXUSQ_CONTEXT_ID. Legacy v1 metadata migration is disabled by default and requires an explicit NEXUSQ_ALLOW_LEGACY_V1=true opt-in.

## Documentation

All OpenTofu-specific documentation is under [docs/](docs/):

- [Documentation index](docs/README.md)
- [Design and trust boundaries](docs/DESIGN.md)
- [Security policy and threat model](docs/SECURITY.md)
- [Compatibility matrix](docs/OPENTOFU-COMPATIBILITY.md)
- [Installation and troubleshooting](docs/INSTALLATION-TROUBLESHOOTING.md)
- [External operator testing](docs/EXTERNAL-TESTING.md)
- [AWS S3 validation runbook](docs/AWS-S3-VALIDATION.md)
- [Release build and verification](docs/RELEASE-BUILDS.md)
- [External review status](docs/EXTERNAL-REVIEW.md)
- [Adapter roadmap](ROADMAP.md)
- [Changelog](CHANGELOG.md)

## Release assets

OpenTofu release bundles are generated into dist/ and ignored by Git. They are published on the OpenTofu-specific GitHub Release, using tags such as opentofu/v0.1.0-rc.2. Assets use the nexusq-keyprovider-opentofu- prefix.

The existing historical tag v0.1.0-rc.1 is preserved exactly as published.

## Validation boundary

The adapter remains experimental. Automated test coverage applies only to the versions and environments recorded in the compatibility matrix. S3Mock testing is not real AWS S3 validation. No independent security audit, FIPS validation, or production certification is claimed.
