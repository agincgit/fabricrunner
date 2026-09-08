# Running a workload

The root `Engine` runs a bounded workload through execution policy, egress
policy, deterministic placement, the provider-neutral loop, and durable storage.
It returns a `WorkloadProjection` loaded from the committed event stream. The
last `Execution` record contains the normalized `LoopResult`, including messages,
usage, failures, and any exhausted budget dimension.

Configure the engine once before use:

```go
engine := &fabricrunner.Engine{
    Store:            store, // e.g. sqlite.Open(ctx, "workloads.db")
    Loop:             loop.Engine{},
    ExecutionPolicy:  baseline.Policy{},
    DataEgressPolicy: baseline.Policy{},
    Router:           deterministic.Router{},
    Providers:        map[string]fabricrunner.Provider{provider.Name(): provider},
}
projection, err := engine.Run(ctx, request)
```

`EngineRequest` supplies workload, session, step and attempt IDs (created with
`NewID`), a goal, source zone, classifications, initial `ModelRequest`, routing
candidates and explicit budgets. Candidates describe the configured provider
models, their zones, capabilities, health, and capacity. The engine evaluates
fresh policy verdicts; caller-supplied candidate verdicts are not trusted.

Token, cost and tool-call budgets of zero permit no usage in those dimensions.
Model-call and wall-time budgets must be positive. The engine inherits the
loop's usage accounting; advance spend reservations are part of spec 0013.
Child scheduling is not supported, so this engine creates no child steps.

The request classification covers metadata and tool definitions. Unlabeled
data defaults to confidential. Each message block and tool result keeps its
own classification; the highest classification governs the complete outgoing
request. Remote response content requires a return-path egress verdict before
it reaches the loop or durable content log. The adapter has already received
the response bytes at that point. Artifact dereferencing is not supported.

Tool bindings execute in the source zone and receive a separate execution
policy check. The baseline policy requires tool approval, which currently
fails closed. Applications may supply their own policy and confined tool
handlers. Built-in tools, sandbox executors, approval handling and compaction
remain specs 0011–0014.

`engine.Replay(ctx, workloadID)` reads the event store alone. It does not contact
models, execute tools, request approval, or resume an interrupted workload.
Submitting the same workload ID again conflicts before effects occur. When a
store fails mid-run, `Run` returns the error and the last readable committed
projection; it may still be running and require reconciliation. Never infer
safe retry from a storage or transport error.

An optional `Observer` receives operational metadata with per-run sequence
numbers. Delivery is asynchronous and may arrive out of order or be dropped
when callbacks saturate the process-wide limit. Observer failures cannot
replace or alter durable audit records. See `observertest.Run` for exporter
conformance checks.

Executable integration coverage lives in `engine_adapter_test.go`: it uses
the OpenAI-compatible HTTP adapter and SQLite, verifies that turn start is
committed before the HTTP call, and replays after the server is shut down.
Run it with `go test -run TestEngineWithOpenAICompatibleAdapter .`.
