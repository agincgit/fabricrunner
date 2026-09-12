# Local Phase 0–2 implementation run

Base: `06fb121`. Branch: `codex/phase-0-through-2`.

The original publication rule kept development commits local until Phase 2
acceptance. On 2026-09-08 that rule was superseded: publish the current work and
merge to main. Native macOS verification is performed separately after the
merge and remains pending; publication does not complete the acceptance gate.
Do not move the published `v0.1.0` tag.

Phase 0 acceptance remains recorded. The v1.0 license review is still a separate
future release prerequisite; implementation cannot substitute for that review.

Local implementation now covers 0011 tools, Linux 0012 confinement, 0013
approvals/reservations/lifecycle, 0014 compaction, and 0010 checkpoint recovery.
Read the dated acceptance tables for the test evidence and limitations.

The Phase 1 gate remains open:

- Run the native macOS Seatbelt behavioral, cleanup, race and static checks.
  Cross-compilation is not native acceptance. Network/process probes now run on
  both supported operating systems; they are not silently skipped on macOS.
- Exercise the personal/managed model flow using the deployment's actual models.
  The synthetic HTTP delegation and separate-process SQLite recovery tests pass.

Phase 2 implementation has not started because the Phase 1-first dependency
still applies. A cloud coordinator host and personal-model endpoint are needed
for the eventual real no-inbound-port/no-VPN acceptance scenario. The required
macOS/deployment access details have been requested from the user.

Useful local checks:

```sh
GOTOOLCHAIN=go1.25.13 go test ./... -race -count=1
GOTOOLCHAIN=go1.25.13 go vet ./...
GOTOOLCHAIN=go1.25.13 go run golang.org/x/vuln/cmd/govulncheck@latest ./...
```

Linux tests require bubblewrap and Python 3. The helper and confinement probe
are compiled by integration tests into temporary directories. Staticcheck
v0.8.1 itself requires Go 1.26 or later; the verified tool runtime is 1.26.8.
The application/toolchain release gate remains the module's Go 1.25.13.
