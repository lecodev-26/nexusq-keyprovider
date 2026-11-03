# fnox adapter

**Status: research / planned — not implemented.**

This directory is reserved for a fnox-specific adapter. It is part of the existing `nexusq-keyprovider` repository; it is not a separate project or repository.

## Why fnox

fnox is a Rust secret manager with a documented custom-provider extension point implemented as Lua 5.4 plugins. Its current plugin contract exposes:

- `get(ctx, ref)` — retrieve a secret, or return `nil` when it does not exist.
- `encrypt(ctx, value)` — return ciphertext for storage in `fnox.toml`.
- `test(ctx)` — validate provider connectivity.
- Optional `get_batch(ctx, refs)` — resolve multiple references in one call.

When a plugin defines `encrypt`, fnox treats it as an encryption provider. The returned ciphertext is stored in configuration; the plugin's `get` hook must accept that ciphertext reference and recover the original value. There is no separate decrypt hook. This differs from OpenTofu's External Key Provider protocol and must remain a separate adapter implementation.

Upstream contract inspected from fnox source revision `e080553b2c4c788407da91027284603dc895ebd4`:

- [Lua plugin documentation](https://github.com/jdx/fnox/blob/e080553b2c4c788407da91027284603dc895ebd4/docs/providers/lua.md)
- [Lua provider implementation](https://github.com/jdx/fnox/blob/e080553b2c4c788407da91027284603dc895ebd4/crates/fnox-core/src/providers/lua/mod.rs)

## Proposed integration flow

1. fnox calls `encrypt(ctx, value)` when a secret is set or re-encrypted.
2. The Lua plugin encodes a versioned, context-bound payload and sends it to NEXUS-Q `POST /v1/encrypt`.
3. The plugin returns the NEXUS-Q envelope as ciphertext for fnox to store in `fnox.toml`.
4. When fnox resolves the secret, `get(ctx, ref)` sends the envelope to NEXUS-Q `POST /v1/decrypt`.
5. The plugin validates the decoded payload and configured context before returning the secret value.

This is a proposed mapping, not implemented or yet validated against a running NEXUS-Q service. The exact API request/response contract, NEXUS-Q metadata label, payload schema, maximum secret size, HTTP redirect behavior, authentication failures, and Lua runtime edge cases must be verified before implementation is considered complete.

## Security and implementation decisions still required

- Use HTTPS for remote NEXUS-Q endpoints; allow plain HTTP only for loopback development if the API and Lua HTTP client behavior can enforce that policy.
- Never log plaintext secrets, bearer tokens, or decrypted payloads.
- Keep credentials outside `fnox.toml`; use a protected environment variable or another supported secret source.
- Define a distinct fnox payload format and NEXUS-Q metadata label. Do not reuse OpenTofu-specific metadata identifiers.
- Bind ciphertext to an explicit context identifier and validate the binding after decryption.
- Validate response status, JSON shape, Base64, payload version, context, and size limits; fail closed on any mismatch.
- Add tests for round-trip encryption, wrong context, malformed envelopes, API errors, authentication failures, missing configuration, and ciphertext tampering.
- Test with the actual fnox CLI and its supported plugin runtime before claiming compatibility.

## Status

The upstream Lua extension contract has been inspected. No Lua provider implementation, integration tests, fnox issue, or pull request has been created yet. The adapter remains planned until the API mapping and security behavior are proven end to end.
