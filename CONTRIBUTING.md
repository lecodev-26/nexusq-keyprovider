# Contributing

Thanks for your interest in NEXUS-Q Key Provider.

## Before contributing

- Open an issue or discussion to describe substantial changes before starting implementation.
- Keep changes focused and explain their security and compatibility impact.
- Do not include credentials, real keys, state files, plans, or private infrastructure details.
- Do not claim support for an integration until it has dedicated tests and documented validation.

## Development checks

Use a supported Go toolchain (Go 1.23 or newer):

```sh
gofmt -w .
go test ./...
go vet ./...
```

For security-sensitive changes, add tests for failure paths as well as successful behavior. Run the race detector on a supported host when available:

```sh
go test -race ./...
```

## Integration changes

The OpenTofu adapter must preserve the External Key Provider protocol contract: protocol output belongs on stdout, diagnostics belong on stderr, and failures must exit non-zero. Never log secrets, bearer tokens, or plaintext data-encryption keys.

Other integrations should be implemented as separate adapters with their own protocol documentation and compatibility tests. Do not imply that the current OpenTofu adapter is a generic provider for unrelated tools.
