# Provider adapter conformance implementation plan

**Specification:** [spec.md](spec.md)

**Status:** Implemented

## Design

Strengthen the root provider contracts instead of introducing a second adapter
interface. Add validation and deep-clone methods to model descriptors,
capabilities, events, and deltas. Centralize incremental stream protocol state
in `ModelStreamValidator`, then use it from both `DrainStream` and the model/tool
loop.

Add `DiscoverModels` as the sole validated catalog boundary. Keep catalog order
stable while rejecting duplicates and provider mismatches.

Create `providertest` as a reusable external-test package. An adapter supplies
fresh credential-free fixtures backed by a scripted local transport. The suite
checks the common contract; provider-specific translation remains in each
adapter package.

## Delivery sequence

1. Define descriptor, capability, delta, event, and stream-state validation.
2. Add isolated catalog discovery and stream cloning.
3. Replace duplicate stream checks in the turn loop with the shared validator.
4. Implement the reusable provider fixture suite.
5. Add adversarial unit tests for protocol, cancellation, cleanup, and
   mutation boundaries.
6. Run local and published acceptance gates and record their evidence.
