# krp — Project Roadmap

**Date:** 2026-03-08

## Overview

`krp` (Kgateway Route Printer) is a CLI tool that visualizes the Envoy route configuration for Kubernetes Gateway API routes managed by Kgateway. The project is built in phases, progressing from raw Envoy config visualization to full K8S correlation.

Each phase has its own design and implementation document pair in `docs/plans/`.

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

### Phase 4 — K8S Correlation
**Status:** Not started

Correlate Envoy filters and routes back to originating K8S resources (Gateway, HTTPRoute, TrafficPolicy, EnterpriseKgatewayTrafficPolicy, etc.). Uses a layered matching strategy:
1. kgateway metadata in Envoy config (`filter_metadata`, `typed_per_filter_config` references)
2. Structural matching (VirtualHost domains, route match config, cluster naming `kube_<ns>_<svc>_<port>`)
3. Route name conventions (embeds HTTPRoute name/namespace)

Annotates each filter in the TUI with its K8S source resource.

### Phase 5 — Interactive Detail View
**Status:** Not started

Side-by-side Envoy config + K8S manifest view when selecting a filter. Search, `--json` output, and UX refinements.

---

## Architecture

Sequential pipeline (evolves per phase):

```
Phase 1:  CLI → Envoy Parser → Renderer
Phase 2:  CLI → K8S Resolver → Envoy Parser → Route Filter → Renderer
Phase 3:  CLI → K8S Resolver → Envoy Parser → Route Filter → Renderer (+ interactive config expansion)
Phase 4:  CLI → K8S Resolver → Envoy Parser → Correlator → Renderer
Phase 5:  CLI → K8S Resolver → Envoy Parser → Correlator → Renderer (+ detail views + JSON)
```

## Future Considerations

- **REST API / server mode:** `RouteGraph` model designed for JSON serialization. Adding `--serve` flag for a Web UI backend requires only a thin HTTP handler.
- **Config mismatch detection:** Surface K8S policies that exist but are absent from the Envoy config (translator bugs, xDS errors).
- **Global Policy Namespace:** Support policy attachment from a designated global namespace to targets in other namespaces.

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
