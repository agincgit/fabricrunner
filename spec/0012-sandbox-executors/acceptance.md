# Sandbox executors acceptance

**Specification:** [spec.md](spec.md)

**Status:** In Progress

Each criterion below is the name of a test to be written before the
implementation exists.

The implementation is accepted when:

- `TestDefaultExecutorDenies` — an unconfigured executor denies side-effecting
  execution;
- `TestSandboxEstablishmentFailureDenies` — a failure to establish confinement
  denies execution rather than proceeding unconfined;
- `TestUnsupportedPlatformDeniesWithReason` — a platform with no executor
  denies and names the platform;
- `TestConfinedWriteOutsideRootFails` — a confined process attempting a write
  outside its root fails, asserted by attempting it;
- `TestConfinedNetworkCallFails` — a confined process attempting a network
  connection fails, asserted by attempting it;
- `TestConfinedProcessEscapeFails` — a confined process cannot escape its
  process group, asserted by attempting it;
- `TestPolicyCanEvaluateSandboxCapability` — an `ExecutionPolicy` receives
  sandbox capability on `ExecutionTarget` and can deny on it;
- `TestSandboxLifecycleRecorded` — establishment, denial, and teardown appear
  in the event stream and observer records; and
- build, vet, static analysis, race tests, and leak scanning pass on both
  darwin and linux.

## Evidence

See the dated evidence and outstanding native macOS gate below.

## Local acceptance evidence — 2026-09-08 UTC

| Requirement | Observable evidence | Result |
|---|---|---|
| FR-BOX-001, FR-BOX-002 | Default and failed-establishment denial tests | Pass |
| FR-BOX-003 | `TestPolicyCanEvaluateSandboxCapability` | Pass |
| FR-BOX-005, FR-BOX-007 (Linux) | Actual outside-write, network and session-escape attempts | Pass under WSL Ubuntu |
| FR-BOX-006 | Unsupported-platform denial names Windows | Pass |
| FR-BOX-008 | `TestSandboxLifecycleRecorded` checks durable records and observer outcomes | Pass |
| FR-BOX-004, FR-BOX-007 (macOS) | Darwin/arm64 cross-build | Build passes; native behavioral acceptance NOT RUN |

0012 remains In Progress. A macOS host is required for native confinement,
process escape, cleanup, race and static-analysis evidence. The initial Seatbelt
profile denies child creation, unlike Linux's PID-namespace-backed executor.
The no-inbound-port Phase 2 work remains sequenced after the Phase 1 gate.
