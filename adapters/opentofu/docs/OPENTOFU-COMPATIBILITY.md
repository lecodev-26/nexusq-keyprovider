# OpenTofu compatibility

## Tested compatibility matrix

The provider implements the OpenTofu External Key Provider protocol v1. Compatibility claims are based on completed end-to-end tests, not on protocol documentation or successful compilation alone.

| OpenTofu version | Status | Test coverage | Runner |
| --- | --- | --- | --- |
| 1.12.11 | CI E2E tested | Local state encryption, saved plans, previous-key recovery/rotation, HTTP remote-state persistence and clean-workdir recovery | Linux AMD64 |
| 1.13.2 | CI E2E tested | Core E2E on Linux; S3Mock and real NEXUS-Q service E2E on Linux; native provider protocol/state/saved-plan/HTTP recovery E2E on macOS and Windows | Linux AMD64/ARM64, macOS 15 ARM64, Windows Server 2025 AMD64 |

These are exact tested patch versions and runner environments. They do not imply that every patch in either minor series, earlier versions, future releases, or other operating systems/architectures are compatible.

## Version selection

As of 2 November 2026, the CI pins the latest patch releases published for the two currently supported OpenTofu release series: 1.12.11 and 1.13.2. The 1.12 series is scheduled to receive security support until 1 February 2027; the 1.13 series until 1 August 2027. Recheck upstream release and support status before changing these pins.

The core E2E workflow downloads the exact Linux AMD64 archive directly from the official OpenTofu GitHub release and verifies it against that release's published SHA256SUMS before installation. This avoids relying on the setup action's release index, which had not yet indexed the newly published 1.12.11 release at the time this matrix was added. The CI matrix deliberately keeps the more extensive S3Mock and real-NEXUS-Q service suites on 1.13.2. The 1.12.11 job runs the core provider E2E suite only. Do not describe the S3 backend or real-service suite as tested on 1.12.11.

## Reproducing the tests

Install the exact OpenTofu version and the requirements listed in the README, then run:

```sh
bash adapters/opentofu/tests/integration/e2e.sh
```

For the S3Mock-specific test (currently exercised in CI on 1.13.2 only):

```sh
bash adapters/opentofu/tests/integration/s3mock-e2e.sh
```

For the real NEXUS-Q service integration (currently exercised in CI on 1.13.2 only), see the instructions in the README and `adapters/opentofu/tests/integration/nexusq-real-e2e.sh`.

## Support policy

- CI should test the latest patch of each OpenTofu minor series that is still receiving upstream security support, subject to runner availability.
- A new patch pin must pass the full relevant E2E suite before the tested version is updated in documentation.
- When a minor series reaches end of security support, reassess whether to keep it in the matrix; do not silently imply active upstream support.
- Add new OpenTofu versions to the matrix only after the exact binary is available and the tests pass.
- macOS 15 ARM64 and Windows Server 2025 AMD64 have native runtime evidence for OpenTofu 1.13.2 only; this does not establish compatibility with other OS releases, architectures, or OpenTofu versions.

Upstream references: [OpenTofu releases](https://github.com/opentofu/opentofu/releases) and [OpenTofu release lifecycle](https://endoflife.date/opentofu).
