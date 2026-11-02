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

**Status: next phase; work in progress.**

Priorities:

1. Define and test an explicit OpenTofu compatibility matrix, starting with the currently exercised 1.13.2 release and selected earlier/later supported releases.
2. Make release builds reproducible and record toolchain, source revision, target platform, and build commands.
3. Publish checksums and verifiable provenance for release artifacts; do not imply provenance merely from a successful cross-build.
4. Document installation, configuration, upgrade, rollback, and troubleshooting for each supported platform.
5. Keep platform claims precise: cross-compilation is not runtime testing on that platform.

## P5 — External review

**Status: pending.**

- Request design and protocol feedback from OpenTofu maintainers/community.
- Seek feedback on External Key Provider interoperability.
- Arrange an independent security review before any production-readiness claim.
- Revisit open P3 AWS checks when an authorized test environment is available.

## Release and production boundary

This repository remains an experimental integration prototype. Passing CI, S3Mock coverage, and a prepared AWS runbook do not establish production readiness, an independent security audit, or AWS service compatibility.
