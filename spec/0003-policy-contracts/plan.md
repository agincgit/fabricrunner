# Policy contracts implementation plan

**Specification:** [spec.md](spec.md)

**Status:** In Progress

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
