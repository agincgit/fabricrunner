# Fabric Runner

A provider-neutral runtime for distributing AI workloads across frontier APIs,
self-hosted cloud models, and models on a personal network.

> **Status: pre-alpha.** Core contracts are usable, but the API is not stable.

## Why

Provider SDKs expose model-specific tool loops. They do not provide one durable
control plane capable of moving a workload between Claude, OpenAI, and local
models while preserving policy, state, and provenance.

Fabric Runner owns:

- the provider-neutral model and tool loop;
- step-level routing between managed cloud, self-hosted cloud, and personal
  network trust zones;
- bidirectional delegation between local and frontier models;
- enforced tool, sandbox, budget, and data-egress policy; and
- a replayable event history explaining every placement and side effect.

The project remains useful on one machine. Distributed workers are an additive
deployment mode, not a prerequisite.

## Design

Provider-agnostic by construction. Fabric Runner owns the loop; provider SDKs
are adapters beneath it. The initial adapter targets are Anthropic Messages and
OpenAI-compatible endpoints, including Ollama, vLLM, llama.cpp, and LM Studio.

A worker performs one bounded model turn or tool action. It may propose a child
task, but only the control plane may approve and place that task.

Go module path: `github.com/agincgit/fabricrunner`

The architecture is explained in
[`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md). Normative requirements,
delivery tasks, and acceptance gates live under [`spec/`](spec/README.md),
starting with the
[`Fabric Runner core specification`](spec/0001-fabric-runner-core/spec.md).

## Current building blocks

- durable SQLite event storage and deterministic replay;
- execution and data-egress policy contracts;
- deterministic eligibility and routing;
- provider-neutral model/tool turns with schemas, budgets, cancellation, and
  ordered parallel tools;
- shared provider conformance tests; and
- OpenAI-compatible Chat Completions and Anthropic Messages adapters, both
  exercised through the same provider-neutral loop.

An OpenAI-compatible endpoint is configured explicitly. Plain HTTP is disabled
unless the operator opts in for a trusted endpoint, such as a runtime on a
personal network:

```go
provider, err := openaicompat.New(openaicompat.Config{
    Name:              "home-models",
    BaseURL:           "http://127.0.0.1:11434/v1",
    AllowInsecureHTTP: true,
})
```

Anthropic authentication is also supplied per request. Bearer and `x-api-key`
headers are supported without storing the credential in the provider:

```go
provider, err := anthropic.New(anthropic.Config{
    Name:    "anthropic",
    BaseURL: "https://api.anthropic.com/v1",
    AuthMode: anthropic.AuthAPIKey,
    TokenSource: func(ctx context.Context) (string, error) {
        return loadToken(ctx)
    },
})
```

## Scope

This repository contains a standalone public source-available product, not a thin SDK
wrapper. Applications can extend it through public contracts for providers,
policies, tools, routing, and telemetry.

## License

Fabric Runner is Fair Source under `FSL-1.1-MIT`. Each published version
irrevocably becomes MIT-licensed on its second anniversary. Versions that have
not reached that date are source-available and are not represented as Open
Source.

See [LICENSE](LICENSE), [licensing policy](LICENSING.md), and
[release record](RELEASES.md).
