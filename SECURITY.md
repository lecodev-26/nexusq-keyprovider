# Security Policy

## Scope

This repository contains independent adapters that connect external tools to the existing NEXUS-Q cryptographic engine. Security properties, supported versions, threat models, and validation status are adapter-specific. Consult the security and threat-model documentation inside the adapter you intend to use.

- [OpenTofu adapter security policy and threat model](adapters/opentofu/docs/SECURITY.md)
- [OpenTofu internal threat-model review](adapters/opentofu/docs/THREAT-MODEL-REVIEW.md)
- [fnox adapter threat model](adapters/fnox/docs/SECURITY.md)
- [SecretSpec adapter threat model](adapters/secretspec/docs/SECURITY.md)

No adapter should be assumed production-ready, independently audited, certified, or suitable for regulated workloads unless that specific evidence is published for that adapter. CI success is not a security audit.

## Reporting a vulnerability

Please do not report security vulnerabilities in public issues. Use GitHub's private vulnerability reporting for this repository:

https://github.com/lecodev-26/nexusq-keyprovider/security/advisories/new

Do not disclose exploit details publicly. Include:

- The affected adapter and component.
- A description of the impact.
- Reproduction steps or a proof of concept, where safe.
- Relevant versions, commit, and environment details.

Do not include real credentials, production keys, state files, saved plans, vault files, or other sensitive data in a report.

## Repository-wide security principles

- Keep the NEXUS-Q cryptographic engine separate; adapters consume its documented APIs and must not duplicate or silently alter its security behavior.
- Each adapter must document its own assets, trust boundaries, assumptions, mitigations, residual risks, and recovery considerations.
- Never commit secrets, tokens, private keys, real state files, saved plans, or production fixtures.
- Fail closed on malformed input, authentication errors, unsupported metadata, context mismatches, and cryptographic/API failures.
- Do not log plaintext secrets, key material, bearer tokens, or sensitive request/response bodies.
- Require independent review before making production-readiness or certification claims.
