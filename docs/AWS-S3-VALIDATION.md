# AWS S3 validation runbook

## Purpose and status

This runbook describes how to repeat the OpenTofu + NEXUS-Q Key Provider S3 backend checks against a real AWS S3 bucket when an authorized, disposable AWS test environment is available.

**Status: prepared, not executed.** The repository currently has CI coverage against Adobe S3Mock, not AWS S3. No AWS compatibility result should be claimed until this procedure (or an equivalent reviewed test) has been run and its evidence recorded.

## Safety requirements

- Use a dedicated non-production AWS account or isolated test environment.
- Use a newly created, uniquely named bucket with no valuable data.
- Set a budget alert and a short test window. S3 storage, requests, logging, KMS, and data transfer may incur charges.
- Use temporary credentials from AWS IAM Identity Center/SSO or an assumed role. Do not put access keys in HCL, command-line arguments, repository files, CI logs, or commits.
- Grant only the permissions required for the test bucket and, if enabled, its lockfile object. Do not use account-wide administrator credentials.
- Keep NEXUS-Q reachable only through the intended protected endpoint. Use a disposable NEXUS-Q context/key and test-only state.
- Record the OpenTofu version, AWS region, test date, policy revision, and results. Redact account IDs, bucket names if sensitive, tokens, and all secrets from public evidence.

## Prerequisites

- Linux or macOS shell with Go, OpenTofu 1.13.2, AWS CLI v2, Python 3, and `curl`.
- A built `nexusq-keyprovider` executable for the test platform.
- A working NEXUS-Q test service, test bearer token, wrapping-key ID, and dedicated context ID.
- An AWS role/session authorized for the dedicated test bucket.
- A bucket in the selected region. Enable bucket versioning for the version-recovery check.

Confirm the active identity before proceeding:

```sh
aws sts get-caller-identity
aws configure get region
tofu version
```

Do not continue if the identity or account is not the intended test environment.

## Configure the S3 backend

Start from `integration/remote-s3.tf`, but create a temporary copy outside the repository and make these changes in its `backend "s3"` block:

1. Replace `bucket`, `key`, and `region` with the dedicated test bucket, a unique test object key, and its actual region.
2. Remove the S3Mock `endpoints` override entirely.
3. Remove S3Mock-only settings: `skip_credentials_validation`, `skip_region_validation`, `skip_metadata_api_check`, `skip_requesting_account_id`, `skip_s3_checksum`, and `use_path_style`.
4. Set `use_lockfile = true` to exercise the S3 backend's native lockfile mechanism. Confirm this option is supported by the selected OpenTofu version before running.
5. Keep the encryption configuration and the external key-provider command, pointing it to the locally built executable.

Use the standard AWS credential chain (for example, an SSO login or assumed role). Do not place credentials in the backend block. Set the NEXUS-Q variables in the shell environment, without echoing them:

```sh
export NEXUSQ_URL='https://your-test-nexusq-endpoint'
export NEXUSQ_TOKEN='(supply through your local secret manager or secure shell session)'
export NEXUSQ_KEY_ID='aws-s3-e2e-key'
export NEXUSQ_CONTEXT_ID='aws-s3-e2e-context'
aws sso login --profile YOUR_TEST_PROFILE
export AWS_PROFILE=YOUR_TEST_PROFILE
export AWS_REGION=YOUR_BUCKET_REGION
```

The values above are placeholders. Do not commit the temporary configuration or environment values.

## Test sequence

Run from the temporary working directory containing the adjusted configuration:

```sh
tofu fmt -check
tofu init -reconfigure -input=false
tofu plan -input=false -out=aws-s3-test.tfplan
tofu show -json aws-s3-test.tfplan > aws-s3-test-plan.json
tofu apply -input=false -auto-approve aws-s3-test.tfplan
tofu show -json > aws-s3-test-state.json
```

Verify that the expected test resource appears in the JSON output. Inspect the remote object only through an authorized method and verify that the raw stored state object does not contain the chosen plaintext marker or NEXUS-Q bearer token. Do not publish state, plans, JSON output, credentials, or raw object contents.

Then perform the following checks, recording each as PASS, FAIL, or NOT RUN:

1. **Persistence/recovery:** use a fresh temporary working directory with the same backend key and encryption configuration; run `tofu init -reconfigure` and `tofu show -json`. Confirm state is readable and the test resource is recovered.
2. **Versioning:** confirm the bucket has versioning enabled and that a state update creates the expected object versions. Test recovery only on disposable data and document the exact recovery procedure; do not delete or overwrite valuable objects.
3. **Locking:** run two independent OpenTofu operations against the same backend key. Confirm the second operation cannot concurrently acquire the lock and that normal completion releases it. Also test safe handling of an interrupted operation and document any stale-lock recovery procedure.
4. **IAM/bucket policy:** repeat with the intended least-privilege role and verify required operations succeed while unauthorized bucket/object actions are denied. Record the policy used, with identifiers and sensitive details redacted.
5. **Failure behavior:** test expired/invalid AWS credentials and temporary NEXUS-Q unavailability using disposable state. Confirm operations fail closed and do not expose plaintext state or credentials.
6. **Optional controls:** if in scope, separately validate SSE-KMS permissions, Object Lock, lifecycle, replication, CloudTrail data events, and TLS/network policy. These are not implied by the basic backend test.

## Evidence to retain

- OpenTofu version, provider commit, platform, AWS region, test date, and test scenario.
- Redacted IAM role/policy and bucket configuration.
- Command transcript with secrets and identifiers redacted.
- PASS/FAIL/NOT RUN result for every check.
- Relevant CloudTrail or service-side evidence where configured.
- Cleanup confirmation.

Never attach state files, saved plans, NEXUS-Q tokens, AWS credentials, vault backups, or unredacted account/bucket identifiers to a public issue or pull request.

## Cleanup

After collecting evidence, destroy only the disposable test resource and remove the temporary working directory and local plan/state outputs. Delete the dedicated bucket and any test-only KMS keys, policies, roles, log groups, and other resources only after verifying that they contain no required evidence and are not shared. Confirm the bucket is empty before deletion. Follow the organization's retention policy for audit logs.

## Interpretation

A successful run validates only the tested AWS account configuration, region, OpenTofu version, IAM policy, bucket settings, and scenarios. It is not a blanket guarantee for all AWS configurations or production workloads. S3Mock CI results and this unexecuted runbook must not be described as real-AWS test results.
