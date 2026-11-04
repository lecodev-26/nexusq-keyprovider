# Design

## Scope

Implement SecretSpec's external secretspec.provider/1 protocol without changing SecretSpec or the NEXUS-Q engine. The adapter combines NEXUS-Q envelope encryption with a local ciphertext-only persistence layer.

## Address model

- Convention address: project/profile/key.
- Native address: coordinates.item.
- Other native coordinates are rejected by advertising no supported coordinate selectors.

A canonical serialization of the original address is hashed and used as the address-binding value in the encrypted payload. The local filename hashes the provider URI plus the serialized address, keeping filenames opaque and isolating configured provider instances.

## Envelope format

The NEXUS-Q encrypted plaintext is a JSON object containing:

- adapter: nexusq-secretspec
- version: 1
- context_id: configured trust-domain identifier
- address_binding: SHA-256 digest of the canonical SecretSpec address
- secret: UTF-8 secret value

NEXUS-Q receives this JSON as the plaintext field of POST /v1/encrypt, together with key_id and fixed metadata. The returned Base64 envelope is stored in a JSON record with a format version and optional expiry timestamp. Reads reverse this flow and validate every binding before returning plaintext.

## Persistence

The store contains one record per address. Writes use a unique temporary file in the same directory, restrictive Unix mode, file fsync, and atomic rename. This prevents readers from observing partially written JSON. The provider does not currently advertise a bulk clear operation because the store intentionally has no plaintext index or manifest of addresses.

## Expiry

The set_expiring operation stores an absolute expiry timestamp beside the ciphertext. Reads and presence checks treat expired entries as absent and attempt to remove them. This is an adapter-side validity policy, not a cryptographic expiry guarantee.

## Deliberate omissions

No reflection, batch read, clear-all, or arbitrary binary-secret support is advertised in this first iteration. These features should be added only with explicit semantics and conformance coverage.
