# Contributing

Fabric Runner is pre-alpha. Discuss large API or protocol changes in an issue
before implementation.

## Development

```sh
go test ./... -race
go vet ./...
gofmt -w .
```

Behavior changes must begin in [`spec/`](spec/README.md) and include observable
acceptance criteria. Never commit credentials, personal or customer data, or
deployment infrastructure identifiers.

New state transitions require tests. New network or provider integrations must
document cancellation, retry, timeout, and data-egress behavior.
