# ADR-0006: Grafana Loki Backend Integration

## Status

ACCEPTED — 2026-05-28.

## Context

ax-go CLIs emit structured JSON logs to `stderr` (per the Golden Rule).
Centralizing those logs in Grafana Loki gives unified search across the
rshade portfolio. Loki indexes only labels (not the payload), which fits
ZeroLog's high-throughput JSON output well — assuming label cardinality
is controlled.

## Decision Drivers

- Logs from short-lived CLIs must reach Loki reliably without blocking
  the CLI's own execution time.
- Network failures must not break the CLI's primary work.
- Label cardinality discipline is non-negotiable — high-cardinality labels
  destroy Loki performance.
- Deployment shape varies: containerized (Talos K8s) and standalone
  binaries (home-lab, edge, dev workstations) both matter.

## Considered Options

### Approach 1: Decoupled (Promtail / Grafana Alloy DaemonSet)

The CLI writes JSON to `stderr`. In containerized environments, a
Promtail or Alloy DaemonSet tails the container's `stderr` and ships to
Loki with auto-attached labels (pod, namespace, node).

Pros: zero in-CLI network overhead; library remains backend-agnostic;
labels handled by infrastructure.
Cons: useless for standalone binaries running outside Kubernetes.

### Approach 2: Direct Push (HTTP client in the CLI to Loki Push API)

A buffered, non-blocking HTTP writer batches log lines and POSTs to
`/loki/api/v1/push`. Auth and trace headers handled in the writer.

Pros: works for standalone binaries; no infrastructure dependency.
Cons: network overhead on CLI execution; the base package becomes
backend-aware; requires retry / drop-on-full-buffer logic.

### Approach 3: Both — decoupled by default, direct push opt-in

`ax-go` defaults to Approach 1 (write `stderr`; let infra ship). When the
`AX_LOKI_URL` env var (or config field) is set, the base auto-wires
Approach 2 alongside `stderr` output.

Pros: covers both deployment shapes; consumer does not pre-commit at
build time.
Cons: two code paths to maintain; failure modes differ.

## Label Cardinality Rule (applies regardless of approach)

The base package enforces this distinction:

- **Labels (low-cardinality, indexed by Loki):** `environment`,
  `application` (tool name), `level`, `host`, `version`.
- **JSON payload (high-cardinality, unindexed):** `trace_id`, `span_id`,
  `user_id`, raw error messages, durations, resource IDs.

`trace_id` must never be promoted to a label. The ZeroLog wrappers
provided by ax-go must enforce this at the type-system level — label
fields and payload fields are separate APIs.

## Decision

Adopt **Approach 3** (both, env-gated).

Default is Approach 1 (decoupled — write to `stderr`, let infrastructure
ship to Loki). When `AX_LOKI_URL` is set, auto-enable Approach 2 (direct
push) as an additional sink alongside `stderr`. The cardinality rule
above is enforced by the base logger API at the type level (label fields
and payload fields are separate APIs).

Rationale: covers both deployment shapes (containerized + standalone)
without forcing consumers to pre-commit at build time.

## Consequences

- Approach 1 alone keeps the base zero-dependency for log shipping.
- Approach 2 (or 3) adds a Loki client dependency and retry logic.
- Cardinality enforcement requires shaping the public logger API to
  separate "label" fields from "payload" fields explicitly.
- ZeroLog's existing context-builder pattern can carry label state;
  payload fields use the standard `.Str` / `.Int` methods.
