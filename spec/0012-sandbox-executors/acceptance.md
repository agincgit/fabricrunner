# Sandbox executors acceptance

**Specification:** [spec.md](spec.md)

**Status:** Draft

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

Pending implementation.
