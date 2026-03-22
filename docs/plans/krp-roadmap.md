# krp — Project Roadmap

**Date:** 2026-03-08
**Last updated:** 2026-03-22

## Overview

`krp` (Kgateway Route Printer) is a CLI tool that visualizes the configuration of Kubernetes Gateway API deployments. The project is built in two tracks that will eventually converge:

- **Envoy track (Phases 1–3):** Parse and visualize the Envoy config dump produced by kgateway — what the data-plane actually has programmed.
- **Gateway API track (Phases 4–7):** Parse and visualize the Kubernetes Gateway API resource graph — what the control-plane was configured to do.

Together, the two views let an operator understand both the intended configuration (Gateway API) and the realized configuration (Envoy), and spot mismatches between them.

Each phase has its own design and implementation document pair in `docs/plans/`.

See `docs/plans/possible-future-phases.md` for deferred and speculative work.

---

## Phases

### Phase 1 — Envoy Config Viewer
**Status:** Complete (patch cycles 1.1, 1.2, 1.3 applied)
**Docs:** `phase-1-envoy-viewer-design.md`, `phase-1-envoy-viewer-implementation.md`

CLI that parses a raw Envoy config dump (from file or live port-forward) and renders the complete structure: listeners, network-level filter chains, HCM, RDS route configs, virtual hosts, routes, HTTP filter pipeline, and backend clusters. No K8S awareness. Static TUI output.

### Phase 2 — HTTPRoute Selector
**Status:** Complete
**Docs:** `2026-03-17-phase-2-httproute-selector-design.md`, `2026-03-17-phase-2-httproute-selector-implementation.md`

Add K8S awareness to select a specific HTTPRoute and optionally a rule index. Uses the deterministic route naming convention (`httproute-<name>-<ns>`) embedded in Envoy route names to filter the view down to only the relevant listeners/routes/filters for that HTTPRoute.

### Phase 3 — Filter Config Detail
**Status:** Complete (patch cycles 3.1, 3.2 applied)
**Docs:** `2026-03-21-phase-3-filter-config-detail-design.md`, `2026-03-21-phase-3-filter-config-detail-implementation.md`

Add the ability to see what each filter does. Two interaction modes:
- **Select mode** — navigate to a filter, Enter to expand/collapse its typed config
- **All mode** — toggle key (e.g. `a`) to expand/collapse config for all filters at once

Typed config sources: `typed_per_filter_config` on route entries and filter-level config in the HCM `http_filters` array.

---

### Phase 4 — Gateway API DAG Printer
**Status:** Not started

Introduce a new command (`krp graph`) that reads Kubernetes Gateway API resources directly from the cluster and prints the resource graph as a tree/DAG in the terminal. The graph is walked starting from either a Gateway or an HTTPRoute (with TCPRoute/TLSRoute as future extensions).

The rendered DAG shows:
- **GatewayClass → Gateway → HTTPRoute** parent-child relationships
- **HTTPRoute rules** with their match conditions and backend refs
- **HTTP filters** attached via `filters` fields on the HTTPRoute (RequestHeaderModifier, ResponseHeaderModifier, URLRewrite, RequestMirror, etc.)
- **Listener** bindings (which Gateway listeners accept this HTTPRoute, based on `allowedRoutes`)

The goal is that an operator can immediately see how an HTTPRoute is wired: which Gateways it runs on, which listeners match it, and which filters are configured at each level.

**New CLI subcommand:**
```
krp graph --route <name> -n <ns>       # walk from an HTTPRoute
krp graph --gateway <name> -n <ns>     # walk from a Gateway
```

**Input sources:** live cluster (kubeconfig / `--context`) or a directory of YAML manifests (`--dir`).

**New packages:**
- `internal/k8s/` — K8S API reader (live and file-based)
- `internal/gwapi/` — Gateway API resource model and parser
- `internal/dag/` — DAG builder (walks parent/child references)
- `internal/dagrenderer/` — static tree renderer for the DAG (lipgloss, same style conventions as `internal/renderer`)

### Phase 5 — Policy Support
**Status:** Not started

Extend the DAG to include policies attached to Gateway API resources via `ExtensionRef` (inline, on HTTPRoute rules) and `TargetRef` / `TargetRefs` (standalone policy CRs targeting a Gateway or HTTPRoute).

