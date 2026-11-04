# SecretSpec adapter roadmap

**Current status: initial implementation · experimental.**

## Implemented

- [x] External SecretSpec provider endpoint using the published secretspec.provider/1 IPC protocol.
- [x] Convention-address mapping and native item-address support.
- [x] NEXUS-Q envelope encryption and decryption using the existing v4.1.2 HTTP API.
- [x] Local ciphertext-only persistence, hashed filenames, restrictive Unix permissions, atomic replacement and file-size limits.
- [x] Encrypted payload binding to adapter version, provider URI, configured context ID and exact SecretSpec address.
- [x] HTTPS-only remote transport, loopback-only HTTP, no redirects and bounded request/response bodies.
- [x] Unit tests, formatting checks and warning-free Clippy.

## Validation still required

- [ ] Run the dedicated GitHub Actions workflow against the pinned NEXUS-Q v4.1.2 service.
- [ ] Exercise the adapter through the released SecretSpec CLI, including provider discovery/registration and set/get/delete.
- [ ] Run the upstream SecretSpec provider IPC conformance cases applicable to the advertised capabilities.
- [ ] Add tests for expiry, malformed/corrupt records, symlink attacks and concurrent writers.
- [ ] Validate supported Linux, macOS and Windows filesystem behavior; implement platform-specific atomic replacement and ACL handling where required.
- [ ] Design and test backup, restore, key rotation and recovery procedures.
- [ ] Review NEXUS-Q authorization scope and operational token separation.
- [ ] Obtain independent security review before any production-readiness claim.

## Explicitly out of scope for the first iteration

- Batch reads, reflection and bulk clear.
- Arbitrary binary secrets over the current external provider protocol.
- Remote replication or synchronization of the local ciphertext store.
- Changes to the SecretSpec core or NEXUS-Q engine.

The adapter remains independently maintained and experimental. A successful CI run will establish tested compatibility for that specific revision and environment; it will not constitute an audit or upstream endorsement.
