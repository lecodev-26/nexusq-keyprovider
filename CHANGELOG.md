# Changelog

All notable changes to this project will be documented in this file.

## Unreleased

- Generate separate SPDX 2.3 SBOMs for source, release binaries, and direct CI workflow actions, declared integration inputs, and release-build tools; pin Syft v1.55.0 and attest each SBOM plus SLSA build provenance. Record the CI build environment and retain the release candidate and verification bundles for 30 days; no GitHub Release is published automatically.
- Run native OpenTofu 1.13.2 provider runtime E2E on macOS 15 ARM64 and Windows Server 2025 AMD64.
- Strengthen release reproducibility checks with separate empty Go build caches on each runner and retain an independent-runner byte-comparison report; this remains GitHub Actions-internal evidence, not third-party verification.

- Add platform-specific build/install guidance, upgrade and rollback cautions, and a troubleshooting reference.
- Expand core OpenTofu E2E coverage to 1.12.11 on Linux AMD64 while retaining 1.13.2 coverage on AMD64/ARM64; document exact compatibility boundaries.
- Add a pinned Go 1.27.2 cross-platform release-build script with repeated-build comparison, SHA-256 checksums, build metadata, and CI verification.
- Document the current OpenTofu 1.13.2 compatibility baseline and release provenance limitations; no signed releases or independent reproducibility claims are made.
- Add a project roadmap that records P3 as partially validated and real AWS S3 checks as pending, and starts the P4 quality/distribution phase.
- Add a reproducible AWS S3 validation runbook; it is explicitly marked as prepared but not executed.
- Clarify in the README that S3Mock CI coverage does not establish AWS S3, IAM, bucket-policy, versioning, or S3 lockfile compatibility.
- Bind v2 wrapped-DEK payloads to an operator-configured context ID and reject mismatched contexts before returning keys.
- Reject legacy v1 metadata by default and provide an explicit, documented v1-to-v2 migration opt-in.
- Add real NEXUS-Q end-to-end coverage for context mismatch, tampered external context metadata, and legacy migration.
- Update design, security, and threat-model documentation to distinguish configured context binding from authenticated OpenTofu workspace or state identity.

- Refactor the provider protocol flow into a testable execution function.
- Add end-to-end protocol tests for initial key exchange, previous-key recovery, and rotation.
- Add GitHub Actions CI for Go tests, vet, race detection, and cross-platform builds.
- Document current source and dependency provenance.
- Require Go 1.26 or newer and test the supported Go 1.26/1.27 toolchains.
- Enforce a strict 1 MiB limit and a single JSON document for OpenTofu metadata input.
- Strengthen protocol-header and malformed-input tests.
- Update GitHub Actions to current major releases and disable unused Go module caching.
- Add native Linux AMD64/ARM64 OpenTofu E2E automation for encrypted state, saved plans, DEK recovery/rotation, and HTTP-backend persistence using test-only fixtures.
- Add a real NEXUS-Q v4.1.2 E2E CI job using a pinned source commit and ephemeral vault, credentials, and wrapping keys; cover authenticated API integration, state/saved-plan encryption, key rotation, server restart recovery, HTTP-backend persistence, and audit verification.

## [0.1.0] - Unreleased

- Initial experimental OpenTofu External Key Provider v1 implementation.
- NEXUS-Q-backed wrapping and recovery of OpenTofu data-encryption keys.
- HTTP and protocol validation with fail-closed behavior.
- Local integration fixtures and operational/security documentation.
