# Threat Model Review Notes

**Review type:** internal design and integration review
**Project state:** experimental prototype
**Date:** 2026-11-02
**Independence:** performed as part of project development; not an independent assessment, penetration test, or security audit.

## Scope and evidence

This review considers the provider process, its OpenTofu External Key Provider boundary, NEXUS-Q envelope API, provider metadata, and remote state storage. It does not assess NEXUS-Q's internal cryptographic implementation, OpenTofu internals, cloud-provider IAM implementations, or a production deployment.

The current implementation uses context-bound v2 metadata. The Go unit tests cover the metadata and payload validation paths; CI also runs OpenTofu integration suites with a test API fixture, S3Mock, and a separately built NEXUS-Q v4.1.2 service from a pinned source commit. These tests are integration evidence, not a security certification.

## Findings and recommended controls

### TM-01 — Configured context binding is not authenticated workspace or state identity

**Severity:** medium; deployment-dependent
**Status:** v2 mitigation implemented; identity limitation remains

V2 metadata carries a visible `context_id`, and the same identifier is embedded in the plaintext payload encrypted by NEXUS-Q. On recovery, the provider first compares the visible metadata value with the configured `NEXUSQ_CONTEXT_ID`, then decrypts the envelope and verifies the context assertion inside the decrypted payload before returning keys. Editing only the visible metadata value cannot change the encrypted assertion.

This is a configured trust-domain binding, not proof of an OpenTofu workspace, backend object key, state lineage, account, or deployment identity. The External Key Provider process does not receive an authenticated identity for those objects. The operator provisions the context ID and must protect its configuration. Replaying or rolling back an older valid envelope within the same configured context is not prevented.

**Controls:** assign a distinct random context ID to each state trust domain that must be isolated; keep it stable for the lifecycle of the associated state and saved plans; protect provider configuration and process environment; isolate backend object paths and write permissions; use separate NEXUS-Q vaults, tokens, or wrapping keys where stronger separation is required; enable backend versioning, audit, and recovery controls.

### TM-02 — Host compromise exposes plaintext DEKs and credentials

**Severity:** high
**Status:** inherent trust assumption

The provider returns plaintext DEKs to OpenTofu and reads the NEXUS-Q bearer token from its process environment. A compromised host, privileged local process, debugger, malicious provider executable, or memory dump may recover them. The implementation clears mutable buffers when practical, but Go does not guarantee zeroization; values may be copied or retained.

**Controls:** use a dedicated execution identity and host; restrict executable and configuration write access; keep tokens short-lived and narrowly scoped where supported; protect process environments and crash dumps; avoid running untrusted workloads under the same identity.

### TM-03 — Bearer-token authority is not scoped by context ID

**Severity:** medium; depends on NEXUS-Q token policy
**Status:** documented residual risk

The provider authenticates with a bearer token and does not provide mutual TLS or a per-workspace identity. Context validation occurs after NEXUS-Q decrypts the envelope. A stolen token may therefore be used to unwrap any envelope allowed by its NEXUS-Q policy, even if the provider later rejects the decrypted payload because its context differs.

**Controls:** use dedicated tokens, vaults, and keys for separate environments or trust domains; grant only required envelope operations; rotate and revoke tokens through NEXUS-Q operational controls; protect token delivery to the provider process.

### TM-04 — Loopback HTTP trusts the local machine

**Severity:** medium
**Status:** documented development exception

HTTP is allowed for loopback NEXUS-Q endpoints to support local development and tests. A hostile process on the same host may observe or impersonate a loopback service depending on host isolation and process permissions.

**Controls:** use HTTPS even on the same host when local processes are not within the same trust boundary; restrict local process execution and network namespaces.

### TM-05 — S3-compatible backend behavior is not equivalent to AWS production validation

**Severity:** medium
**Status:** validation boundary

The CI S3 backend test exercises OpenTofu against an isolated Adobe S3Mock service. It checks encrypted state and saved-plan handling, plaintext-marker scans, recovery from a clean working directory after a service restart, and wrapping-key ID rotation. It does not validate AWS IAM, bucket policies, versioning, Object Lock, SSE-KMS, replication, lifecycle rules, multi-region behavior, production TLS configuration, or S3 locking under contention.

**Controls:** for an AWS deployment, separately validate IAM least privilege, TLS, bucket policy, versioning, recovery, audit logging, and locking in a non-production account. Keep backend credentials in the AWS credential chain or a workload identity mechanism rather than embedding them in HCL or committed backend configuration.

### TM-06 — Backend storage controls are independent of application-layer encryption

**Severity:** medium
**Status:** operational responsibility

OpenTofu's enforced state and saved-plan encryption protects their payloads before they are stored by the backend. It does not protect backend credentials, object names, access patterns, logs, or every piece of backend metadata. Server-side encryption at the storage provider is an additional layer, not a replacement for OpenTofu encryption or NEXUS-Q key management.

**Controls:** enforce TLS to remote S3 endpoints; configure storage-side encryption and access policies; enable versioning and appropriate retention/recovery controls; restrict read, write, and delete permissions; treat object names and access logs as potentially sensitive.

### TM-07 — Legacy v1 migration cannot establish historical context

**Severity:** medium; migration-dependent
**Status:** explicit opt-in implemented

Legacy v1 metadata contains no context assertion. It is rejected by default. When `NEXUSQ_ALLOW_LEGACY_V1=true` is explicitly enabled, the provider can recover the legacy DEK and immediately emit v2 metadata using the configured context. That first migration is a trust-on-operator transition: the provider cannot determine whether the v1 envelope was copied from another state before migration.

**Controls:** migrate only artifacts with known provenance; back up state and saved plans first; configure the intended context ID; enable the flag only for the migration window; rewrite and verify all dependent state and saved-plan artifacts; retire obsolete artifacts and unset the flag; retain old NEXUS-Q wrapping keys until no retained artifact depends on them.

## E2E acceptance and validation boundaries

The local HTTP-backend fixture exercises the OpenTofu HTTP backend protocol and recovery using a loopback-only development service. It is not a production remote-backend validation.

The S3Mock fixture uses generated test credentials and an isolated local service. It deliberately does not enable S3 locking. Neither the HTTP fixture nor S3Mock validates production cloud IAM, bucket policy, versioning, Object Lock, SSE-KMS, replication, lifecycle behavior, production TLS, or distributed locking.

The real NEXUS-Q CI job builds NEXUS-Q v4.1.2 from a pinned source commit and uses ephemeral test vaults, credentials, and wrapping keys. It exercises authenticated API operations, explicit v1 migration, wrong-context rejection, tampered external-context rejection through the encrypted payload assertion, state and saved-plan flows, wrapping-key rotation, server restart recovery, HTTP-backend persistence, and audit verification. It does not replace independent review of NEXUS-Q or a production deployment.

## Review conclusion

Context-bound v2 metadata improves separation between configured trust domains, but it does not authenticate OpenTofu workspace or state identity and does not prevent same-context replay or rollback. The principal residual risks remain the trusted-host assumption, bearer-token scope, operator-controlled context configuration, legacy migration provenance, and deployment-level backend controls. The project remains experimental and requires independent security review before any production-readiness claim.
