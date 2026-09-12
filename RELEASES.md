# Release record

Fabric Runner's source was first made publicly available on 2026-09-07.
v0.1.0 is the initial pre-alpha module release, with the single-node execution
engine, durable storage/replay, policy and routing, provider adapters, and
optional telemetry. Its API remains unstable.

Under `FSL-1.1-MIT`, every pushed commit is independently eligible for the MIT
license on the second anniversary of its publication. Git and GitHub preserve
the publication evidence for those commit versions.

Publication dates below use UTC. Each immutable annotated release tag identifies
the exact source commit; resolve it with `git rev-parse 'v0.1.0^{commit}'`.

| Version | Commit | Published | MIT effective |
|---|---|---|---|
| [v0.1.0](https://github.com/agincgit/fabricrunner/releases/tag/v0.1.0) | [`579ded9`](https://github.com/agincgit/fabricrunner/commit/579ded98dd18e85610b606b0d3406c144413e313) | 2026-09-08 | 2028-09-08 |
| [v0.2.0](https://github.com/agincgit/fabricrunner/releases/tag/v0.2.0) | [`e432bfa`](https://github.com/agincgit/fabricrunner/commit/e432bfab57568297d437aa5a5f1b7838dc54ac97) | 2026-09-12 | 2028-09-12 |

The MIT effective date is exactly two years after publication; it is not reset
by a later release.

## v0.1.0 scope

The annotated tag is published and resolves to
`579ded98dd18e85610b606b0d3406c144413e313`. The GitHub pre-release was published
at 2026-09-08T03:58:56Z. The
[release preparation CI](https://github.com/agincgit/fabricrunner/actions/runs/34185189210)
passed formatting, build, vet, race tests, vulnerability scanning and secret
scanning before tagging.

- Specs 0002–0010 supply the implemented building blocks. Policy and routing
  decisions are committed before provider calls; replay reads durable history.
- `ModelCapabilities.AutomaticCompaction` and `ModelError.Retryable` are
  informational metadata. Neither enables runner compaction or automatic retry.
- Built-in tools, sandbox executors, approvals, compaction and restart
  continuation remain unfinished Phase 1 work.
- The outbound personal-network model worker is specified in 0016 and is not
  included. This tag does not provide a cloud-to-home network path or complete
  the Phase 1 or Phase 2 acceptance gates.

Consumers can pin `github.com/agincgit/fabricrunner@v0.1.0`. The annotated tag
and GitHub release record the publication and MIT effective dates and the
resolved commit. Never move a published release tag to different source.

## v0.2.0 scope

The annotated tag is published and resolves to
`e432bfab57568297d437aa5a5f1b7838dc54ac97`. The GitHub pre-release was
published at 2026-09-12T17:04:15Z. The
[release preparation CI](https://github.com/agincgit/fabricrunner/actions/runs/34707106600)
passed formatting, build, vet, race tests, vulnerability scanning and secret
scanning on `ubuntu-24.04` before tagging.

Added since v0.1.0:

- Built-in rooted tools: read, write, edit, and command, with an argument
  vector rather than a shell string.
- Sandbox executors and a denying default, so absence of confinement refuses
  execution rather than bypassing it.
- Approval gates, budget admission checked before spend, cancellation, and
  cleanup on every termination path.
- Context accounting, auditable compaction, and restart continuation.

Known limitation, stated plainly:

- **Linux confinement has behavioral test evidence. The macOS Seatbelt profile
  is experimental and its confinement tests do not currently pass natively.**
  CI runs on Linux only and does not exercise the Seatbelt path. Do not rely on
  macOS confinement in this release. Spec 0012 remains in progress.
- The outbound personal-network model worker (0016) is still not included, and
  neither the Phase 1 nor Phase 2 acceptance gate is complete.
- The v1.0 license review remains a separate future prerequisite.

Consumers can pin `github.com/agincgit/fabricrunner@v0.2.0`. The API remains
unstable. Never move a published release tag to different source.
