# Design

## Trust boundaries

- OpenTofu owns state serialization and AES-GCM encryption.
- This executable implements the External Key Provider v1 process protocol.
- NEXUS-Q owns wrapping/unwrapping of the DEK through its envelope API.
- The bearer token authenticates the companion to the NEXUS-Q protected API; it is not a per-user identity or mTLS credential.

## Initial invocation

1. OpenTofu starts the executable and sends null metadata on stdin.
2. The provider emits a fresh random 32-byte DEK.
3. The provider sends the Base64 representation of that DEK to NEXUS-Q `POST /v1/encrypt`.
4. NEXUS-Q returns an envelope in its `data` field.
5. The provider returns the DEK as `keys.encryption_key` and the envelope as versioned `meta.external_data`.

## Subsequent invocation

1. OpenTofu sends the prior provider metadata.
2. The provider calls NEXUS-Q `POST /v1/decrypt` with the stored envelope.
3. The provider decodes the NEXUS-Q response and recovers the previous 32-byte DEK.
4. The provider generates and wraps a fresh DEK.
5. The provider returns the fresh DEK as `encryption_key`, the previous DEK as `decryption_key`, and refreshed metadata.

## Metadata format

```json
{
  "external_data": {
    "format_version": 1,
    "envelope": "<NEXUS-Q envelope>",
    "wrapping_key_id": "<NEXUS-Q key ID>"
  }
}
```

No bearer token or plaintext DEK is stored in metadata.

## Failure behavior

Invalid configuration, unsupported metadata, HTTP/API errors, malformed API responses, oversized responses, invalid Base64, and recovered DEKs of an unexpected length terminate the provider with a non-zero exit status. Diagnostics go to stderr; stdout is reserved for the protocol header and response.

## Operational considerations

- Preserve old NEXUS-Q wrapping keys until all dependent state and plan artifacts are migrated or retired.
- State and saved plan artifacts use separate OpenTofu encryption settings; the same AES-GCM method can be configured for both. Both were exercised in the local end-to-end test.
- A provider invocation can wrap a new DEK even during a plan. Only metadata persisted by OpenTofu with a state/plan artifact becomes durable.
- Four concurrent local plan processes were exercised successfully, but sustained load, distributed CI, and rate-limit behavior under production traffic still require testing.
- OpenTofu's HTTP backend protocol was tested with a loopback-only development fixture. Remote state was persisted encrypted, read from a clean workspace, and recovered after restarting the fixture. This does not validate production remote backend services.
- The current implementation uses standard Go byte slices; it does not claim guaranteed memory zeroization.
