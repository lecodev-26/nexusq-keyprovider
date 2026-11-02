# Changelog

All notable changes to this project will be documented in this file.

## Unreleased

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
