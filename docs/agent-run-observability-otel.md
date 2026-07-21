# Agent Run Observability (OpenTelemetry)

**Status:** P2  
**Scope:** Brand-neutral, OSS

## Problem

Agent runs execute as headless, isolated jobs. When a run fails, succeeds, or hangs, the reason is often opaque without deep-diving into execution logs. Furthermore, the individual steps an agent takes—such as harness interactions, gateway calls (`llm.request`), or sub-agent invocations—lack a unified trace context. This makes it impossible to query distributed tracing systems to understand an entire agent run from end to end.

## Goal

Provide a unified, single-trace view of an entire agent run by establishing a "trace root" at the orchestrator (`weave start`) level, and threading that context down to all child processes (agents, gateways, harnesses).

## Design

1. **`weave.run` Span (Root Span)**
   Every `weave start` execution creates a root `weave.run` span. This span encompasses the entire lifecycle of the agent run for a specific issue.
   
2. **Context Propagation**
   The trace context (Traceparent/Tracestate) of the `weave.run` span is injected into the child environment (`weaveChildEnv`, the `weave start` launch environment path) using standard W3C trace context format (`TRACEPARENT` env var).

3. **Child Spans**
   Any traceparent-aware agent, harness, or gateway will extract the trace context from the environment. Harness spans (`agent.turn`) and gateway spans (`llm.request`) will automatically nest under the root `weave.run` span, creating a single unified trace tree.

4. **Outcome Attributes**
   The `weave.run` span is enriched with terminal evidence at the end of the run. It captures:
   - `agent` and `nick`: The agent owner identifier.
   - `band`: The capability band of the agent.
   - `tool:model`: The canonical tool and model string.
   - `issue`: The specific issue ID being worked on.
   - `outcome`: The terminal queue state recorded for the run.
   - `converged`: A boolean indicating if the run resulted in a `submitted`, `merged`, or `verified` state.
   - `gate_result`: The exit code of the verify/gate step.
   - `turns`: The total number of conversational turns the agent took.
   - `duration`: Total execution time of the run.

5. **Metrics**
   A histogram metric `fleet.run.turns` is emitted upon completion, tagged by `agent` and `band`. This enables fleet-level visibility into agent efficiency and loops.

6. **Configuration & No-Op**
   Telemetry must remain free when disabled. We rely purely on standard OpenTelemetry environment variables (e.g. `OTEL_EXPORTER_OTLP_ENDPOINT`). If `OTEL_EXPORTER_OTLP_ENDPOINT` is unset, the telemetry subsystem is a pure NO-OP: it allocates no exporters, starts no goroutines, and drops all spans, though the global propagator remains installed so wire context survives hops.

## Implementation Notes

- `pkg/weave` starts `weave.run` immediately before the agent process launch and ends it after terminal evidence is recorded.
- `weaveChildEnv` remains the child environment choke point for `weave start`; the active span context is injected into that environment as standard W3C `TRACEPARENT`/`TRACESTATE` entries.
- The `weave.run` span carries `agent`, `nick`, `band`, `tool:model`, `issue`, `outcome`, `converged`, `gate_result`, `turns`, and `duration`.
- `fleet.run.turns` is recorded as a histogram with `agent` and `band` attributes when the launched tool reports `num_turns`.
- Tests verify that a traceparent-aware mock agent creates an `agent.turn` span under the same trace and parent span, and that unset `OTEL_EXPORTER_OTLP_ENDPOINT` leaves telemetry disabled.
