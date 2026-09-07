// Package fabricrunner contains the canonical domain model for a distributed,
// provider-neutral AI workload fabric.
//
// Fabric Runner owns the model and tool loop, workload state, placement,
// policy, and replayable event history. Provider SDKs are transport adapters
// beneath that loop; no provider SDK owns a top-level session.
//
// Workloads may move between managed frontier APIs, self-hosted cloud models,
// and models on a personal network. Every cross-zone handoff is explicit and
// attributable.
//
// # Status
//
// Pre-alpha. There is no stable API yet, and no exported identifiers.
//
// Normative requirements and acceptance criteria are maintained under spec/.
package fabricrunner
