# Context Binding Design (P2)

Status: implemented in the provider and exercised by unit tests plus the real NEXUS-Q CI E2E suite. This document records the design and its limitations; it is not a claim of authenticated workspace or state identity.
Target NEXUS-Q API: v4.1.2 (source commit `8735e5537bc687ee906ed68bdf119fc8956d9439`).

## 1. Problem

The current provider stores `format_version: 1`, a NEXUS-Q envelope, and a wrapping-key ID. The NEXUS-Q encrypt request uses a constant application-metadata label. The DEK plaintext is only its Base64 representation.

NEXUS-Q envelopes authenticate their header metadata as AEAD associated data. However, the v4.1.2 HTTP decrypt endpoint returns only the decrypted plaintext; it does not return the authenticated header metadata to this companion. Consequently, the companion cannot use the envelope header metadata as a context assertion without changing the NEXUS-Q API.

OpenTofu External Key Provider protocol v1 also does not provide an authenticated backend, workspace, state-lineage, or state-object identity to the external process. The companion must not pretend that a configured label is an identity asserted by OpenTofu.

## 2. Security objective and limits

Bind each wrapped DEK to an operator-configured, stable trust-domain identifier. A copied envelope/metadata pair must fail to yield a DEK when used with a different configured context.

This is a **configured context binding**, not proof of the actual OpenTofu workspace or backend identity. Correct isolation depends on operators assigning distinct values and protecting each provider configuration.

This design does not prevent:
- replay or rollback of an older valid envelope within the same context;
- a compromised host, provider process, NEXUS-Q server, or bearer token from exposing plaintext keys;
- an operator from deliberately configuring the same context identifier for unrelated states;
- a malicious actor with control of the OpenTofu configuration and provider environment from selecting a different context.

## 3. Configuration

Introduce required environment variable:

- `NEXUSQ_CONTEXT_ID`: opaque, stable identifier for one trust domain. Generate a random UUID or similarly collision-resistant identifier; do not use sensitive workspace names, account IDs, or secrets. Keep it unchanged for the lifetime of the encrypted state and saved plans. Use different values for states that must not share DEKs.

The value is visible in OpenTofu provider metadata and is not a secret. It is security-sensitive configuration and must be provisioned consistently with the state.

## 4. Metadata and encrypted payload v2

OpenTofu `external_data`:

```json
{
  "format_version": 2,
  "context_id": "opaque-stable-context-id",
  "envelope": "<NEXUS-Q envelope>",
  "wrapping_key_id": "<NEXUS-Q key ID>"
}
```

The plaintext sent to NEXUS-Q is a Base64 encoding of a JSON payload. The JSON `dek` field is a 32-byte Go `[]byte` encoded by JSON as Base64:

```json
{
  "format": "nexusq-opentofu-ekp",
  "version": 2,
  "context_id": "opaque-stable-context-id",
  "dek": "<base64-encoded 32-byte DEK>"
}
```

NEXUS-Q encrypts this payload in its envelope. The context assertion is therefore inside the encrypted-and-authenticated payload, not merely in editable OpenTofu metadata. The provider also sets a version-specific NEXUS-Q envelope metadata label for diagnostics, but does not rely on that label for validation because the current decrypt API does not return it.

## 5. Validation order

For v2 metadata, the provider must:
1. Validate metadata structure, exact supported version, non-empty envelope, wrapping key ID, and external `context_id`.
2. Require `NEXUSQ_CONTEXT_ID` and compare it with external `context_id` before contacting NEXUS-Q.
3. Decrypt the envelope through NEXUS-Q.
4. Decode the API's Base64 plaintext and parse exactly one payload JSON document.
5. Require the exact payload format and version, a 32-byte DEK, and payload `context_id` equal to the configured context.
6. Return no keys if any check fails.
7. On success, generate a fresh DEK, wrap a new v2 payload with the same configured context, and return the prior DEK for OpenTofu decryption.

Both context comparisons are necessary: the external metadata comparison is an early rejection; the encrypted payload comparison is the cryptographic binding.

## 6. Legacy v1 migration

A v1 envelope contains only the Base64-encoded DEK and has no authenticated context assertion. It is impossible to infer or verify its original trust domain retroactively.

Migration is therefore explicit and operator-authorized:
- Require `NEXUSQ_ALLOW_LEGACY_V1=true` and a non-empty `NEXUSQ_CONTEXT_ID`.
- Reject v1 by default.
- When the opt-in is present, recover the legacy DEK, then immediately return a newly wrapped v2 envelope bound to the configured context.
- Document that this first migration invocation is a trust-on-operator decision: it cannot detect whether a legacy envelope was copied from another state before migration.
- Remove the opt-in after state and saved-plan metadata have been rewritten and verified. Keep the old NEXUS-Q wrapping key until all dependent artifacts have been migrated or retired.

Unknown versions, malformed v1/v2 metadata, and v1 without the explicit opt-in fail closed. The migration flag must not weaken v2 validation.

## 7. Test requirements

- Initial v2 exchange stores the configured context in external metadata and encrypted payload.
- Correct context restores the previous DEK.
- Different configured context rejects before returning any keys.
- Tampered external context rejects before an API call.
- Tampered encrypted payload/context fails authentication or context validation.
- Missing context rejects.
- Unknown versions and malformed fields reject.
- v1 rejects by default; explicit migration recovers and rewraps to v2.
- v1 migration without context rejects.
- Rotation preserves context binding and old-key recovery.
- Real NEXUS-Q E2E covers correct context, wrong context, v1 migration, key rotation, saved plans, and server restart.

## 8. Memory handling

The implementation should clear mutable byte buffers containing DEKs and decrypted payloads as soon as practical. This is best-effort hygiene only. Go may copy values, retain strings, or optimize memory operations; the project must not claim guaranteed zeroization.

## 9. NEXUS-Q API compatibility

No NEXUS-Q API change is required. The existing encrypt endpoint accepts application metadata and plaintext strings; the decrypt endpoint returns the decrypted plaintext. The context is embedded in that plaintext payload so the companion can validate it after decrypt.

The NEXUS-Q envelope's authenticated header metadata remains useful as authenticated envelope metadata, but the current HTTP API does not expose it in decrypt responses. A future API enhancement returning verified envelope metadata could provide an additional independent check, not a replacement for validating the encrypted payload.
