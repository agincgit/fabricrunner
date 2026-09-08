# Policy contracts implementation plan

**Specification:** [spec.md](spec.md)

**Status:** Implemented

## Design

Place canonical contracts in the root module so routers, workers, tools, and
applications can depend on them without importing an implementation package.
Keep manifests value-oriented and clone slice data at API boundaries.

Provide a small `policy/baseline` implementation that encodes only universal
safe defaults. It is deterministic and performs no I/O.

## Delivery sequence

1. Define manifest items, effective classification, and validation.
2. Define execution and egress requests and target validation.
3. Define verdict actions, validation, and policy interfaces.
4. Implement the fail-closed deterministic baseline policy.
5. Add table-driven contract, cancellation, mutation-isolation, and baseline
   behavior tests.
6. Run build, vet, static analysis, race tests, vulnerability scanning, and CI.

## 0012 supersession

The original omission of sandbox capability from `ExecutionTarget` is superseded
by 0012. `ExecutionTarget.Sandbox` carries observed establishment and backend;
policy scope equality includes both values. It never authorizes an unconfined
fallback. The engine denies declared side effects without establishment.
