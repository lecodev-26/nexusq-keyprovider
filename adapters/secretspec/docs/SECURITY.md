# Security notes and threat model

**Status: experimental; not independently audited or approved for production.**

## Assets and boundaries

- Plaintext secrets received from and returned to SecretSpec.
- NEXUS-Q bearer token, key ID and context ID.
- Encrypted envelopes persisted in the local filesystem.
- SecretSpec's private IPC session and the NEXUS-Q HTTPS connection.
- The adapter binary and its registration claim.

## Controls implemented

- HTTPS is required for non-loopback endpoints; HTTP is allowed only for loopback.
- TLS certificate validation is not disabled.
- HTTP redirects are disabled to prevent forwarding bearer credentials or secret-bearing request bodies.
- Request and response bodies are bounded; response chunks are checked while streaming.
- API errors and transport failures are returned without printing response bodies, credentials or secret values.
- Secret references are stored as Base64 ciphertext envelopes, not plaintext.
- Payloads bind adapter name/version, configured context ID and a SHA-256 digest of the exact SecretSpec address inside the NEXUS-Q-encrypted plaintext.
- Address binding prevents copying an envelope to another SecretSpec address from returning a valid value.
- The local store uses opaque SHA-256 filenames, restrictive Unix directory/file modes, same-directory atomic replacement, and rejects symlinked roots and records.
- SecretSpec's URI is checked by the protocol implementation and the adapter never logs it.

## Residual risks and limitations

- The NEXUS-Q bearer token may have broader decrypt authority than one SecretSpec project. Use a dedicated token/key/context per trust domain where possible.
- Context/address binding is checked after NEXUS-Q decrypts. It is not a substitute for server-side authorization.
- An older valid envelope can be replayed for the same address/context; rollback protection is not implemented.
- The local ciphertext store is not synchronized, replicated or backed up automatically.
- Expiry is local metadata and can be altered by an attacker who can write the ciphertext directory.
- Unix mode checks do not replace a full ownership/ACL audit of every ancestor directory. Windows ACL hardening has not yet been implemented or tested.
- Filesystem durability and locking behavior have not been tested on every supported OS/filesystem.
- The provider IPC protocol is UTF-8 string based here; arbitrary binary values are not supported.
- Plaintext and serialized request buffers may exist transiently in process memory. Not all intermediate allocations are guaranteed to be zeroized.
- The adapter has not yet passed SecretSpec's provider conformance suite or a real-service end-to-end integration test.
- No independent security review, audit, certification, upstream endorsement or production-readiness claim is made.

## Operational guidance

- Keep the provider registration file and executable in user-controlled, non-world-writable locations.
- Supply secrets through a secret manager or protected process environment; do not put credentials in the provider URI, claim file, shell history or command-line arguments.
- Restrict NEXUS-Q tokens and keys to the narrowest practical trust domain.
- Back up the ciphertext store and separately test recovery before relying on it.
- Preserve old NEXUS-Q keys until all dependent envelopes have been migrated and verified.
- Treat the adapter binary and its source as privileged code: SecretSpec external providers run with the user's operating-system permissions.
