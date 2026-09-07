# Fabric Runner

A provider-neutral runtime for distributing AI workloads across frontier APIs,
self-hosted cloud models, and models on a personal network.

> **Status: pre-alpha.** The repository is being scaffolded. There is no usable
> API yet, and nothing here is stable.

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

## Scope

This repository contains a standalone open-source product, not a thin SDK
wrapper. Applications can extend it through public contracts for providers,
policies, tools, routing, and telemetry.

## License

Apache 2.0 — see [LICENSE](LICENSE).
