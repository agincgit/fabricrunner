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
    SpendEstimator:   spendBounds, // application-supplied conservative upper bounds
    ContextCounter:   tokenCounter, // required for declared context windows
}
projection, err := engine.Run(ctx, request)
```

`EngineRequest` supplies workload, session, step and attempt IDs (created with
`NewID`), a goal, source zone, classifications, initial `ModelRequest`, routing
candidates and explicit budgets. Candidates describe the configured provider
models, their zones, capabilities, health, and capacity. The engine evaluates
fresh policy verdicts; caller-supplied candidate verdicts are not trusted.

Token, cost and tool-call budgets of zero permit no usage in those dimensions.
Model-call, total-step (`MaxSteps`) and wall-time budgets must be positive.
The engine reserves supplied input/output token and cost upper bounds durably
before each call. Missing bounds deny the call. Bounds must cover the exact
request, including provider defaults when no output cap is supplied. Reservations
remain conservatively charged; actual usage is reported separately. Compaction
calls consume the same call, token, cost and time budgets.
Child scheduling is not supported, so this engine creates no child steps.

The request classification covers metadata and tool definitions. Unlabeled
data defaults to confidential. Each message block and tool result keeps its
own classification; the highest classification governs the complete outgoing
request. Remote response content requires a return-path egress verdict before
it reaches the loop or durable content log. The adapter has already received
the response bytes at that point. Artifact dereferencing is not supported.

Tool bindings execute in the source zone and receive a separate execution
policy check. The baseline policy requires tool approval. Configure an
`Approver` to return an attributed decision for a scope and content manifest;
raw content is never included. A missing approver cannot grant policy-required
approval. A configured approver also gates the initial step.

`tool.New` supplies read, write, edit and command bindings. Set an explicit root,
output bound and executor. Build `cmd/fabricrunner-tool` as a trusted helper
outside that writable root for write/edit operations. `sandbox.New()` uses
bubblewrap on Linux and the experimental Seatbelt profile on macOS; other
platforms deny. A nil executor denies side effects. Commands accept only an
argument vector. Linux confinement has behavioral test evidence; macOS runtime
acceptance is still pending. The initial macOS profile denies child creation.

`ModelRequest.AutomaticCompaction` explicitly enables compaction at the context
threshold. A capability flag alone does nothing. Compaction records its exact
permitted inputs, excluded message indexes, summary, model, usage and replaced
event range. Requests too large for even the summarization call fail explicitly.

`engine.Replay(ctx, workloadID)` reads the event store alone. It does not contact
models, execute tools, request approval, or resume an interrupted workload.
Submitting the same workload ID again conflicts before effects occur. When a
store fails mid-run, `Run` returns the error and the last readable committed
projection; it may still be running and require reconciliation. Never infer
safe retry from a storage or transport error.

`engine.Resume(ctx, originalRequest)` can continue a running workload from a
committed turn checkpoint. It preserves IDs, budgets, approvals and compacted
history, and retains the original wall-time deadline. Supply fresh tool handlers
with the original definitions. A request digest mismatch is rejected. Newer
unresolved effects invalidate an older checkpoint and return
`ErrUncertainOutcome`; no model or tool call is repeated automatically.

An optional `Observer` receives operational metadata with per-run sequence
numbers. Delivery is asynchronous and may arrive out of order or be dropped
when callbacks saturate the process-wide limit. Observer failures cannot
replace or alter durable audit records. See `observertest.Run` for exporter
conformance checks.

Executable integration coverage lives in `engine_adapter_test.go`: it uses
the OpenAI-compatible HTTP adapter and SQLite, verifies that turn start is
committed before the HTTP call, and replays after the server is shut down.
Run it with `go test -run TestEngineWithOpenAICompatibleAdapter .`.
