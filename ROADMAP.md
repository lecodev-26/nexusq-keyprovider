# Project roadmap

This roadmap distinguishes implemented and automated checks from validations that require external infrastructure or independent review.

## P2 — Close design risks

**Status: complete.**

- Context-bound v2 wrapped-DEK payloads are implemented.
- Legacy v1 metadata is rejected by default; migration requires an explicit opt-in.
- E2E tests cover context mismatch, tampered external context metadata, and explicit v1 migration.
- Design, security, and threat-model documentation describe the remaining trust boundaries and limitations.

Evidence: CI run [41424371993](https://github.com/lecodev-26/nexusq-keyprovider/actions/runs/41424371993), commit `ef20f03cb9b0e8b4cb1e60404ff26d73f9e19d80`.

## P3 — Backend compatibility and operational validation

**Status: partially validated; AWS-specific validation is pending.**

Completed evidence:

- OpenTofu S3 backend E2E runs in CI against a pinned, isolated Adobe S3Mock service.
- The S3Mock suite checks encrypted state and saved-plan handling, plaintext-marker scans, recovery after service restart from a clean working directory, and wrapping-key ID rotation.
- The CI suite does not enable S3 locking and uses generated local test credentials.

Not confirmed because no dedicated AWS test account/environment is currently available:

- AWS S3 behavior against the real service.
- IAM least-privilege policies and bucket policies.
- S3 versioning, Object Lock, SSE-KMS, replication, lifecycle, and production TLS configuration.
- Native S3 lockfile behavior and lock contention under concurrent OpenTofu operations.
- AWS-specific failure and recovery behavior.

The reproducible validation procedure is documented in [AWS S3 validation runbook](docs/AWS-S3-VALIDATION.md). It is a prepared procedure, **not evidence that AWS tests have passed**. Keep these items open until they are executed in an authorized, disposable AWS environment.

## P4 — Quality, compatibility, and distribution

**Status: technical checks expanded; external validations remain pending.**

Completed and automated:

- Added a pinned Go 1.27.2 release-build script for Linux AMD64/ARM64, macOS ARM64, and Windows AMD64.
- Each target is built twice using separate, initially empty Go build-cache directories; checksums and build metadata are generated.
- A second GitHub-hosted runner rebuilds the same commit with fresh caches, verifies the release bundle and all attestations, compares all four binaries byte-for-byte, and uploads a comparison report.
- Added three separate SPDX 2.3 inventories: source workspace, compiled release binaries, and direct CI workflow actions and declared integration inputs, plus release-build tools. Syft is pinned to v1.55.0.
- Added a build-environment manifest with runner image metadata, OS, Go environment/module graph, and direct action/tool references.
- Added signed attestations for SLSA build provenance and each of the three SBOMs; the second runner verifies each Sigstore bundle.
- Added native OpenTofu 1.13.2 runtime E2E jobs on macOS 15 ARM64 and Windows Server 2025 AMD64, including provider protocol, local state/saved plans, DEK recovery/rotation, and HTTP remote-state recovery using test fixtures.
- Added Linux/macOS/Windows installation guidance, environment setup, upgrade/rollback precautions, and troubleshooting in [Installation, upgrades, and troubleshooting](docs/INSTALLATION-TROUBLESHOOTING.md).

Scope and remaining limitations:

1. The CI environment SBOM inventories direct actions across the CI workflow, declared integration inputs, and directly invoked release-build tools. It does not enumerate every preinstalled GitHub runner package, action-internal transitive dependency, GitHub service component, or NEXUS-Q deployment dependency.
2. Cross-runner reproducibility remains inside GitHub Actions. A separate operator must reproduce the build outside this workflow and record the result.
3. macOS/Windows runtime coverage is limited to the exact tested runner labels and OpenTofu 1.13.2; it does not imply compatibility with all OS releases, architectures, or OpenTofu patches.
4. Arrange an independent security review before making any production-readiness claim.
5. Execute the [AWS S3 validation runbook](docs/AWS-S3-VALIDATION.md) in an authorized disposable AWS account. It remains a prepared procedure, not evidence that AWS tests have passed.
6. Keep the OpenTofu compatibility matrix current as supported patch releases change; the exact tested versions and scope are recorded in [OpenTofu compatibility](docs/OPENTOFU-COMPATIBILITY.md).

The release candidate and verification evidence are retained as 30-day workflow artifacts; no GitHub Release is published automatically. See [Release build and verification](docs/RELEASE-BUILDS.md) for the precise SBOM, attestation, reproducibility, and runtime-validation boundaries.

## P5 — External review

**Status: pending.**

- Request design and protocol feedback from OpenTofu maintainers/community.
- Seek feedback on External Key Provider interoperability.
- Arrange an independent security review before any production-readiness claim.
- Revisit open P3 AWS checks when an authorized test environment is available.

## Release and production boundary

This repository remains an experimental integration prototype. Passing CI, S3Mock coverage, and a prepared AWS runbook do not establish production readiness, an independent security audit, or AWS service compatibility.
