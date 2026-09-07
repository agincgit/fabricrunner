# Approval gates, budgets, and lifecycle implementation plan

**Specification:** [spec.md](spec.md)

**Status:** Draft

## Design

All three concerns attach to the engine's transition loop rather than to the
turn loop, because all three are workload-scoped: a budget spans steps, an
approval gates a step, and cleanup runs once per workload.

Budget checks precede the spend. The engine computes the projected cost of a
call before making it and refuses when the projection exceeds the remaining
budget. Checking after the fact would record an overspend it had already
incurred, which is a report rather than a control.

Cleanup is registered as the engine acquires each resource, so the termination
path is uniform and does not enumerate resource kinds. Every exit runs the same
release sequence.

## Delivery sequence

1. Add the `Approver` contract and the unconfigured no-pause default.
2. Record approval outcomes and make replay skip re-requesting.
3. Enforce each budget dimension before spend.
4. Implement cancellation propagation with a bounded interval.
5. Implement uniform cleanup across every termination path.
