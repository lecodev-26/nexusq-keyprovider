# fnox adapter security and threat model

**Status: implemented · experimental.**

This threat model describes the current fnox Lua adapter and its tested controls. It is an internal engineering document, not an independent security audit, certification, or production approval.

## Assets

- Plaintext secrets entering fnox set/import and returned by get.
- NEXUS-Q encrypted envelopes stored in fnox.toml.
- NEXUS-Q bearer token (NEXUSQ_TOKEN).
- NEXUS-Q key identifier and configured context identifier.
- Request/response bodies, subprocess stdin/stdout/stderr, fnox logs and shell history.
- Integrity and availability of the NEXUS-Q service, trusted TLS termination and plugin source.

## Trust boundaries

1. fnox CLI and its embedded Lua 5.4 runtime.
2. Lua plugin and the fnox.cmd child-process interface.
3. curl executable and its HTTP/TLS implementation.
4. NEXUS-Q HTTP server and its configured bearer-token boundary.
5. TLS termination proxy for remote NEXUS-Q deployments.
6. fnox.toml and the operator-controlled environment/configuration that supplies context and credentials.

fnox warns that Lua plugins run with the user's permissions. Only use trusted plugin code and pin the central repository to a reviewed commit when deploying it.

## Threats and controls

### Redirect-based disclosure of plaintext

**Threat:** A 307/308 response can replay a secret-bearing POST body to another URL.

**Evidence:** The actual fnox 1.47.0 fnox.http implementation followed a local 307 and the test server received the same POST body at the redirect target.

**Implemented control:** Use curl through fnox.cmd.run with config passed over stdin; do not enable redirect following. Invoke curl with --disable as its first option to ignore any user .curlrc that might enable redirects. An actual fnox CLI test confirmed curl did not send the body to the redirect target, even with a hostile .curlrc enabling location and insecure. Do not use fnox.http for encrypt/decrypt requests unless upstream adds an enforceable no-redirect option and it is verified.

### Token or plaintext leakage through process arguments

**Threat:** Secrets placed in shell commands or process arguments may be visible in process listings, logs or shell history.

**Implemented control:** Invoke curl with a fixed argv table (curl --disable --config -) and pass the token and serialized request body only through the child stdin pipe. Never interpolate them into a shell command or command-line flag. Never print curl config or diagnostics containing sensitive values.

### Oversized or unbounded HTTP responses

**Threat:** An untrusted or compromised endpoint may return an arbitrarily large response and exhaust plugin memory.

**Implemented control:** Require curl >=8.4.0 and use --max-filesize, which aborts unknown-length/chunked transfers at the configured 2 MiB threshold. The actual fnox CLI test suite confirmed a chunked response was stopped at the configured 2 MiB cap. Also validate response size and JSON shape after transfer. The fnox.http host API cannot enforce this limit.

### Plaintext transport or endpoint substitution

**Threat:** A remote plaintext HTTP connection exposes bearer credentials and secret material to network observers or active attackers.

**Implemented control:** Require HTTPS with normal certificate verification for all remote endpoints. Permit HTTP only for explicitly validated loopback addresses used for local development; bypass proxy environment settings for these loopback requests. Reject URL userinfo, fragments, queries and unexpected paths. The NEXUS-Q server itself does not terminate TLS; remote use must be behind a trusted TLS termination boundary.

### Cross-context ciphertext reuse

**Threat:** An envelope copied from one environment may be accepted in another if context is not bound cryptographically.

**Implemented control:** Include adapter, version, context_id and secret inside the plaintext payload encrypted by NEXUS-Q. After decrypt, validate all fields and require an exact context match before returning the secret. Do not trust an editable external context field or envelope metadata alone; the current decrypt API does not return authenticated envelope metadata.

### Ciphertext tampering and malformed references

**Threat:** Attackers may modify the stored reference, change its version/prefix, truncate Base64, or supply malformed payloads.

**Implemented control:** Use a strict versioned prefix, strict Base64 and JSON validation, exact schema checks, and NEXUS-Q authenticated envelope decryption. Fail closed. Never turn malformed or wrong-context data into nil/not-found.

