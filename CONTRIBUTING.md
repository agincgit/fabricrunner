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

## Contribution license

Fabric Runner is Fair Source under `FSL-1.1-MIT`. Unless AgInc agrees otherwise
in a signed writing, every intentional contribution submitted for inclusion is
licensed by its contributor under the same `FSL-1.1-MIT` terms as the
repository, including the irrevocable future MIT grant. Copyright remains with
the contributor; this is not a copyright assignment.

By submitting a contribution, you represent that you have the right to license
it on those terms. Do not submit third-party work under incompatible terms.

Each commit must include a developer sign-off:

```text
Signed-off-by: Your Name <your.email@example.com>
```

Create it with `git commit -s`. The sign-off certifies that you authored the
contribution or otherwise have the right to submit it under the contribution
terms above, and that the contribution and sign-off will be maintained as part
of the project's public record.
