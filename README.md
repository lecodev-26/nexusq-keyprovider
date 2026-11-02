# NEXUS-Q Key Provider

[![CI](https://github.com/lecodev-26/nexusq-keyprovider/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/lecodev-26/nexusq-keyprovider/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go)](https://go.dev/)
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
- **Implemented:** v2 wrapped-DEK payloads bound to an operator-configured context ID, with v1 metadata rejected by default and an explicit migration opt-in.
- **Not implemented:** adapters for Terraform, other IaC tools, cloud KMS services, or a generic provider framework.
- **Backend validation boundary:** S3-compatible integration is tested against S3Mock in CI; real AWS S3, AWS IAM/bucket policies, versioning, and S3 lockfile contention have not been validated. See the [roadmap](ROADMAP.md) and [AWS S3 validation runbook](docs/AWS-S3-VALIDATION.md).
- **Not claimed:** production readiness, independent security audit, FIPS validation, or post-quantum certification.

The project is intentionally named for the integration layer it may grow into. OpenTofu is its first adapter; each future integration will need its own protocol implementation and compatibility tests.

## Validated behavior

The core provider E2E suite is now tested with OpenTofu 1.12.11 on Linux AMD64 and 1.13.2 on Linux AMD64/ARM64. The S3Mock and real NEXUS-Q service E2E suites remain pinned to 1.13.2. The local and CI integration suites cover the following scenarios:

- OpenTofu initialization and apply with state encryption enabled.
- State refresh and decryption through `tofu show -json`.
- Saved-plan encryption, inspection, and successful apply.
- Wrapping-key rotation while retaining the ability to read state encrypted with the previous DEK.
- Context mismatch rejection, tampered external context rejection using the encrypted payload assertion, and explicit legacy v1 migration in the real NEXUS-Q CI E2E suite.
- Plaintext scans confirming the tested state and plan files did not contain the test resource name or test value.
- Four concurrent local plan processes in independent workspace copies.
- OpenTofu's HTTP backend protocol against a loopback-only development fixture, including recovery from a clean workspace and after restarting the fixture.
- CI S3 backend coverage using an isolated Adobe S3Mock instance, including encrypted state and saved-plan handling, plaintext-marker scans, recovery from a clean working directory after an S3Mock restart, and wrapping-key ID rotation.

The S3Mock test exercises the S3 API subset implemented by that local fixture. It is not AWS S3 validation and does not test IAM, bucket policies, versioning, SSE-KMS, replication, or S3 locking. Real-AWS validation is currently pending because no dedicated AWS test environment is available; the [AWS S3 validation runbook](docs/AWS-S3-VALIDATION.md) documents a reproducible procedure, but that procedure has not been executed.

The automated Go suite covers URL policy, authenticated HTTP calls, API response validation and size limits, redirect rejection, protocol output, initial key exchange, and recovery of the previous DEK during rotation. One CI E2E job runs OpenTofu 1.13.2 against the companion on Linux AMD64 and ARM64 with a test-only NEXUS-Q API substitute. A separate Linux AMD64 CI job builds the NEXUS-Q v4.1.2 CLI and server from a pinned source commit, creates an ephemeral vault and wrapping keys, and exercises the real authenticated API with state encryption, saved plans, previous-key recovery, key-ID rotation, server restart recovery, HTTP-backend persistence, audit-chain verification, wrong-context rejection, tampered external-context rejection, and explicit v1 migration. The real-service job uses only generated CI credentials and a temporary vault; it does not depend on repository secrets. Local statement coverage for the provider package is **80.6%** with the current Go test suite.

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
- Go 1.26 or newer to build from source.

## Build and test

Build on the host that will run OpenTofu:

```sh
git clone https://github.com/lecodev-26/nexusq-keyprovider.git
cd nexusq-keyprovider

go test -count=1 ./...
go vet ./...
go build -trimpath -buildvcs=false -o nexusq-keyprovider .
```

For a pinned cross-platform release-candidate build, use Go 1.27.2 and the checked-in script:

```sh
bash scripts/release-build.sh
(cd dist && sha256sum --check SHA256SUMS)
```

The script builds Linux AMD64/ARM64, macOS ARM64, and Windows AMD64 twice, compares each pair byte-for-byte, and writes SHA-256 checksums plus build metadata. See [Release build and verification](docs/RELEASE-BUILDS.md) for independent verification and limitations. A downloadable experimental pre-release is available from the [GitHub Releases page](https://github.com/lecodev-26/nexusq-keyprovider/releases). Verify its SHA-256 checksums and Sigstore attestations before use; see [Release build and verification](docs/RELEASE-BUILDS.md). This pre-release is not independently security-audited or production-certified.

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
| `NEXUSQ_CONTEXT_ID` | Yes | Stable opaque identifier for this state trust domain (1–128 ASCII letters, digits, `.`, `_`, `:`, or `-`); use a distinct random value for each state that must be isolated. |
| `NEXUSQ_ALLOW_LEGACY_V1` | No | Set to `true` only during an intentional v1-to-v2 metadata migration. Defaults to `false`. |

Example for a local development server:

```sh
export NEXUSQ_URL="http://127.0.0.1:8443"
export NEXUSQ_TOKEN="<load-from-your-secret-manager>"
export NEXUSQ_KEY_ID="<active-wrapping-key-id>"
export NEXUSQ_CONTEXT_ID="<stable-random-id-for-this-state>"
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
3. It builds a versioned payload containing the DEK and configured context ID, then Base64-encodes that payload.
4. It sends the encoded payload to NEXUS-Q `POST /v1/encrypt`, which returns an envelope.
5. The companion returns the plaintext DEK to OpenTofu and stores the envelope plus visible context ID in provider metadata.

On a subsequent invocation, the companion validates the metadata schema and visible context ID, decrypts the prior envelope through NEXUS-Q, then checks the context ID embedded inside the decrypted payload before returning any key material. Only after those checks succeed does it generate and wrap a fresh DEK, returning the fresh encryption key and previous decryption key to OpenTofu.

The visible metadata context check is an early rejection; the context assertion inside the NEXUS-Q-encrypted payload is the cryptographic binding. The companion does not rely on NEXUS-Q decrypt returning authenticated envelope-header metadata.

The v2 metadata contains a format version, the configured context ID, the NEXUS-Q envelope, and the wrapping-key ID. The DEK and context assertion are inside the encrypted NEXUS-Q payload; metadata does **not** contain the bearer token or plaintext DEK. Legacy v1 metadata is rejected by default. For a deliberate migration, set `NEXUSQ_ALLOW_LEGACY_V1=true` with the correct `NEXUSQ_CONTEXT_ID`, refresh and verify all dependent state and saved-plan artifacts, then unset the flag. v1 has no context binding, so the first migration is an operator-trusted transition and cannot retrospectively prove the legacy envelope origin.

**Rotation warning:** keep old NEXUS-Q wrapping keys available until every state or saved-plan artifact that depends on them has been migrated or retired. Removing an old wrapping key too early can make historical state or plans unreadable.

## Security considerations and limitations

- The provider fails closed when configuration, metadata, API responses, or recovered key lengths are invalid.
- NEXUS-Q responses are limited to 1 MiB; HTTP requests have a 15-second timeout.
- The companion uses Go's standard library only; there are no third-party Go module dependencies in the current implementation.
- Mutable byte buffers containing key material and request payloads are cleared when practical. This is best-effort hygiene; Go does not guarantee secure zeroization, and values may be copied or retained.
- The configured context ID is not an identity authenticated by OpenTofu and does not prove a workspace, backend object key, or state lineage. It does not replace NEXUS-Q authorization or backend access controls.
- Replaying or rolling back an older valid envelope within the same configured context is not prevented.
- The bearer token authenticates the companion to NEXUS-Q; it is not per-user identity or mutual TLS.
- Protect the NEXUS-Q server with appropriate network controls. Do not expose it directly to untrusted networks.
- Keep old wrapping keys available for as long as encrypted artifacts depend on them.
- No independent security audit, FIPS validation, or post-quantum certification is claimed.

Read [SECURITY.md](SECURITY.md) before reporting a vulnerability or planning deployment.

For upstream interoperability feedback and the independent-review scope, see [External review and community feedback](docs/EXTERNAL-REVIEW.md). The community post is prepared but not yet published; no independent audit is claimed.

## Integration fixtures

`integration/remote-http.tf` and `integration/fixtures/http-backend.py` provide a local OpenTofu HTTP-backend test fixture. `integration/remote-s3.tf` and `integration/s3mock-e2e.sh` exercise the OpenTofu S3 backend against an ephemeral Adobe S3Mock service. `integration/mock-nexusq/` provides a test-only API substitute that wraps test DEKs with AES-256-GCM and a fixed fixture key. These fixtures are intended only for automated development/testing. They do not replace the NEXUS-Q server or validate its implementation, and must not be exposed to a network or used with real infrastructure state or secrets.

The S3Mock test uses generated local credentials and a loopback endpoint. It deliberately does not enable S3 locking. It does not establish compatibility with AWS S3 or other vendors, nor validate production IAM, bucket policies, versioning, Object Lock, SSE-KMS, replication, lifecycle rules, TLS configuration, or lock contention. Real AWS checks are pending; see the [AWS S3 validation runbook](docs/AWS-S3-VALIDATION.md) for the prepared test procedure and [ROADMAP.md](ROADMAP.md) for phase status. For production backend controls, see [SECURITY.md](SECURITY.md) and [Threat Model Review Notes](docs/THREAT-MODEL-REVIEW.md).

Run the core automated suite on Linux with Go 1.27+, either OpenTofu 1.12.11 or 1.13.2, Python 3, and `curl`:

```sh
bash integration/e2e.sh
```

The script builds the provider and mock API locally, creates temporary state/plan/backend files, and removes its temporary files and fixture processes on exit. It uses loopback ports 18443 and 18444; ensure those ports are available.

Run the S3 backend test against S3Mock separately with Docker, Go 1.27+, OpenTofu 1.13.2, and `curl`:

```sh
bash integration/s3mock-e2e.sh
```

It pulls the pinned Adobe S3Mock 5.2.3 image, starts a temporary local service with a named volume, configures test-only credentials and a bucket, then removes the container and volume on exit. Ensure Docker is running and loopback port 9090 is available.

`integration/nexusq-real-e2e.sh` runs the same integration boundary against real NEXUS-Q binaries. Build the `nexusq` and `nexusq-server` release binaries from the NEXUS-Q v4.1.2 source first, then run:

```sh
NEXUSQ_SOURCE_DIR=/path/to/nexus-q bash integration/nexusq-real-e2e.sh
```

The real-service test requires Rust/Cargo, Go, OpenTofu 1.13.2, Python 3, `curl`, and `openssl`. It creates an ephemeral vault, random password and server token, and temporary audit/state files. It binds only to loopback ports 18445 and 18444. The script removes the temporary vault and stops its server/backend processes on exit. It is a CI/development integration test, not a deployment recipe.

## CI

GitHub Actions runs:

- Formatting checks, unit tests, and `go vet` on Go 1.26 and Go 1.27.
- Dedicated Go 1.27 fuzzing for metadata parsing, NEXUS-Q URL validation, and API response parsing.
- Go race detection on Go 1.27.
- Cross-builds for Linux AMD64/ARM64, macOS ARM64, and Windows AMD64. Pinned Go 1.27.2 release builds use separate empty Go caches for repeated builds; a second GitHub-hosted runner rebuilds and compares all four outputs byte-for-byte. This is cross-runner CI evidence, not independent third-party verification.
- Three SPDX 2.3 SBOMs cover the source workspace, release binaries, and direct CI workflow actions, declared integration inputs, and release-build tools. The CI tool inventory is scoped and does not enumerate every preinstalled runner package or transitive action dependency.
- Native OpenTofu 1.13.2 runtime E2E on macOS 15 ARM64 and Windows Server 2025 AMD64, in addition to the Linux compatibility matrix.
- OpenTofu core E2E tests on Linux AMD64 with 1.12.11 and on Linux AMD64/ARM64 with 1.13.2, using an isolated test-only NEXUS-Q API fixture and the local HTTP backend fixture.
- An OpenTofu 1.13.2 S3-compatible backend E2E job against a pinned Adobe S3Mock test service, including encrypted state/plan storage, plaintext-marker scans, service-restart recovery, and wrapping-key rotation. This fixture does not validate AWS production controls or S3 locking.
- A real NEXUS-Q E2E job on Linux AMD64. It builds NEXUS-Q v4.1.2 from a pinned commit, provisions a temporary vault and wrapping keys, and tests authenticated API operations, encrypted local state/saved plans, key rotation, server restart recovery, HTTP-backend recovery, and audit verification. No long-lived repository secrets are required.

See the [OpenTofu compatibility matrix](docs/OPENTOFU-COMPATIBILITY.md) and [CI workflow](.github/workflows/ci.yml) for exact tested versions and coverage. The S3Mock and real-service suites currently remain on 1.13.2. Cross-build success is not the same as runtime testing on every target platform. See [Release build and verification](docs/RELEASE-BUILDS.md) for the artifact verification boundary.

## Contributing

Bug reports, protocol feedback, documentation improvements, and focused tests are welcome. Please read [CONTRIBUTING.md](CONTRIBUTING.md) before opening a pull request. Keep changes scoped, include reproducible tests, and never include credentials, real state files, or production secrets.

## Project documentation

- [Project roadmap and validation status](ROADMAP.md)
- [OpenTofu compatibility matrix](docs/OPENTOFU-COMPATIBILITY.md)
- [Installation, upgrades, and troubleshooting](docs/INSTALLATION-TROUBLESHOOTING.md)
- [AWS S3 validation runbook (not yet executed)](docs/AWS-S3-VALIDATION.md)
- [Design and protocol flow](DESIGN.md)
- [Security policy](SECURITY.md)
- [Threat model review notes](docs/THREAT-MODEL-REVIEW.md)
- [Source and dependency provenance](docs/PROVENANCE.md)
- [Contribution guide](CONTRIBUTING.md)
- [Changelog](CHANGELOG.md)

## License

Licensed under the [Apache License, Version 2.0](LICENSE).
