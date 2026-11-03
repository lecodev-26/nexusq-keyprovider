# fnox adapter design

**Status: implemented · experimental.**

This document describes the current adapter implementation and its evidence boundaries. It is not an independent security review or a claim of production readiness.

## Target revisions

- fnox v1.47.0, commit 3ec91ccf83c0dcf482a61c96d6cfc63f7ef52a9f.
- NEXUS-Q server source commit 8735e5537bc687ee906ed68bdf119fc8956d9439 (v4.1.2 source revision inspected for the HTTP API contract).

The fnox Lua provider docs and implementation were compared with previously inspected revision e080553b2c4c788407da91027284603dc895ebd4; those files are unchanged.

## Integration boundary

fnox embeds Lua 5.4. The adapter is a Lua provider plugin; it does not implement cryptography. NEXUS-Q remains the sole encryption/key-management engine.

The plugin exposes:

- encrypt(ctx, value): wrap a secret using NEXUS-Q and return a versioned ciphertext reference for fnox to store in fnox.toml.
- get(ctx, ref): validate and unwrap that reference through NEXUS-Q, then return the plaintext secret.
- test(ctx): verify configuration, curl availability, connectivity, authentication and the configured key. The key-list route requires an unlocked NEXUS-Q vault session.
- No put hook initially; defining encrypt makes the plugin an encryption provider even if put is also present.
- No get_batch initially; NEXUS-Q's inspected HTTP API has no batch decrypt endpoint.

fnox calls encrypt for set, import and reencrypt; it later calls get with the ciphertext string stored as the secret's value. There is no separate decrypt hook.

## Stored-reference and encrypted payload formats

Stored fnox reference:

~~~text
nexusq-fnox:v1:<standard-base64-envelope>
~~~

The prefix is an adapter-level format discriminator. Unknown or malformed prefixes must raise an error; they must not return nil (which fnox interprets as a missing secret).

Plaintext payload encrypted by NEXUS-Q:

~~~json
{
  "adapter": "nexusq-fnox",
  "version": "1",
  "context_id": "<configured-trust-domain>",
  "secret": "<secret-value>"
}
~~~

The payload is JSON-encoded and passed as the NEXUS-Q plaintext string. The version is encoded as a string because fnox 1.47.0’s JSON host module maps numeric values to an internal serde-number table in Lua; the adapter avoids depending on that host implementation detail. On recovery, the plugin must decrypt, Base64-decode the API response, parse JSON, verify the exact adapter identifier and version, compare context_id byte-for-byte with the current configuration, and only then return secret.

The NEXUS-Q envelope metadata label is the fixed, fnox-specific value nexusq-keyprovider-fnox-v1. It is diagnostic/envelope metadata only: the current decrypt API does not return authenticated envelope metadata, so context validation must rely on the assertion inside the encrypted payload.

## NEXUS-Q HTTP contract

The inspected NEXUS-Q server source defines:

### Encrypt

POST /v1/encrypt

~~~json
{
  "key_id": "<key-id>",
  "plaintext": "<UTF-8 string>",
  "metadata": "nexusq-keyprovider-fnox-v1"
}
~~~

metadata is optional at the server boundary. Success response:

~~~json
{ "data": "<standard-base64-encoded-envelope-bytes>" }
~~~

### Decrypt

POST /v1/decrypt

~~~json
{ "envelope": "<standard-base64-envelope>" }
~~~

Success response:

~~~json
{ "data": "<standard-base64-encoded-plaintext-bytes>" }
~~~

Both routes require an Authorization bearer token header. The server reads NEXUSQ_SERVER_TOKEN, which must be at least 32 bytes. The adapter client-side environment variable is NEXUSQ_TOKEN.

The server defaults to a 1 MiB request-body limit (NEXUSQ_SERVER_MAX_BODY_BYTES can override it), a 60-request/minute rate limit, and a 15-minute unlocked-session inactivity timeout. The adapter has a fixed 1 MiB request ceiling and does not batch decrypt requests. It does not terminate TLS itself. Loopback is the safe default; remote deployments require a trusted TLS termination boundary and NEXUSQ_TRUSTED_TLS_TERMINATION=1.

The decrypt response contains plaintext only; it does not return authenticated envelope metadata.

## Configuration

Required process environment:

- NEXUSQ_URL: base URL of the NEXUS-Q server or trusted TLS proxy.
- NEXUSQ_TOKEN: bearer token; never place it in fnox.toml.
- NEXUSQ_KEY_ID: active NEXUS-Q encryption key identifier.
- NEXUSQ_CONTEXT_ID: stable trust-domain identifier. Use a distinct value for each environment/domain and preserve it for recovery.

The context identifier is not a substitute for NEXUS-Q authorization. It is an encrypted payload assertion checked after decryption.

## HTTP transport implementation

