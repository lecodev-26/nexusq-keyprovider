# OpenTofu adapter security policy and threat model

## Project status

The OpenTofu adapter in this directory is experimental software. It implements the OpenTofu External Key Provider protocol v1 and uses NEXUS-Q to wrap and recover OpenTofu DEKs. It has not undergone an independent security audit. Do not assume it is suitable for production or regulated workloads.

## Reporting a vulnerability

Please do not report security vulnerabilities in public issues. Use GitHub's private vulnerability reporting for this repository:

https://github.com/lecodev-26/nexusq-keyprovider/security/advisories/new

Do not disclose exploit details publicly. Include:
- A description of the affected component and impact.
- Reproduction steps or a proof of concept, where safe.
- Relevant versions and environment details.

Do not include real credentials, production keys, state files, or other sensitive data in a report.

## Operational guidance

- Require HTTPS for NEXUS-Q endpoints outside loopback.
- Keep NEXUS-Q bearer tokens out of source control, command-line arguments, HCL, and provider metadata.
- Restrict access to the NEXUS-Q server and use a dedicated vault/token where practical.
- Set a stable, random `NEXUSQ_CONTEXT_ID` for each state trust domain. Keep it unchanged across runs and use different values for states that must be isolated. It is visible in provider metadata and is not a secret.
- Do not treat a context ID as a substitute for NEXUS-Q authorization, separate vaults, backend access controls, or host isolation.
- Reject v1 metadata by default. Enable `NEXUSQ_ALLOW_LEGACY_V1=true` only for an intentional migration with known legacy provenance, then unset it after all dependent state and saved plans have been migrated or retired.
- Preserve old wrapping keys while encrypted state or plan artifacts still depend on them.
- Review the adapter [README](../README.md) and its validation boundaries before deployment.

## Threat model

### Assets

- Plaintext 256-bit DEKs while they are held by the provider or OpenTofu.
- The NEXUS-Q bearer token used to authorize envelope operations.
- NEXUS-Q envelopes and OpenTofu provider metadata, including the visible context ID.
- Encrypted state and saved-plan artifacts, including their availability for recovery.

### Trust boundaries and assumptions

- The host running OpenTofu and this executable is trusted. The operating system is responsible for process isolation and protection of environment variables.
- The provider executable and its installation path are trusted and must not be writable by untrusted users.
- NEXUS-Q is trusted to authenticate callers, enforce token permissions, protect wrapping keys, and correctly implement authenticated envelope encryption.
- Remote NEXUS-Q connections use HTTPS with Go's normal certificate and hostname verification. The provider does not install custom roots or disable certificate checks.
- Operators provision a dedicated, least-privilege token, configure distinct context IDs for separate trust domains, and retain old wrapping keys for as long as encrypted artifacts need them.

### Mitigations in this prototype

- Remote plaintext HTTP is rejected; HTTP is accepted only for loopback development endpoints.
- HTTP redirects are not followed, preventing the bearer token from being forwarded to a redirect target.
- Provider metadata and API response bodies are bounded to 1 MiB; API calls have a 15-second timeout.
- Metadata must contain exactly one JSON document. The external metadata schema and wrapped payload are strictly checked; unsupported versions, unknown fields, missing values, invalid Base64, malformed API responses, context mismatches, and recovered keys that are not exactly 32 bytes fail closed.
- V2 embeds the configured context ID inside the NEXUS-Q-encrypted payload and compares it after decryption. Editing the visible external context ID alone cannot change the encrypted assertion.
- V1 metadata is rejected unless the operator explicitly enables `NEXUSQ_ALLOW_LEGACY_V1=true` for migration. The migration path immediately emits v2 metadata.
- API error bodies are not copied into diagnostics. Standard output is reserved for the OpenTofu protocol; errors are written to standard error.
- Fresh DEKs are generated with Go's cryptographically secure random source.
- Mutable byte buffers containing key material and request payloads are cleared when practical. This is best-effort hygiene, not guaranteed zeroization.

### Out of scope and residual risks

- The context ID is operator-provisioned configuration, not an authenticated identity supplied by OpenTofu. The provider cannot prove that a configured context corresponds to a particular workspace, backend object key, state lineage, account, or deployment.
- Replaying or rolling back an older valid envelope within the same configured context is not prevented. Backend versioning, write permissions, audit, and recovery controls remain necessary.
- A malicious actor who can change both the provider configuration and its environment may select another context. Protect configuration and process launch policy.
- A v1 envelope has no original context assertion. The explicit migration path is a trust-on-operator transition and cannot detect whether an envelope was copied from another state before migration. Migrate only artifacts with known provenance.
- Context binding is not NEXUS-Q authorization. The companion validates context after NEXUS-Q decrypts the envelope; a stolen bearer token may still unwrap any envelope permitted by its NEXUS-Q policy. Use separate vaults, keys, and tokens where stronger isolation is required.
- A compromised host, malicious provider binary, privileged local process, debugger, or process-memory dump can expose plaintext DEKs or environment variables.
- Go does not guarantee that key material in byte slices or strings is securely zeroized.
- Loopback HTTP is not protected against a hostile local process able to observe or impersonate the local endpoint. Use HTTPS where the local host is not fully trusted.
- A compromised NEXUS-Q server, wrapping key, token issuer, host trust store, or DNS/network control plane is outside this provider's protection boundary.
- Availability attacks, rate limiting, backup strategy, disaster recovery, and production remote-backend behavior require deployment-level controls and separate validation.
- For S3-compatible storage, OpenTofu's enforced state/plan encryption protects the payload, not backend credentials, object names, access patterns, or access logs. Use TLS for remote endpoints and configure least-privilege storage access, versioning, recovery, audit logging, and server-side encryption independently.
- The S3Mock integration fixture does not validate AWS IAM, bucket policies, versioning, Object Lock, SSE-KMS, replication, lifecycle rules, or S3 lock contention. See [Threat Model Review Notes](THREAT-MODEL-REVIEW.md) for the internal review findings and validation boundaries.

This threat model documents the current prototype's boundaries; it is not a security audit, penetration test, or certification.
