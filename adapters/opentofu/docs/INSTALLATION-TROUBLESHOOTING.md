# OpenTofu adapter — installation, upgrades, and troubleshooting

## Scope

This guide applies only to the OpenTofu adapter in `adapters/opentofu/`. It is not a generic installation guide for every adapter in this repository.


This guide installs the companion executable locally. It does not install NEXUS-Q or OpenTofu. Experimental prebuilt NEXUS-Q Key Provider binaries are available from the [GitHub Releases page](https://github.com/lecodev-26/nexusq-keyprovider/releases); verify their checksums and attestations before use. You may also build from a reviewed source commit. Native runtime E2E evidence covers OpenTofu 1.13.2 on Linux AMD64/ARM64, macOS 15 ARM64, and Windows Server 2025 AMD64. This does not establish compatibility with other OS releases, architectures, or OpenTofu versions. macOS and Windows commands below describe build/install steps; consult the compatibility matrix for exact runtime coverage.

## Requirements

- OpenTofu with External Key Provider protocol v1 support; see the [tested compatibility matrix](OPENTOFU-COMPATIBILITY.md).
- Go 1.26 or newer to build from source.
- A reachable NEXUS-Q server and an active AES-256-GCM wrapping key with purpose `wrap`.
- A token authorized for the NEXUS-Q protected API.
- A stable, unique `NEXUSQ_CONTEXT_ID` for the state trust domain.

## Build and install on Linux

From a clean checkout at the reviewed commit:

```sh
(cd adapters/opentofu && go test -count=1 ./...)
(cd adapters/opentofu && go vet ./...)
mkdir -p "$HOME/.local/bin"
(cd adapters/opentofu && go build -trimpath -buildvcs=false -o "$HOME/.local/bin/nexusq-keyprovider" .)
chmod 0755 "$HOME/.local/bin/nexusq-keyprovider"
```

Confirm the file exists and is executable:

```sh
test -x "$HOME/.local/bin/nexusq-keyprovider"
```

## Build and install on macOS

Use the same Go build command, with an installation path such as `$HOME/.local/bin/nexusq-keyprovider`:

```sh
mkdir -p "$HOME/.local/bin"
(cd adapters/opentofu && go build -trimpath -buildvcs=false -o "$HOME/.local/bin/nexusq-keyprovider" .)
chmod 0755 "$HOME/.local/bin/nexusq-keyprovider"
```

The binary must be built for the host architecture. This project currently has a macOS ARM64 cross-build check, but does not yet run the provider E2E suite on macOS.

## Build and install on Windows (PowerShell)

From the repository root:

```powershell
$installDir = Join-Path $env:LOCALAPPDATA "NEXUSQ\bin"
New-Item -ItemType Directory -Force -Path $installDir | Out-Null
Push-Location adapters/opentofu
go test -count=1 ./...
go vet ./...
go build -trimpath -buildvcs=false -o (Join-Path $installDir "nexusq-keyprovider.exe") .
Pop-Location
```

Use the full executable path in the OpenTofu configuration. Windows runtime behavior has not yet been E2E tested.

## Configure the environment

The process inherits environment variables from the shell or service that launches OpenTofu. Set them in that same environment:

```sh
export NEXUSQ_URL="https://nexusq.example.internal"
export NEXUSQ_TOKEN="<retrieve-from-your-secret-manager>"
export NEXUSQ_KEY_ID="<active-wrapping-key-id>"
export NEXUSQ_CONTEXT_ID="<stable-random-context-id>"
```

On Windows PowerShell, use process-scoped variables for the current session:

```powershell
$env:NEXUSQ_URL = "https://nexusq.example.internal"
$env:NEXUSQ_TOKEN = "<retrieve-from-your-secret-manager>"
$env:NEXUSQ_KEY_ID = "<active-wrapping-key-id>"
$env:NEXUSQ_CONTEXT_ID = "<stable-random-context-id>"
```

Avoid putting tokens in HCL, source control, command-line arguments, shell history, or CI logs. Do not use persistent `setx` for secrets unless your operating-system secret-handling policy explicitly permits it. Remote NEXUS-Q endpoints require HTTPS; plain HTTP is accepted only for loopback development endpoints.

In the OpenTofu encryption block, use an absolute path to the executable:

```hcl
key_provider "external" "nexusq" {
  command = ["/absolute/path/to/nexusq-keyprovider"]
}
```

On Windows, use a quoted absolute path to `nexusq-keyprovider.exe`, for example `C:/Users/<user>/AppData/Local/NEXUSQ/bin/nexusq-keyprovider.exe`.

## Upgrade procedure

1. Record the source commit and Go toolchain used for the new binary.
2. Back up the current state and any saved plans using your normal protected backup process.
3. Keep the existing executable at a separate rollback path; build the candidate to a new path.
4. Keep `NEXUSQ_CONTEXT_ID`, NEXUS-Q credentials, and the existing wrapping-key IDs unchanged during the binary upgrade.
5. In a controlled working directory with a copy of the state, run `tofu init` and `tofu show -json` to verify that the state can still be read. Use the tested OpenTofu version and do not run concurrent operations against the same state.
6. Review a plan before applying any infrastructure changes. Confirm state and saved-plan handling before promoting the new executable.

Do not delete or deactivate old NEXUS-Q wrapping keys while state or saved-plan artifacts may still depend on them.

## Rollback considerations

Rollback means restoring a previously tested executable and a compatible state backup. It is not enough to copy back an older binary: a build that predates context-bound v2 metadata may not understand state metadata written by the current provider. Never change the context ID to make a failed decrypt succeed. If a rollback build cannot read the current metadata format, restore the matching state backup and use a provider build known to support that format. Keep old wrapping keys available throughout recovery.

Legacy v1 migration is a separate, deliberate procedure. Do not leave `NEXUSQ_ALLOW_LEGACY_V1=true` enabled as a routine setting; see the README migration notes.

## Troubleshooting

| Symptom / error | Likely cause and safe checks |
| --- | --- |
| `NEXUSQ_TOKEN and NEXUSQ_KEY_ID are required` | One or both variables are missing from the environment that launches OpenTofu. Check the process environment without printing the token. |
| `NEXUSQ_CONTEXT_ID is required` or invalid characters | Set a stable 1–128 character ASCII ID using only letters, digits, dot, underscore, colon, or hyphen. Do not add leading/trailing whitespace. |
| `invalid NEXUSQ_URL` | Check that the URL has a valid scheme and host and contains no userinfo, query, or fragment. |
| `HTTP is allowed only on loopback` | Use HTTPS for remote endpoints. HTTP is permitted only for localhost/loopback development. |
| `NEXUS-Q decrypt failed: HTTP 401/403` | Check token validity, authorization scope, vault availability, and whether the token is authorized for the requested operation. Never paste the token into logs or support reports. |
| `NEXUS-Q decrypt failed: HTTP 404` | Check that the configured envelope/key material still exists and that the expected NEXUS-Q service/API is being used. Keep historical wrapping keys available. |
| `metadata context_id does not match NEXUSQ_CONTEXT_ID` | The configured context differs from the context recorded with this state. Restore the correct configuration; do not generate a new ID to bypass the check. |
| `decrypted payload context_id does not match NEXUSQ_CONTEXT_ID` | The decrypted payload is bound to a different context. Stop and investigate the state, metadata, and NEXUS-Q response. Do not continue with apply. |
| `legacy metadata v1 is disabled` | The artifact uses legacy v1 metadata. Follow the documented, backed-up migration procedure and enable the opt-in only for that deliberate migration. |
| Provider executable not found / permission denied | Use an absolute path, check file existence and executable permissions, and ensure the binary matches the host OS/architecture. |
| Provider protocol / JSON parsing error | Ensure the configured command runs the provider executable directly. Do not wrap it in a script that writes banners, debug text, or other output to stdout. Diagnostic output belongs on stderr. |

The provider has a 15-second HTTP timeout and rejects redirects. It fails closed when configuration, metadata, API responses, or recovered key material are invalid. Preserve the exact error text (after removing secrets and sensitive identifiers) when reporting an issue.

## Evidence and support boundaries

The core E2E compatibility matrix covers OpenTofu 1.12.11 on Linux AMD64 and 1.13.2 on Linux AMD64/ARM64. Native provider runtime E2E also covers OpenTofu 1.13.2 on macOS 15 ARM64 and Windows Server 2025 AMD64. The S3Mock and real NEXUS-Q E2E suites remain on Linux with 1.13.2. These tests do not establish real AWS S3 compatibility or support for other OS releases/architectures. See [ROADMAP.md](../ROADMAP.md), [OpenTofu compatibility](OPENTOFU-COMPATIBILITY.md), and the [AWS S3 validation runbook](AWS-S3-VALIDATION.md).
