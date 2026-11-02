# Threat Model Review Notes

**Review type:** internal design and integration review  
**Project state:** experimental prototype  
**Date:** 2026-11-02  
**Independence:** this review was performed as part of project development. It is not an independent assessment, penetration test, or security audit.

## Scope

This review considers the provider process, its OpenTofu External Key Provider boundary, the NEXUS-Q envelope API, provider metadata, and remote state storage. It does not assess the cryptographic implementation inside NEXUS-Q, OpenTofu internals, cloud-provider IAM implementations, or the security of a production deployment.

## Findings and recommended controls

### TM-01 — Envelope metadata is not bound to a workspace or state lineage

**Severity:** medium; deployment-dependent  
**Status:** documented residual risk

The provider stores the NEXUS-Q envelope and wrapping-key ID in OpenTofu provider metadata. The provider does not add an authenticated binding to a workspace, backend key, state lineage, or deployment identity.

A party able to replace or replay state artifacts and their associated metadata may cause rollback, substitution, or availability problems. The provider alone cannot distinguish an artifact copied from another authorized state context.

**Controls:** isolate backend object paths and write permissions by trust domain; use separate NEXUS-Q vaults or narrowly scoped wrapping keys where stronger separation is required; enable backend versioning and recovery controls; protect state and saved-plan artifacts as sensitive data.

**Future work:** evaluate context-bound envelope support in NEXUS-Q, with a stable and migration-safe context identifier. Do not derive security context from untrusted state metadata alone.

### TM-02 — Host compromise exposes plaintext DEKs and credentials

**Severity:** high  
**Status:** inherent trust assumption

The provider returns plaintext DEKs to OpenTofu and reads the NEXUS-Q bearer token from its process environment. A compromised host, privileged local process, debugger, malicious provider executable, or memory dump may recover them. Go does not guarantee zeroization of byte slices.

**Controls:** use a dedicated execution identity and host; restrict executable and configuration write access; keep tokens short-lived and narrowly scoped where supported; protect process environments and crash dumps; avoid running untrusted workloads under the same identity.

### TM-03 — Bearer-token authority is not scoped by state identity

**Severity:** medium; depends on NEXUS-Q token policy  
**Status:** documented residual risk

The provider authenticates with one bearer token and does not supply mutual TLS or a per-workspace identity. If a token can wrap or unwrap arbitrary envelopes permitted by its NEXUS-Q policy, compromise of that token may affect more than one state.

**Controls:** use dedicated tokens and keys for separate environments or trust domains; grant only the required envelope operations; rotate and revoke tokens through NEXUS-Q operational controls; protect token delivery to the provider process.

### TM-04 — Loopback HTTP trusts the local machine

**Severity:** medium  
**Status:** documented development exception

HTTP is allowed for loopback NEXUS-Q endpoints to support local development and tests. A hostile process on the same host may observe or impersonate a loopback service depending on host isolation and process permissions.

**Controls:** use HTTPS even on the same host when local processes are not within the same trust boundary; restrict local process execution and network namespaces.

### TM-05 — S3-compatible backend behavior is not equivalent to AWS production validation

**Severity:** medium  
**Status:** validation boundary

The Adobe S3Mock E2E fixture exercises OpenTofu's S3 backend against a local implementation of a subset of the S3 API. It is intended to validate encrypted state/plan interoperability, object persistence, and recovery after a service restart. It does not validate AWS IAM, bucket policies, versioning, Object Lock, SSE-KMS, replication, lifecycle rules, multi-region behavior, production TLS configuration, or distributed locking under contention.

**Controls:** for an AWS deployment, separately validate IAM least privilege, TLS, bucket policy, versioning, recovery, audit logging, and locking in a non-production account. Keep backend credentials in the AWS credential chain or a workload identity mechanism rather than embedding them in HCL or committed backend configuration.

### TM-06 — Backend storage controls are independent of application-layer encryption

**Severity:** medium  
**Status:** operational responsibility

OpenTofu's enforced state/plan encryption protects the state and saved-plan payloads before they are stored by the backend. It does not protect backend credentials, object names, access patterns, logs, or every piece of backend metadata. Server-side encryption at the storage provider is an additional layer, not a replacement for OpenTofu encryption or NEXUS-Q key management.

**Controls:** enforce TLS to remote S3 endpoints; configure storage-side encryption and access policies; enable versioning and appropriate retention/recovery controls; restrict read, write, and delete permissions; treat object names and access logs as potentially sensitive.

## S3-compatible E2E acceptance boundary

The CI test uses fixed, test-only credentials and a local S3Mock endpoint. It must never be configured with real infrastructure state or production credentials. It checks that the saved plan and stored remote state do not contain a known plaintext marker, that OpenTofu can recover state from a clean working directory after an S3Mock restart, and that the provider can recover an earlier DEK while rotating the wrapping-key ID.

The test does not claim AWS S3 certification or validate S3 locking. The current fixture deliberately does not enable use_lockfile; concurrency and lock semantics remain separate validation work.

## Review conclusion

The principal risks remain the trusted-host assumption, bearer-token scope, lack of workspace/state-context binding, and deployment-level backend controls. The S3-compatible test improves integration evidence but does not reduce those architectural risks. The project remains experimental and requires independent security review before any production-readiness claim.
