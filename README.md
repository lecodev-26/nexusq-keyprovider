# NEXUS-Q Key Provider

[![CI](https://github.com/lecodev-26/nexusq-keyprovider/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/lecodev-26/nexusq-keyprovider/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.23%2B-00ADD8?logo=go)](https://go.dev/)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache--2.0-blue.svg)](LICENSE)
[![Status: Experimental](https://img.shields.io/badge/status-experimental-orange.svg)](#status)

**Bring NEXUS-Q key management to OpenTofu state and saved-plan encryption.**

NEXUS-Q Key Provider is a small Go companion that connects [OpenTofu](https://opentofu.org/) to [NEXUS-Q](https://github.com/lecodev-26/nexus-q). It implements the **OpenTofu External Key Provider protocol v1** and uses NEXUS-Q to wrap and recover the data-encryption keys (DEKs) OpenTofu uses for AES-GCM encryption.

It runs as a separate executable. **It does not modify, fork, or patch OpenTofu.**

> [!WARNING]
> This project is experimental and has not received an independent security audit. It is a tested integration prototype, not a production-readiness or compliance claim. Review the limitations and security notes before using it with valuable infrastructure state.

## Why this project?

OpenTofu can encrypt state and saved plans with externally supplied keys. This project connects that capability to NEXUS-Q:

- OpenTofu continues to encrypt state and saved plans using its native AES-GCM method.
- The companion generates fresh 256-bit DEKs and asks NEXUS-Q to wrap them.
- When OpenTofu needs an earlier key, the companion asks NEXUS-Q to unwrap the prior DEK.
- NEXUS-Q retains responsibility for the wrapping key and its lifecycle; OpenTofu retains responsibility for data encryption.

This keeps key wrapping separate from the infrastructure tool and avoids embedding NEXUS-Q credentials in OpenTofu's encryption metadata.

## Project status and scope

- **Implemented:** OpenTofu External Key Provider protocol v1.
- **Implemented:** NEXUS-Q envelope API integration for wrapping and recovering 32-byte DEKs.
- **Implemented:** initial key exchange, prior-key recovery, and key rotation behavior.
- **Not implemented:** adapters for Terraform, other IaC tools, cloud KMS services, or a generic provider framework.
- **Not claimed:** production readiness, independent security audit, FIPS validation, or post-quantum certification.

The project is intentionally named for the integration layer it may grow into. OpenTofu is its first adapter; each future integration will need its own protocol implementation and compatibility tests.

## Validated behavior

A local end-to-end test was run with OpenTofu 1.13.2 on Linux ARM64 and NEXUS-Q 4.1.2. The exercised scenarios included:

- OpenTofu initialization and apply with state encryption enabled.
- State refresh and decryption through `tofu show -json`.
- Saved-plan encryption, inspection, and successful apply.
- Wrapping-key rotation while retaining the ability to read state encrypted with the previous DEK.
- Plaintext scans confirming the tested state and plan files did not contain the test resource name or test value.
- Four concurrent local plan processes in independent workspace copies.
- OpenTofu's HTTP backend protocol against a loopback-only development fixture, including recovery from a clean workspace and after restarting the fixture.

The automated Go suite covers URL policy, authenticated HTTP calls, API response validation and size limits, redirect rejection, protocol output, initial key exchange, and recovery of the previous DEK during rotation. Local statement coverage was **77%** at the time of this README update.

These results describe specific local tests. They do not establish compatibility with production remote backend services, all OpenTofu versions, sustained workloads, or distributed CI environments.

## Architecture

```text
                  OpenTofu
          state / saved-plan encryption
                       |
              External Key Provider v1
                       |
                       v
              NEXUS-Q Key Provider
                (this project)
                       |
              HTTPS + bearer token
                       |
                       v
                    NEXUS-Q
            envelope API + key vault
                       |
             wrapping key (AES-256-GCM)
```

**Key responsibilities**

| Component | Responsibility |
| --- | --- |
| OpenTofu | State/plan serialization and AES-GCM data encryption |
| NEXUS-Q Key Provider | External process protocol, DEK generation, API calls |
| NEXUS-Q | Envelope wrapping/unwrapping and wrapping-key lifecycle |

See [DESIGN.md](DESIGN.md) for protocol details, metadata format, trust boundaries, and failure behavior.

## Requirements

- OpenTofu with External Key Provider protocol v1 support.
- A reachable NEXUS-Q server.
- An active NEXUS-Q AES-256-GCM key with purpose `wrap`.
- An unlocked NEXUS-Q vault while provider operations run.
- Go 1.23 or newer to build from source.

## Build and test

Build on the host that will run OpenTofu:

```sh
git clone https://github.com/lecodev-26/nexusq-keyprovider.git
cd nexusq-keyprovider

go test ./...
go vet ./...
go build -trimpath -o nexusq-keyprovider .
```

Cross-compile a Linux ARM64 executable:

```sh
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 \
  go build -trimpath -ldflags="-s -w" \
  -o nexusq-keyprovider-linux-arm64 .
```

Prebuilt release artifacts are not currently published. Build from source and review the code before use.

## Configure NEXUS-Q

1. Initialize or unlock the NEXUS-Q vault using your normal operational procedure.
2. Create or select an **active AES-256-GCM key** whose purpose is `wrap`.
3. Create a bearer token authorized for the NEXUS-Q protected API.
4. Make the NEXUS-Q API reachable from the machine executing OpenTofu.

Use a dedicated vault and narrowly scoped token for this integration where practical. The companion does not activate NEXUS-Q keys automatically.

## Configuration

The executable reads these environment variables:

| Variable | Required | Description |
| --- | --- | --- |
| `NEXUSQ_URL` | Yes | NEXUS-Q base URL. |
| `NEXUSQ_TOKEN` | Yes | Bearer token for the protected API. |
| `NEXUSQ_KEY_ID` | Yes | Active NEXUS-Q wrapping-key ID. |

Example for a local development server:

```sh
export NEXUSQ_URL="http://127.0.0.1:8443"
export NEXUSQ_TOKEN="<load-from-your-secret-manager>"
export NEXUSQ_KEY_ID="<active-wrapping-key-id>"
```

**Transport policy:** plain HTTP is accepted only for localhost/loopback addresses. Use HTTPS for remote NEXUS-Q endpoints. The provider does not follow HTTP redirects.

Do not commit credentials, place tokens in HCL, or pass secrets as command-line arguments. Supply them through a protected environment or secret manager. Be aware that environment-variable exposure depends on the host's process and operating-system security model.

## OpenTofu example

Add an encryption block to your OpenTofu configuration and point the external provider command to the executable's absolute path:

```hcl
terraform {
  encryption {
    key_provider "external" "nexusq" {
      command = ["/absolute/path/to/nexusq-keyprovider"]
    }

    method "aes_gcm" "nexusq" {
      keys = key_provider.external.nexusq
    }

    state {
      method   = method.aes_gcm.nexusq
      enforced = true
    }

    plan {
      method   = method.aes_gcm.nexusq
      enforced = true
    }
  }
}
```

This example enables encryption for both state and saved plans. Consult the [OpenTofu state-encryption documentation](https://opentofu.org/docs/language/state/encryption/) for version-specific configuration details and operational guidance.

## Key lifecycle and rotation

On an initial invocation:

1. OpenTofu starts the executable and sends null provider metadata.
2. The companion generates a fresh random 32-byte DEK.
3. It sends the Base64-encoded DEK to NEXUS-Q `POST /v1/encrypt`.
4. NEXUS-Q returns an envelope.
5. The companion returns the DEK to OpenTofu and stores the envelope in provider metadata.

On a subsequent invocation, the companion uses the prior envelope with NEXUS-Q `POST /v1/decrypt`, recovers the previous 32-byte DEK, generates and wraps a fresh DEK, and returns both keys in the provider response.

The metadata contains a format version, the NEXUS-Q envelope, and the wrapping-key ID. It does **not** contain the bearer token or plaintext DEK.

**Rotation warning:** keep old NEXUS-Q wrapping keys available until every state or saved-plan artifact that depends on them has been migrated or retired. Removing an old wrapping key too early can make historical state or plans unreadable.

## Security considerations and limitations

- The provider fails closed when configuration, metadata, API responses, or recovered key lengths are invalid.
- NEXUS-Q responses are limited to 1 MiB; HTTP requests have a 15-second timeout.
- The companion uses Go's standard library only; there are no third-party Go module dependencies in the current implementation.
- Go byte slices are not guaranteed to be securely zeroized by this prototype.
- The bearer token authenticates the companion to NEXUS-Q; it is not per-user identity or mutual TLS.
- Protect the NEXUS-Q server with appropriate network controls. Do not expose it directly to untrusted networks.
- Keep old wrapping keys available for as long as encrypted artifacts depend on them.
- No independent security audit, FIPS validation, or post-quantum certification is claimed.

Read [SECURITY.md](SECURITY.md) before reporting a vulnerability or planning deployment.

## Integration fixture

`integration/remote-http.tf` and `integration/fixtures/http-backend.py` provide a local OpenTofu HTTP-backend test fixture. It binds to loopback and is intended only for development/testing. It has no TLS, authentication, or production-grade durability and must not be exposed to a network or used to store real infrastructure state.

## CI

GitHub Actions runs:

- Formatting checks, unit tests, and `go vet` on Go 1.23 and the current stable Go release.
- Go race detection on the stable Go runner.
- Cross-builds for Linux AMD64/ARM64, macOS ARM64, and Windows AMD64.

See the [CI workflow](.github/workflows/ci.yml) for the exact matrix. Cross-build success is not the same as runtime testing on every target platform.

## Contributing

Bug reports, protocol feedback, documentation improvements, and focused tests are welcome. Please read [CONTRIBUTING.md](CONTRIBUTING.md) before opening a pull request. Keep changes scoped, include reproducible tests, and never include credentials, real state files, or production secrets.

## Project documentation

- [Design and protocol flow](DESIGN.md)
- [Security policy](SECURITY.md)
- [Source and dependency provenance](docs/PROVENANCE.md)
- [Contribution guide](CONTRIBUTING.md)
- [Changelog](CHANGELOG.md)

## License

Licensed under the [Apache License, Version 2.0](LICENSE).
