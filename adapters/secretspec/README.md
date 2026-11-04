# NEXUS-Q provider for SecretSpec

**Status: experimental.** This is an independently maintained external provider for SecretSpec 0.21+, using the official secretspec.provider/1 IPC protocol. It is not an official SecretSpec integration and has not received an independent security audit.

The adapter stores one **NEXUS-Q encrypted envelope per SecretSpec address** in a local private directory. NEXUS-Q remains the cryptographic authority; the local directory is only the ciphertext store.

## Architecture

1. SecretSpec launches secretspec-provider-nexusq provider over private stdin/stdout IPC.
2. The adapter resolves convention addresses to project/profile/key; native addresses use their item coordinate.
3. provider.set sends the secret to NEXUS-Q's POST /v1/encrypt endpoint.
4. Only the returned Base64 envelope is written to the local store.
5. provider.get reads the envelope and calls NEXUS-Q's POST /v1/decrypt endpoint.
6. The decrypted payload is checked against the adapter version, configured context ID, and a hash of the exact SecretSpec address before plaintext is returned.

The local filename is a SHA-256 digest of the provider URI and address. Writes use a same-directory temporary file, mode 0600 on Unix, fsync, and atomic rename. The store directory is created with mode 0700 on Unix. Symlinked store roots and symlinked record files are rejected.

## Build

Requires Rust/Cargo and network access to crates.io:

    cd adapters/secretspec
    cargo build --release

The executable is target/release/secretspec-provider-nexusq.

## Configuration

Set these environment variables for the SecretSpec process:

| Variable | Purpose |
| --- | --- |
| NEXUSQ_URL | NEXUS-Q origin, e.g. https://nexusq.example; no path, query, fragment or userinfo |
| NEXUSQ_TOKEN | NEXUS-Q bearer token (at least 32 non-whitespace characters) |
| NEXUSQ_KEY_ID | Key identifier used for envelope encryption |
| NEXUSQ_CONTEXT_ID | Stable context identifier cryptographically bound into each encrypted payload |
| NEXUSQ_STORE_DIR | Optional local ciphertext-store directory |

Plain HTTP is accepted only for loopback development endpoints. HTTPS certificate validation remains enabled. Redirects are never followed. HTTP request and response sizes are bounded; response bodies are streamed with a 2 MiB ceiling.

By default, ciphertexts are stored in:

- $XDG_DATA_HOME/nexusq-secretspec, when set; otherwise
- $HOME/.local/share/nexusq-secretspec.

Keep this directory backed up if you need local recovery of stored envelopes. Backups contain ciphertext, not plaintext. Losing an envelope loses the corresponding secret even if its NEXUS-Q key still exists.

## Register the executable

Create the user provider directory and a claim file. Replace the executable path with its absolute path:

    mkdir -p "$HOME/.config/secretspec/providers.d"
    cat > "$HOME/.config/secretspec/providers.d/nexusq.secretspec.json" <<'JSON'
    {
      "executable": "/absolute/path/to/secretspec-provider-nexusq",
      "environment": [
        "NEXUSQ_URL",
        "NEXUSQ_TOKEN",
        "NEXUSQ_KEY_ID",
        "NEXUSQ_CONTEXT_ID",
        "NEXUSQ_STORE_DIR"
      ]
    }
    JSON
    chmod 600 "$HOME/.config/secretspec/providers.d/nexusq.secretspec.json"

SecretSpec validates the registration and executable ownership/permissions before launch. Do not put credentials in the claim file.

Configure the provider using the URI nexusq://default in the SecretSpec project manifest. For example:

    [project]
    name = "payments"
    revision = "1.0"

    [profiles.default]
    DATABASE_URL = { description = "Production database URL", providers = ["nexusq://default"] }

The provider URI contains no credentials; authentication comes from the explicitly allow-listed environment variables.

## Current protocol coverage

- Address resolution (convention and native item addresses)
- Read, presence check, write, expiring write and delete
- Mutation preflight and write-target description

The adapter intentionally does not advertise batch reads, reflection, or clear-all operations. Native coordinate selectors (field, vault, section, version) are not currently supported.

SecretSpec's external provider protocol currently carries values as UTF-8 strings. This adapter therefore does not claim arbitrary binary-secret support.

## Limitations

- The local filesystem provides persistence, not replication or remote synchronization.
- Expiry metadata is stored locally; NEXUS-Q envelope encryption does not enforce the expiry.
- The configured bearer token's NEXUS-Q authorization scope is the actual cryptographic access boundary. The adapter's context and address checks happen after decryption and do not replace server-side authorization.
- Replaying an older envelope for the same address and context is not prevented.
- The adapter does not yet implement a migration, key-rotation or recovery workflow.
- Plaintext exists transiently in process memory and in the IPC/HTTP request buffers; Rust zeroization is applied to the configured bearer token and SecretSpec's IPC secret wrapper, but not every intermediate allocation.
- A dedicated GitHub Actions run has completed a real end-to-end test against NEXUS-Q v4.1.2. The test launches the pinned service and exercises the adapter over the actual SecretSpec provider IPC protocol, including initialization, address resolution, encrypted write/read, presence checks, context isolation, deletion, and missing-record behavior. See [the successful SecretSpec adapter workflow run](https://github.com/lecodev-26/nexusq-keyprovider/actions/runs/41745106439). This validates that pinned service revision and CI environment; it is not a SecretSpec CLI registration test, a full upstream conformance run, or an independent security audit.

See ROADMAP.md for validation milestones, SECURITY.md for the threat model, and docs/DESIGN.md for implementation decisions.
