> **Scope:** This document describes the OpenTofu adapter only. It is not a universal adapter protocol or a design specification for every integration in this repository.

# Design

## Trust boundaries

- OpenTofu owns state serialization and AES-GCM encryption.
- This executable implements the OpenTofu External Key Provider protocol v1 process protocol.
- NEXUS-Q owns wrapping/unwrapping of the DEK through its envelope API.
- The bearer token authenticates the companion to the NEXUS-Q protected API; it is not a per-user identity, per-state authorization, or mTLS credential.
- `NEXUSQ_CONTEXT_ID` is operator-provisioned configuration. The protocol does not provide an authenticated workspace, backend, state-lineage, or state-object identity to this process.

## Configuration

Required environment variables:

- `NEXUSQ_URL`: NEXUS-Q base URL.
- `NEXUSQ_TOKEN`: bearer token for the protected API.
- `NEXUSQ_KEY_ID`: active NEXUS-Q wrapping-key ID used for new envelopes.
- `NEXUSQ_CONTEXT_ID`: stable opaque identifier for one state trust domain. It must contain 1–128 ASCII letters, digits, dot, underscore, colon, or hyphen. Use a distinct random identifier for each state that must be isolated. Keep it stable across runs, state operations, and saved plans associated with that state. It is visible in provider metadata and is not a secret.

Optional migration variable:

- `NEXUSQ_ALLOW_LEGACY_V1`: defaults to false. Set to exactly `true` only while deliberately migrating legacy v1 metadata.

## Initial invocation

1. OpenTofu starts the executable and sends null metadata on stdin.
2. The provider generates a fresh random 32-byte DEK.
3. It creates a version 2 payload containing the format identifier, payload version, configured context ID, and DEK.
4. The provider Base64-encodes that JSON payload and sends it to NEXUS-Q `POST /v1/encrypt`, with the v2 application-metadata label.
5. NEXUS-Q returns an envelope.
6. The provider returns the DEK as `keys.encryption_key` and stores v2 metadata in `meta.external_data`.

## Subsequent invocation

1. OpenTofu sends the prior provider metadata.
2. The provider validates the exact metadata schema and supported version.
3. For v2, it requires the external context ID to match `NEXUSQ_CONTEXT_ID` before contacting NEXUS-Q.
4. It asks NEXUS-Q `POST /v1/decrypt` to unwrap the envelope.
5. It decodes the encrypted payload and validates its format, version, DEK length, and embedded context ID against the configured context.
6. Only after successful validation does the provider generate and wrap a fresh DEK.
7. It returns the fresh DEK as `encryption_key`, the previous DEK as `decryption_key`, and refreshed v2 metadata.

The external context comparison is an early rejection. The context ID inside the NEXUS-Q-encrypted payload is the cryptographic binding. Both are required.

## Metadata format v2

OpenTofu `external_data`:

```json
{
  "format_version": 2,
  "context_id": "opaque-stable-context-id",
  "envelope": "<NEXUS-Q envelope>",
  "wrapping_key_id": "<NEXUS-Q key ID>"
}
```

The encrypted payload plaintext is a Base64 encoding of this JSON structure. The `dek` field is encoded as Base64 by Go's JSON handling for `[]byte`:

```json
{
  "format": "nexusq-opentofu-ekp",
  "version": 2,
  "context_id": "opaque-stable-context-id",
  "dek": "<base64-encoded 32-byte DEK>"
}
```

The external context ID is visible and editable; it is not trusted by itself. A changed external value cannot change the context ID inside the encrypted payload. NEXUS-Q v4.1.2 authenticates envelope header metadata, but its HTTP decrypt endpoint returns only plaintext, not the authenticated header metadata. The companion therefore validates the context inside the encrypted payload and does not depend on the envelope header metadata being returned.

No bearer token or plaintext DEK is stored in OpenTofu metadata.

## Legacy metadata v1 migration

V1 metadata contains a NEXUS-Q envelope whose plaintext is only the Base64-encoded DEK. It has no context binding, and its original trust domain cannot be verified retroactively.

V1 is rejected by default. To migrate intentionally:

1. Back up the state and all saved-plan artifacts.
2. Configure the correct stable `NEXUSQ_CONTEXT_ID`.
3. Set `NEXUSQ_ALLOW_LEGACY_V1=true` only for the migration window.
4. Run the OpenTofu operations needed to rewrite state metadata and regenerate any saved plans that must remain usable. Retire old saved-plan artifacts that are not regenerated.
5. Verify that persisted metadata is v2 and that the state and newly generated plans can be read.
6. Unset `NEXUSQ_ALLOW_LEGACY_V1`.

The first v1 migration is an operator-trusted transition. It cannot detect whether a legacy envelope was copied from another state before migration. Do not use migration to legitimize an envelope whose provenance is unknown. Preserve old NEXUS-Q wrapping keys until all dependent state and plan artifacts have been migrated or retired.

## Failure behavior

Invalid configuration, unsupported or malformed metadata, context mismatch, HTTP/API errors, malformed API responses, oversized responses, invalid Base64, malformed wrapped payloads, and recovered DEKs of an unexpected length terminate the provider with a non-zero exit status. Diagnostics go to stderr; stdout is reserved for the protocol header and response. No keys are emitted on validation or API failure.

## Memory handling

The implementation clears mutable byte buffers containing DEKs, decrypted payloads, and marshaled request bodies when practical. This is best-effort hygiene only. Go may copy values, retain strings, or optimize memory operations; the project does not claim guaranteed zeroization.

## Operational considerations

- Keep the context ID stable for each trust domain. Changing it intentionally makes existing v2 metadata fail closed; it is not a routine rotation mechanism.
- Use different context IDs for separate states that must not share DEKs. The context ID is not authorization: NEXUS-Q token permissions and vault/key isolation remain separate controls.
- Preserve old NEXUS-Q wrapping keys until all dependent state and plan artifacts are migrated or retired.
- State and saved-plan artifacts use separate OpenTofu encryption settings; both should use a provider configuration with the same context ID when they belong to the same trust domain.
- A provider invocation can wrap a new DEK even during a plan. Only metadata persisted by OpenTofu with a state/plan artifact becomes durable.
- Replay or rollback of an older valid envelope within the same context is not prevented by this design. Backend versioning, write permissions, audit, and recovery controls remain necessary.
- Four concurrent local plan processes were exercised successfully, but sustained load, distributed CI, and rate-limit behavior under production traffic still require testing.
- OpenTofu's HTTP backend protocol was tested with a loopback-only development fixture. Remote state was persisted encrypted, read from a clean workspace, and recovered after restarting the fixture. This does not validate production remote backend services.