### Credential compromise and excessive authority

**Threat:** The current NEXUS-Q HTTP server uses one configured bearer token, not per-caller identity-aware RBAC. A stolen token may be able to decrypt any envelope allowed by the server's current authorization boundary.

**Controls/limitations:** Protect the token with the host's secret manager/environment controls; use a dedicated token and key/vault per trust domain where operationally possible; rotate/revoke through NEXUS-Q controls. The adapter's context check happens after decrypt and does not limit what the token can ask NEXUS-Q to decrypt.

### Replay and rollback

**Threat:** An older valid envelope from the same context can be replayed or restored.

**Limitation:** Context binding does not prevent rollback/replay within the same trust domain. Protect fnox.toml history and distribution with repository access controls, review, and change monitoring. NEXUS-Q envelope encryption alone does not provide application-level freshness.

### Key rotation and recovery

**Threat:** Removing an old wrapping key may make previously stored fnox ciphertext unrecoverable.

**Control:** Preserve historical NEXUS-Q keys until all dependent fnox configs have been re-encrypted and verified. Test fnox reencrypt and recovery before documenting a rotation procedure.

### Logging and memory exposure

**Threat:** Plaintext may appear in fnox debug logs, curl stderr, process output, shell history or process memory.

**Controls:** Do not log request/response bodies or secret values. Keep error messages generic. Use fnox's prompt/stdin or --from-file rather than passing real secrets as positional CLI arguments; fnox documents that positional values can appear in shell history and process listings. Lua strings and child-process buffers are not guaranteed to be zeroized by this adapter.

## Known residual risks and limitations

- The curl transport adds a runtime dependency on curl >=8.4.0; cross-platform availability/version checks remain required.
- The real NEXUS-Q service round trip has now passed in GitHub Actions through this fnox adapter; see the validation evidence below. This single Linux CI environment does not establish cross-platform compatibility or production readiness.
- The NEXUS-Q server defaults to a 1 MiB request-body cap, 60 requests/minute and a 15-minute unlocked-session inactivity timeout. Large values and many concurrent secret resolutions require testing.
- The server's current bearer-token model is not full identity-aware RBAC and does not issue short-lived tokens.
- Decrypt does not return authenticated envelope metadata; context is validated only from the encrypted plaintext payload.
- Context binding does not prevent replay/rollback within the same configured context.
- fnox plugins run with the user's permissions; a compromised plugin source can access that user's resources.
- No independent review, audit, certification or production approval is claimed.

## Validation status

The following have been tested with the actual fnox 1.47.0 CLI running from its official aarch64 Linux release asset inside Debian 13 proot on Termux, using curl 8.14.1. The release archive was not independently signature-verified:

- fnox set called the plugin's encrypt hook, persisted the returned ciphertext reference, and fnox get passed that reference to get and recovered the test fixture.
- fnox.http followed a local 307 and replayed the POST body to the redirect target.
- fnox.cmd.run with curl config on stdin did not follow the 307; the redirect target received no body.
- An initial curl smoke test capped a chunked response at 1 KiB; the repository CLI suite separately verifies the configured 2 MiB cap.
- curl config quoting preserved JSON containing quotes, backslashes, newline escapes and Unicode.

The repository integration suite passes ten tests, including context/version rejection, malformed references, auth failure, locked vault, URL-policy enforcement, hostile curlrc redirect refusal, and chunked response limits. These tests used local fixtures and a mock HTTP server. A separate GitHub Actions workflow builds pinned NEXUS-Q v4.1.2 source and tests a real service round trip. The run for this PR head passed: [GitHub Actions run #5](https://github.com/lecodev-26/nexusq-keyprovider/actions/runs/41725683858), on commit `af925f45005923434bd62da67c11f6ed3422eb5c`. It exercised actual fnox 1.47.0 encrypt/get, wrong-context rejection, locked-vault failure, and recovery after unlock against the real service. The adapter remains experimental pending cross-platform curl checks, key-rotation/recovery validation across key changes, and independent security review; this integration run is not a security audit or production approval.
