# Security Policy

## Project status

NEXUS-Q Key Provider is experimental software. The current implementation supports the OpenTofu External Key Provider protocol v1 and has not undergone an independent security audit. Do not assume it is suitable for production or regulated workloads.

## Reporting a vulnerability

Please do not report security vulnerabilities in public issues. Use GitHub's private vulnerability reporting for this repository:

https://github.com/lecodev-26/nexusq-keyprovider/security/advisories/new

Do not disclose exploit details publicly. Include:
- A description of the affected component and impact.
- Reproduction steps or a proof of concept, where safe.
- Relevant versions and environment details.

Do not include real credentials, production keys, state files, or other sensitive data in a report.

## Operational guidance

- Require HTTPS for NEXUS-Q endpoints outside loopback.
- Keep NEXUS-Q bearer tokens out of source control, command-line arguments, HCL, and provider metadata.
- Restrict access to the NEXUS-Q server and use a dedicated vault/token where practical.
- Preserve old wrapping keys while encrypted state or plan artifacts still depend on them.
- Review the README's validation boundaries before deployment.