**Policy tiers — rendered as distinct node types in the DAG:**
1. **Gateway API native policies** — `BackendLBPolicy`, `BackendTLSPolicy`, `SessionPersistencePolicy` (when standardised)
2. **kgateway policies** — `TrafficPolicy`, `VirtualHostOption`, `RouteOption`
3. **Solo Enterprise for kgateway policies** — `EnterpriseKgatewayTrafficPolicy` and others

**Plugin architecture:** The core DAG builder knows only about Gateway API-native policy attachment points. kgateway and Solo Enterprise policy types are registered as plugins that the DAG builder calls during graph construction. This keeps the core tool useful for any Gateway API implementation while allowing vendor-specific enrichment when the CRDs are present on the cluster.

**New packages:**
- `internal/policy/` — policy attachment model and resolution logic
- `internal/policy/kgateway/` — kgateway policy plugin
- `internal/policy/enterprise/` — Solo Enterprise for kgateway policy plugin

### Phase 6 — Status Detection and Error Marking
**Status:** Not started

Parse the `.status` field of every K8S resource in the DAG and surface configuration problems directly in the rendered graph.

**Two problem classes:**
- **Error status:** The controller processed the resource but reported a condition with `status: False` or `reason: Invalid/Error`. The node is marked with a red indicator and the condition message is shown inline.
- **Empty status:** The resource has been applied to the cluster but has no status conditions at all — meaning the kgateway controller has not yet processed it (e.g. CRD not installed, controller not running, or manifest applied to wrong cluster). Empty status is flagged as a distinct warning (distinct from an error) because it signals a controller-level problem rather than a misconfiguration in the manifest itself.

Operators can run `krp graph` after applying new configuration and immediately see which resources are healthy, which have errors, and which have not been picked up by the controller at all.

### Phase 7 — Interactive DAG Detail View
**Status:** Not started

Add interactive expand/collapse to the DAG renderer, similar to Phase 3's interactive Envoy view.

**Two display levels per node:**
- **Summary view (default):** Show the node type, name, namespace, and a one-line summary of its configuration (e.g. for an HTTPRoute: number of rules and backends; for a policy: the top-level policy fields).
- **Detail view (expanded):** Show the full resource spec as pretty-printed YAML or JSON inline below the node, using the same inline-expansion pattern as the Phase 3 filter config detail.

**Key bindings** follow the Phase 3 conventions: `↑/↓` to navigate, `Enter/Space` to expand/collapse, `a` to expand/collapse all, `q` to quit.

---

## Architecture

Sequential pipeline (evolves per track):

```
Envoy track:
  Phase 1:  CLI → Envoy Parser → Renderer
  Phase 2:  CLI → K8S Resolver → Envoy Parser → Route Filter → Renderer
  Phase 3:  CLI → K8S Resolver → Envoy Parser → Route Filter → Renderer (+ interactive expansion)

Gateway API track:
  Phase 4:  CLI → K8S Reader → GatewayAPI Parser → DAG Builder → DAG Renderer
  Phase 5:  CLI → K8S Reader → GatewayAPI Parser + Policy Plugins → DAG Builder → DAG Renderer
  Phase 6:  CLI → K8S Reader → GatewayAPI Parser + Policy Plugins → DAG Builder + Status Checker → DAG Renderer (+ error markers)
  Phase 7:  CLI → K8S Reader → GatewayAPI Parser + Policy Plugins → DAG Builder + Status Checker → Interactive DAG Renderer
```

---

## Test Scenarios

Real K8S + Envoy config pairs in `testdata/scenarios/`:

| Scenario | Description | Has config_dump |
|----------|-------------|:---:|
| 01-simple | HTTP Gateway, 1 route, no policies | Yes |
| 02_1-single-policy | HTTPS Gateway, transformation EKTP | Yes |
| 02_2-single-policy | HTTP, RequestHeaderModifier (native K8S filter) | Yes |
| 02_3-single-policy | HTTP, ResponseHeaderModifier (native K8S filter) | Yes |
| 02_4-single-policy | HTTP, RequestMirror (native K8S filter) | Yes |
| 02_5-single-policy | HTTP, URLRewrite (native K8S filter) | Yes |
| 02_6-single-policy | HTTP, CORS (EKTP) | Yes |
| 02_7-single-policy | HTTP, ExtAuth (EKTP) | Yes |
| 02_8-single-policy | HTTP, RateLimit (EKTP) | Yes |
| 03-multi-policy | Multiple policies on one route | No |
| 04-multi-rule | One route, multiple rules/backends | No |
| 05-multi-listener | Gateway with HTTP + HTTPS listeners | No |
