# External review and community feedback

**Project:** NEXUS-Q Key Provider for OpenTofu
**Current status:** experimental integration prototype
**Prepared:** 2026-11-02
**Public upstream outreach:** posted to OpenTofu Discussions; feedback pending
**Independent security audit:** not yet commissioned or performed

This document separates community interoperability feedback from a formal security assessment. Neither is a substitute for the other.

## 1. OpenTofu community review

The project is an external executable implementing OpenTofu External Key Provider protocol v1. It does not change OpenTofu core, add a built-in key provider, or require an OpenTofu patch. The appropriate initial request is a question/discussion seeking protocol and integration feedback, not a request to merge this repository into OpenTofu.

OpenTofu's contribution guide directs general questions to GitHub Discussions or the OpenTofu Slack. It also asks contributors to discuss changes before submitting code. See [OpenTofu contributing guide](https://github.com/opentofu/opentofu/blob/main/CONTRIBUTING.md).

### Existing upstream context

- [OpenTofu External Key Provider documentation](https://opentofu.org/docs/language/state/encryption/) describes the executable protocol: header on stdout, metadata on stdin, and key output on stdout.
- [OpenTofu issue #4206](https://github.com/opentofu/opentofu/issues/4206) is an accepted core issue about optional additional encryption context for built-in cloud key providers. It is related conceptually but is not the same implementation: this project keeps its context assertion inside the NEXUS-Q-encrypted payload and uses the external provider's metadata. It does not implement or replace the core issue.
- Maintainer comments on #4206 note that provider-specific context APIs should preserve the semantics and naming of each remote key-management service. This project does not propose changing those built-in provider APIs.

### Draft community discussion

The discussion was published in the OpenTofu community forum: [Discussion #4801](https://github.com/orgs/opentofu/discussions/4801). The following is the text that was posted.

**Suggested title:** Feedback request: external NEXUS-Q key provider integration for OpenTofu state and plan encryption

> Hi OpenTofu community,
>
> I have built an experimental external key-provider companion that connects OpenTofu state and saved-plan encryption to NEXUS-Q. It runs as a separate executable and implements External Key Provider protocol v1; it does not modify or fork OpenTofu.
>
> The provider generates 256-bit DEKs for OpenTofu's native AES-GCM encryption and uses NEXUS-Q's envelope API to wrap and recover them. Its v2 metadata includes an operator-configured context assertion inside the NEXUS-Q-encrypted payload. Legacy v1 metadata is rejected by default, with an explicit migration opt-in.
>
> **Validation currently documented:** OpenTofu 1.12.11 core E2E on Linux AMD64; OpenTofu 1.13.2 core E2E on Linux AMD64 and ARM64; and OpenTofu 1.13.2 native runtime E2E on macOS 15 ARM64 and Windows Server 2025 AMD64. S3-compatible E2E uses S3Mock, not AWS. CI includes a separately built NEXUS-Q test service, cross-runner rebuild comparisons, checksums, SBOMs, and Sigstore attestations. See the repository and CI evidence for exact scope.
>
> Repository: https://github.com/lecodev-26/nexusq-keyprovider
>
> **I would appreciate feedback on:**
> 1. Whether the process protocol handling and metadata lifecycle align with the intended External Key Provider contract.
> 2. Whether there are interoperability expectations or edge cases not covered by the current E2E matrix.
> 3. Whether the documentation accurately describes the limits of context binding, key rotation, state/saved-plan recovery, and external process trust.
> 4. Whether there is a preferred place to maintain a community integration reference or compatibility report.
>
> This is an experimental integration prototype, not a production-readiness claim. It has not received an independent security audit. Real AWS S3 validation and an independent external build reproduction are still pending.
>
> I am not requesting an OpenTofu core change or implying endorsement. I would value technical feedback before considering any broader release.

Before posting, replace the validation summary with the latest CI run link and confirm the current compatibility matrix. Do not attach state files, plans, credentials, tokens, or real infrastructure data.

## 2. Technical review questions

Reviewers are invited to challenge the implementation and its assumptions, particularly:

- Does the companion correctly implement the protocol header, stdin metadata, stdout response, process exit behavior, and stderr diagnostics?
- Are null initial metadata, subsequent metadata, omitted/empty metadata, and malformed input handled consistently with OpenTofu's contract?
- Are the provider's encryption/decryption key outputs and metadata lifecycle correct across initial state creation, refresh, saved-plan creation, plan apply, and rotation?
- Are there OpenTofu versions or execution modes with different process lifecycle or metadata behavior that should be tested?
- Does the visible `context_id` plus encrypted-payload context assertion have any unintended compatibility or migration consequences?
- Are the documented residual risks clear: no authenticated workspace/state identity, no same-context replay/rollback prevention, and bearer-token scope independent of context ID?
- Are there operational failure cases around NEXUS-Q outage, wrapping-key rotation, backups, and restoration that require additional guidance?

A review comment should identify the OpenTofu version, operating system/architecture, test setup, observed behavior, and whether the finding is a protocol mismatch, implementation bug, documentation issue, or deployment concern. Never include sensitive state or credentials.

## 3. Independent security review brief

A qualified independent reviewer should assess at least the following scope:

1. Go implementation and dependency/build inputs, including static analysis and dependency/supply-chain review.
2. External Key Provider protocol parsing, output discipline, error handling, process lifecycle, and malformed-input behavior.
3. DEK generation, byte handling, envelope API use, metadata validation, context binding, and legacy migration.
4. TLS, redirect policy, bearer-token handling, response limits, timeout behavior, logging, and secret exposure.
5. Threat-model assumptions, replay/rollback risks, host compromise, context identity limitations, and recovery/rotation procedures.
6. OpenTofu state and saved-plan integration, including failure injection and artifact recovery.
7. CI permissions, release attestations, SBOM scope, reproducibility claims, and artifact verification.

**Explicit exclusions unless separately contracted:** a full audit of NEXUS-Q itself, OpenTofu core, cloud provider services, production AWS configuration, and the operator's deployment environment.

### Evidence package for an auditor

- `README.md`, `DESIGN.md`, `SECURITY.md`
- `docs/THREAT-MODEL-REVIEW.md` (internal review only)
- `docs/CONTEXT-BINDING-DESIGN.md`
- `docs/OPENTOFU-COMPATIBILITY.md`
- `docs/RELEASE-BUILDS.md` and `docs/PROVENANCE.md`
- `docs/AWS-S3-VALIDATION.md` (prepared runbook; not executed evidence)
- Source commit under review and associated CI workflow run/artifacts

The reviewer should receive a fixed commit SHA and reproducible evidence, not a moving branch. Findings should be classified by severity, include reproduction steps, and distinguish exploitable vulnerabilities from hardening recommendations. Any public report should be coordinated with the project maintainer and avoid exposing exploit details before remediation.

## 4. Other external validations

These are separate work items and must remain visibly pending until actually completed:

- **Independent operator build:** a person outside this project's GitHub Actions workflow reproduces the build from a fixed commit and publishes the toolchain, environment, and hash comparison.
- **AWS S3:** execute the [AWS S3 validation runbook](AWS-S3-VALIDATION.md) in an authorized disposable AWS account. Record IAM, bucket policy, versioning, TLS, lockfile, and recovery results. S3Mock results are not a substitute.
- **Compatibility maintenance:** re-run the relevant matrix when new OpenTofu releases or supported OS runner images are adopted.

## 5. Status ledger

| Activity | Status | Evidence |
| --- | --- | --- |
| Internal threat-model review | Complete; internal only | `THREAT-MODEL-REVIEW.md` |
| OpenTofu protocol and integration feedback | Public discussion posted; feedback pending | [Discussion #4801](https://github.com/orgs/opentofu/discussions/4801) |
| Independent security audit | Not commissioned / not performed | No external report |
| Independent operator build | Pending | GitHub second-runner comparison is not independent-operator evidence |
| Real AWS S3 validation | Pending | `AWS-S3-VALIDATION.md` is a runbook only |

Do not change these statuses based on CI success alone.
