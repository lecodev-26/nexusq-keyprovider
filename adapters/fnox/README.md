# NEXUS-Q adapter for fnox

**Status: implemented · experimental · not production-approved.**

This directory contains a Lua 5.4 custom provider for [fnox](https://github.com/jdx/fnox). It delegates envelope encryption and decryption to the existing NEXUS-Q service; it does not implement cryptography or modify the NEXUS-Q engine.

The adapter has been tested with the fnox 1.47.0 CLI against a deterministic mock API and in a dedicated GitHub Actions workflow that builds NEXUS-Q v4.1.2 from an immutable source commit and exercises a real-service encrypt/decrypt round trip. The workflow also checks wrong-context rejection, locked-vault failure, and recovery after unlock. The local mock uses reversible Base64 framing, not cryptography. No independent security audit, certification, upstream endorsement, or production approval is claimed.

## Compatibility and runtime requirements

- **Validated with fnox 1.47.0** (embedded Lua 5.4 and the documented hooks and host modules). Other fnox versions have not yet been verified.
- **curl 8.4.0 or newer** available on PATH.
- A reachable NEXUS-Q server exposing /v1/keys, /v1/encrypt, and /v1/decrypt.
- A NEXUS-Q bearer token and an active key identifier.

The curl minimum is security-relevant: --max-filesize must abort unknown-length/chunked responses during transfer. The adapter bypasses the user curlrc file and passes its own curl configuration through child stdin (curl --disable --config -); the token and request body are not put in curl's argv or shell command. It does not enable redirect following.

## Install the plugin

Clone the central repository and link the adapter directory. provider.lua is at the root of this plugin directory, as fnox expects:

~~~sh
git clone https://github.com/lecodev-26/nexusq-keyprovider.git
fnox plugin link nexusq ./nexusq-keyprovider/adapters/fnox
~~~

Configure the provider in your project's fnox.toml:

~~~toml
[providers.nexusq]
type = "lua"
plugin = "nexusq"
~~~

The plugin link is local to the fnox user's plugin directory. For a deployment, pin the central repository to a reviewed commit and manage plugin updates deliberately. fnox Lua plugins run with the user's permissions.

## Configuration

Provide these environment variables to every fnox process that resolves or writes secrets:

| Variable | Required | Meaning |
|---|---:|---|
| NEXUSQ_URL | Yes | NEXUS-Q origin URL, e.g. https://vault.example.net |
| NEXUSQ_TOKEN | Yes | Bearer token; at least 32 non-whitespace bytes |
| NEXUSQ_KEY_ID | Yes | Existing NEXUS-Q key identifier used for encryption |
| NEXUSQ_CONTEXT_ID | Yes | Stable trust-domain identifier, e.g. prod-eu |

NEXUSQ_URL must be an origin only: no path, query, fragment, URL userinfo, or whitespace. Remote endpoints must use HTTPS with normal certificate verification. Plain HTTP is accepted only for validated loopback addresses (localhost, 127.0.0.0/8, or [::1]) for local development. The NEXUS-Q server itself does not terminate TLS; remote deployments need a trusted TLS termination boundary configured according to the NEXUS-Q server documentation.

The context identifier is part of the encrypted payload and is compared byte-for-byte after decryption. Use a distinct value for each trust domain and preserve it for recovery. It does not replace NEXUS-Q authorization or prevent replay/rollback within the same context.

## Use

Check connectivity and that the configured key exists:

~~~sh
fnox provider test nexusq
~~~

Set a secret (stdin avoids placing the value in shell history and process arguments):

~~~sh
printf '%s' "$DATABASE_URL" | fnox set DATABASE_URL --provider nexusq
~~~

Resolve it:

~~~sh
fnox get DATABASE_URL
~~~

When encrypt is defined, fnox stores the ciphertext reference returned by the hook in fnox.toml. Later, get receives that exact reference and asks NEXUS-Q to decrypt it. There is no separate decrypt hook in fnox's Lua provider contract.

## Stored format and API mapping

Stored value:

~~~text
nexusq-fnox:v1:<standard-base64-envelope>
~~~

The NEXUS-Q encrypted plaintext is a JSON object with exactly these fields:

~~~json
{
  "adapter": "nexusq-fnox",
  "version": "1",
  "context_id": "prod-eu",
  "secret": "<secret-value>"
}
~~~

version is deliberately a string. fnox 1.47.0's fnox.json.decode currently represents JSON numeric values through an internal serde-number table in Lua; using a string avoids depending on that implementation detail.

- encrypt(ctx, value) encodes the payload and POSTs {key_id, plaintext, metadata} to /v1/encrypt.
- get(ctx, ref) validates the reference prefix, POSTs {envelope} to /v1/decrypt, Base64-decodes the result, and validates adapter, version, and context before returning the secret.
- test(ctx) GETs /v1/keys and checks that the configured key identifier is present.

The NEXUS-Q metadata label is nexusq-keyprovider-fnox-v1. Context validation does not rely on that metadata because the current decrypt response returns plaintext only, not authenticated envelope metadata.

## Transport and limits

- Fixed argv invocation: curl --disable --config -; config, bearer header, and JSON body are supplied over stdin, not interpolated into a shell command.
- No --location: 3xx responses fail; POST bodies are not replayed to redirect targets.
- HTTPS and curl's default certificate verification for remote endpoints; HTTP restricted to loopback.
- curl connect timeout: 5 seconds; total timeout: 30 seconds.
- Maximum response: 2 MiB, enforced during transfer with curl 8.4.0+ --max-filesize, including chunked responses.
- Maximum request: 1 MiB, matching the NEXUS-Q server's default request-body limit. A newly returned envelope is rejected if its future decrypt request would exceed this limit.
- HTTP 401/403 map to fnox.auth_error; HTTP 423 reports a locked NEXUS-Q vault. Other non-2xx statuses, malformed JSON/Base64, unsupported references, invalid payloads, and context mismatches fail closed.
- The plugin never intentionally logs request bodies, response bodies, plaintext, ciphertext, or bearer tokens.

These are adapter-side fixed limits. If the NEXUS-Q server is configured with a lower request-body limit, its lower limit still applies. Increasing the server limit does not increase the adapter's 1 MiB ceiling.

## Tests

Run the CLI integration suite with fnox 1.47.0 and curl 8.4.0+ installed:

~~~sh
FNOX_BIN=/path/to/fnox python3 adapters/fnox/tests/test_cli.py
~~~

The suite uses a local mock HTTP service and tests set/get, provider test, special-character transport, wrong context, malformed reference, authentication failure, locked vault, POST redirect handling, chunked-response size enforcement, and remote-HTTP rejection. The separate real-service workflow validates the NEXUS-Q encrypt/decrypt round trip and the locked/unlocked vault lifecycle.

**The mock is intentionally non-cryptographic.** It wraps the plaintext with a marker and Base64-encodes it so the tests exercise the fnox lifecycle and adapter validation only. Passing this suite is not evidence of NEXUS-Q cryptographic correctness or a real-service integration test.

## Remaining validation

- **Passed:** [GitHub Actions run #2](https://github.com/lecodev-26/nexusq-keyprovider/actions/runs/41719182840), on branch `ci/fnox-real-service` at commit `f3361d11cf614a29a3779eb741a49f5b1f65abbc`. The workflow builds NEXUS-Q v4.1.2 from pinned commit `8735e5537bc687ee906ed68bdf119fc8956d9439`, runs the mock suite, and verifies a real local-service round trip, wrong-context rejection, locked-vault handling, and recovery after unlock. This is evidence for the tested Linux/Ubuntu workflow only, not a general compatibility or production-security claim.
- Confirm key status/rotation semantics and maximum practical secret size with the real NEXUS-Q envelope implementation.
- Verify plugin behavior and supported curl versions on macOS and Windows environments used by fnox operators.
- Confirm the upstream maintainers' preferred process for listing or documenting third-party Lua providers.
- Obtain an independent security review before any production-use claim.

## Security notes

See DESIGN.md and SECURITY.md. Context binding does not prevent same-context rollback, NEXUS-Q's current bearer-token model is not identity-aware RBAC, and Lua strings/process buffers are not guaranteed to be zeroized. Preserve old NEXUS-Q keys until all dependent fnox configuration has been re-encrypted and verified.
