# Security Policy

## Project status

NEXUS-Q Key Provider is experimental software. The current implementation supports the OpenTofu External Key Provider protocol v1 and has not undergone an independent security audit. Do not assume it is suitable for production or regulated workloads.

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
- Preserve old wrapping keys while encrypted state or plan artifacts still depend on them.
- Review the README's validation boundaries before deployment.


## Threat model

### Assets

- Plaintext 256-bit DEKs while they are held by the provider or OpenTofu.
- The NEXUS-Q bearer token used to authorize envelope operations.
- NEXUS-Q envelopes and OpenTofu provider metadata.
- Encrypted state and saved-plan artifacts, including their availability for recovery.

### Trust boundaries and assumptions

- The host running OpenTofu and this executable is trusted. The operating system is responsible for process isolation and protection of environment variables.
- The provider executable and its installation path are trusted and must not be writable by untrusted users.
- NEXUS-Q is trusted to authenticate callers, enforce token permissions, protect wrapping keys, and correctly implement authenticated envelope encryption.
- Remote NEXUS-Q connections use HTTPS with Go's normal certificate and hostname verification. The provider does not install custom roots or disable certificate checks.
- Operators provision a dedicated, least-privilege token and retain old wrapping keys for as long as encrypted artifacts need them.

### Mitigations in this prototype

- Remote plaintext HTTP is rejected; HTTP is accepted only for loopback development endpoints.
- HTTP redirects are not followed, preventing the bearer token from being forwarded to a redirect target.
- Provider metadata and API response bodies are bounded to 1 MiB; API calls have a 15-second timeout.
- Metadata must contain exactly one JSON document. Unsupported metadata versions, missing envelopes, invalid Base64, malformed API responses, and recovered keys that are not exactly 32 bytes fail closed.
- API error bodies are not copied into diagnostics. Standard output is reserved for the OpenTofu protocol; errors are written to standard error.
- Fresh DEKs are generated with Go's cryptographically secure random source.

### Out of scope and residual risks

- A compromised host, malicious provider binary, privileged local process, debugger, or process-memory dump can expose plaintext DEKs or environment variables.
- Go does not guarantee that key material in byte slices is securely zeroized.
- Loopback HTTP is not protected against a hostile local process able to observe or impersonate the local endpoint. Use HTTPS where the local host is not fully trusted.
- A stolen bearer token can be used within the permissions granted to it. The provider does not implement mutual TLS or per-user identity.
- A compromised NEXUS-Q server, wrapping key, token issuer, host trust store, or DNS/network control plane is outside this provider's protection boundary.
- The provider does not independently bind an envelope to a workspace, state lineage, or deployment identity. Access control and operational separation must be enforced by the NEXUS-Q deployment and the surrounding infrastructure.
- Availability attacks, rate limiting, backup strategy, disaster recovery, and production remote-backend behavior require deployment-level controls and separate validation.
- For S3-compatible storage, OpenTofu's enforced state/plan encryption protects the payload, not backend credentials, object names, access patterns, or access logs. Use TLS for remote endpoints and configure least-privilege storage access, versioning, recovery, audit logging, and server-side encryption independently.
- The S3Mock integration fixture does not validate AWS IAM, bucket policies, versioning, Object Lock, SSE-KMS, replication, lifecycle rules, or S3 lock contention. See [Threat Model Review Notes](docs/THREAT-MODEL-REVIEW.md) for the internal review findings and validation boundaries.

This threat model documents the current prototype's boundaries; it is not a security audit, penetration test, or certification.
