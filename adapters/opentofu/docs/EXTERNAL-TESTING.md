# External operator testing guide

**Project:** NEXUS-Q Key Provider for OpenTofu  
**Current release:** [v0.1.0-rc.1](https://github.com/lecodev-26/nexusq-keyprovider/releases/tag/v0.1.0-rc.1)  
**Status:** experimental pre-release; external testing requested  
**Last updated:** 2026-11-02

Thanks for helping evaluate this project. This guide is for operators who can test OpenTofu and NEXUS-Q in a disposable development environment. The goal is to find installation, interoperability, recovery, and documentation problems before considering a stable release.

> [!WARNING]
> Do not use this release with production infrastructure or valuable state. Use a disposable directory, test-only resources, a dedicated NEXUS-Q test vault, and credentials created only for this evaluation. This project has not received an independent security audit and is not production-certified.

## 1. What is being tested

This repository provides an external executable implementing OpenTofu External Key Provider protocol v1. It connects OpenTofu state and saved-plan encryption to NEXUS-Q envelope encryption. It does not modify OpenTofu core.

This guide covers only the OpenTofu adapter. The repository is intended to host multiple adapters; fnox is a planned candidate, not an available integration. Other IaC, Kubernetes, and CI/CD integrations remain exploratory until implemented and validated.

Please focus on:

- Installation and executable discovery on your operating system.
- OpenTofu protocol interoperability and configuration clarity.
- Encrypted local state and saved-plan creation, inspection, and apply.
- Recovery of existing state and plans using previously wrapped DEKs.
- Behavior when the configured wrapping-key ID changes, while historical keys remain available.
- Error messages and safe failure behavior for incorrect configuration or unavailable NEXUS-Q.
- Whether the documentation is sufficient for another operator to reproduce the setup.

## 2. Safety rules and test boundaries

1. Use only a new, disposable working directory and a test-only NEXUS-Q vault/key/token.
2. Use a harmless local-only OpenTofu resource such as `terraform_data`; do not configure cloud, network, DNS, Kubernetes, or other real infrastructure providers for this evaluation.
3. Do not point the test at a production backend, production vault, or real infrastructure state.
4. Never share bearer tokens, passwords, DEKs, vault files, state files, saved plans, raw provider metadata, or unredacted logs.
5. Do not delete or deactivate a wrapping key while any test state or saved plan may still depend on it.
6. Do not change `NEXUSQ_CONTEXT_ID` to work around a decryption failure. Stop and record the sanitized error.
7. Do not enable `NEXUSQ_ALLOW_LEGACY_V1=true` unless you are deliberately testing a migration with disposable v1 test data. It defaults to false.
8. Do not expose the test-only mock services or NEXUS-Q test server to an untrusted network.

The context ID is an operator-supplied trust-domain label. It is not authenticated workspace/state identity and does not prevent replay or rollback of an older valid envelope within the same context. See the adapter [security policy and threat model](SECURITY.md) before testing.

## 3. Choose a test path

### Path A — reproducible local integration test (recommended first)

This path runs the repository's automated E2E script using its test-only NEXUS-Q API substitute and local HTTP-backend fixture. It does not require a real NEXUS-Q server or credentials. The script creates temporary test data and removes its temporary directory on exit.

Requirements:

- Linux, macOS, or a compatible Unix-like shell environment.
- Go 1.26 or newer.
- An OpenTofu version listed in the [compatibility matrix](OPENTOFU-COMPATIBILITY.md). The core E2E script is the relevant test; the documented CI baseline includes 1.12.11 and 1.13.2 on Linux AMD64, with 1.13.2 also tested on Linux ARM64.
- Python 3, `curl`, and standard shell utilities used by the script.
- Local TCP ports `18443` and `18444` available.

From a clean clone:

```sh
git clone https://github.com/lecodev-26/nexusq-keyprovider.git
cd nexusq-keyprovider
git checkout v0.1.0-rc.1

go version
tofu version
python3 --version
curl --version

(cd adapters/opentofu && go test -count=1 ./...)
(cd adapters/opentofu && go vet ./...)
bash adapters/opentofu/tests/integration/e2e.sh
```

Expected result: the script ends with a `PASS` line confirming local state, saved plans, prior-key recovery/rotation, and HTTP remote-state recovery. It also checks that selected plaintext markers are absent from the tested encrypted state and saved-plan files.

This fixture is a development test double. It does **not** validate the NEXUS-Q server implementation, production authentication, AWS S3, S3 locking, or any production deployment.

If a required port is already in use, stop the conflicting local service or use an isolated test machine. Do not expose the fixture on a public interface.

### Path B — operator smoke test with a real NEXUS-Q test vault

Run this path only if you already operate a NEXUS-Q test instance and understand its vault/key lifecycle. Use a dedicated disposable vault and a token scoped to the protected API operations required by the provider.

1. Follow [Installation, upgrades, and troubleshooting](INSTALLATION-TROUBLESHOOTING.md) to build or install the provider. The release page contains experimental platform binaries; verify checksums and Sigstore attestations using [Release build and verification](RELEASE-BUILDS.md).
2. Create or select an active AES-256-GCM wrapping key with purpose `wrap` in the test vault. Create a separate test token; do not reuse a production token.
3. Configure `NEXUSQ_URL`, `NEXUSQ_TOKEN`, `NEXUSQ_KEY_ID`, and a new stable random `NEXUSQ_CONTEXT_ID` in the same protected environment that launches OpenTofu. Use HTTPS for remote NEXUS-Q endpoints. HTTP is accepted only for loopback development endpoints.
4. Create a fresh directory containing only a minimal OpenTofu configuration with the encryption block shown in the [README](../README.md#opentofu-example) and a local-only `terraform_data` resource. Use the absolute path to the provider executable.
5. Run `tofu init`, then `tofu plan -out=test.tfplan`. Confirm the plan can be inspected with `tofu show -json test.tfplan`, and apply only this disposable local-only test plan.
6. Confirm the resulting local state can be read with `tofu show -json`. Do not upload or share the state or plan files.
7. In a copy of the disposable test directory, verify that the state remains readable with the same context and credentials. Record whether recovery works after closing and restarting the shell or test service.
8. If testing wrapping-key rotation, create/activate a second test wrapping key, update only `NEXUSQ_KEY_ID`, and perform a plan/apply against the disposable test state. Verify that the state remains readable. Keep the old wrapping key available throughout and after the test until all test artifacts are deleted.
9. If the test fails, stop before applying any unexpected infrastructure change. Preserve only sanitized error text and non-sensitive environment/version details.

Do not use this smoke test to infer compatibility with remote production backends. Real AWS S3 validation has a separate [runbook](AWS-S3-VALIDATION.md) and remains pending until executed in an authorized AWS test account.

## 4. Suggested test matrix

Please report only the combinations you actually tested. A successful build is not runtime compatibility evidence.

| Area | Test | Result to record |
| --- | --- | --- |
| Install | Build or install the exact release/source commit | OS, architecture, Go version, binary/source SHA |
| OpenTofu | Initialize a disposable configuration | Exact OpenTofu version and result |
| State | Create, read, and refresh encrypted local state | Pass/fail and sanitized error |
| Saved plan | Create, inspect, and apply a saved plan | Pass/fail; do not attach the plan |
| Recovery | Reopen the working directory or recover from a clean copy | What was restarted/copied and result |
| Rotation | Change to a second wrapping-key ID while retaining the old key | Pass/fail and recovery result |
| Negative behavior | Try an incorrect context ID or unavailable test endpoint using disposable data | Expected fail-closed behavior and sanitized error |
| Documentation | Follow this guide without undocumented assistance | Missing, unclear, or incorrect steps |

Optional: if you have an isolated disposable AWS account and are authorized to incur its costs, follow the separate AWS S3 runbook. Do not treat S3Mock results as AWS validation.

## 5. What to send back

Open a GitHub issue for a reproducible bug or use [OpenTofu community discussion #4801](https://github.com/orgs/opentofu/discussions/4801) for general interoperability feedback. You can also use the repository's issue tracker for documentation and installation problems.

Include:

- Operating system and version; CPU architecture.
- OpenTofu version and Go version (if building from source).
- Release tag or exact source commit SHA.
- Test path (A or B) and the steps you performed.
- Expected behavior and observed behavior.
- Pass/fail result for each relevant row in the test matrix.
- Sanitized error messages and relevant timestamps, if useful.
- Whether the problem is reproducible from a clean disposable directory.

Never attach or paste credentials, tokens, vault files, state files, saved plans, plaintext DEKs, provider metadata/envelopes, or logs containing sensitive values. Redact hostnames, account IDs, paths, and identifiers if they reveal private infrastructure details.

For suspected security vulnerabilities, follow [SECURITY.md](../../../SECURITY.md) and use the private reporting instructions there. Do not publish exploit details or sensitive evidence in a public issue or discussion.

## 6. Feedback questions

1. Could you build/install the provider without maintainer assistance?
2. Was the OpenTofu encryption configuration clear and consistent with your expectations?
3. Did state and saved-plan encryption, inspection, apply, and recovery work as documented?
4. Were key rotation and historical-key retention understandable and successful?
5. Were failures understandable and safely fail-closed?
6. What important workflow or compatibility case is missing?
7. Would you be willing to test a follow-up release candidate?

## 7. Current validation limits

- The project is experimental and has not received an independent security audit.
- CI's second-runner rebuild is still within GitHub Actions; an outside-operator reproduction is pending.
- Real AWS S3, IAM/bucket policies, versioning, Object Lock, SSE-KMS, replication, and S3 lockfile contention have not been validated.
- Compatibility applies only to the exact OpenTofu/OS/architecture combinations in the [compatibility matrix](OPENTOFU-COMPATIBILITY.md).
- Context binding does not establish workspace/state lineage and does not prevent same-context replay/rollback.
- NEXUS-Q availability, backups, and retention of historical wrapping keys remain operational dependencies.

See the [project roadmap](../ROADMAP.md) and [external review status](EXTERNAL-REVIEW.md) for outstanding work.