Do not use fnox.http for secret-bearing POST requests in the inspected fnox release. Its host implementation uses a redirect-following reqwest client, exposes no redirect policy, and buffers the complete response without a plugin-configurable body-size cap. An HTTP 307/308 can replay the POST body to the redirect destination.

The implemented transport uses the following controls and has been exercised through the actual fnox 1.47.0 CLI:

- Use fnox.cmd.run({"curl", "--disable", "--config", "-" }, { stdin = config }); --disable must be the first curl option so a user .curlrc cannot enable redirects or disable TLS verification.
- Pass the curl config through child stdin. Do not put the bearer token or request body in command arguments or shell text. The curl --version check also uses --disable.
- Do not enable curl's --location; curl does not follow redirects by default. The --disable option is first so user .curlrc files cannot change the transport policy. For loopback HTTP only, explicitly bypass proxy environment settings with noproxy=*.
- Require curl >= 8.4.0 so --max-filesize also aborts unknown-length/chunked responses during transfer.
- Set a 5-second connect timeout, 30-second total timeout, a 2 MiB response-size cap, and a protocol restriction matching the validated URL scheme.
- Validate the HTTP status separately; never log response bodies or curl diagnostics that may contain sensitive details.
- Check the serialized NEXUS-Q request body against the server's configured/default request-body limit before starting curl.

The implementation was tested with actual fnox cmd.run: a 307 response was not followed, a chunked response was capped, and curl-config quoting preserved JSON containing quotes, backslashes, newline escapes and Unicode. The ten-test CLI integration suite also exercises hostile curlrc handling, URL policy, context/version checks, authentication, and locked-vault errors. The dedicated GitHub Actions workflow builds NEXUS-Q from pinned v4.1.2 source and performs a real service round trip. The run for this PR head passed: [GitHub Actions run #7](https://github.com/lecodev-26/nexusq-keyprovider/actions/runs/41729178136), on commit `a87c5f2949a5f85bd719f740b4e78595ad61785e`.

Remote endpoints must use HTTPS with certificate verification. Plain HTTP may be permitted only for a rigorously validated loopback URL used for local development. The plugin rejects remote HTTP, URL userinfo, query/fragment components, and unexpected base paths. The URL parser and protocol restrictions are implemented and covered by negative CLI tests. Loopback HTTP requests bypass proxy environment settings; remote HTTPS may use a configured proxy while retaining TLS verification.

## Error handling

- HTTP 401/403: use fnox.auth_error with a generic authentication message; never include the token.
- HTTP 423: report that the NEXUS-Q vault/session is locked.
- HTTP 400/409/413/429/5xx and transport failures: fail closed with a concise, non-sensitive error.
- Malformed JSON, missing data, invalid Base64, unknown prefix/version, wrong adapter identifier, wrong context, and missing fields: fail closed.
- Return nil only for a genuine not-found condition, not for malformed ciphertext or API errors.
- Never log plaintext, ciphertext, bearer tokens, full request/response bodies, or secret-bearing references.

## Size handling

The adapter must check the actual serialized JSON request size before sending it. It must also check that a newly returned envelope can fit inside a future /v1/decrypt request under the configured request-body ceiling before returning it to fnox; otherwise fnox could persist a ciphertext it cannot later recover.

Use curl >= 8.4.0 with --max-filesize to bound response bytes while streaming. The response cap is 2 MiB. The exact maximum usable secret size still depends on NEXUS-Q envelope overhead and must be verified against the real service.

## Packaging constraint

fnox's installed-plugin format expects provider.lua at the root of the plugin repository. This adapter lives in a monorepo subdirectory, so the central repository cannot be installed directly as a fnox git plugin without a separate wrapper/repository (which is not the project strategy).

Initial usage should therefore point plugin at the local adapters/fnox directory, or use fnox plugin link to link that directory after cloning this repository. Do not create a separate plugin repository.

## Validation status and remaining gates

- Passed with actual fnox 1.47.0 CLI and curl 8.14.1: provider test, set/get round trip, special-character JSON/curl-config transport, wrong-context rejection, malformed-reference rejection, authentication failure, locked-vault handling, redirect refusal even with a hostile user .curlrc, chunked-response cap, and remote-HTTP rejection.
- A deterministic mock NEXUS-Q API is included at tests/test_cli.py. Its envelope is reversible Base64 framing, not cryptography.
- Passed in CI: real NEXUS-Q v4.1.2 service round trip through the actual fnox 1.47.0 CLI, including encrypt/get, wrong-context rejection, locked-vault failure, and recovery after unlock. Evidence: [GitHub Actions run #7](https://github.com/lecodev-26/nexusq-keyprovider/actions/runs/41729178136), commit `a87c5f2949a5f85bd719f740b4e78595ad61785e`.
- Still required: curl compatibility checks on supported Linux/macOS/Windows operator environments; key-rotation and recovery validation across key changes; independent security review. This CI run is integration evidence, not a production-readiness assessment or security audit.
