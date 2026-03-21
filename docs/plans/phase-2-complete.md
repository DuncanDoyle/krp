# Phase 2 — HTTPRoute Selector (Complete)

**Completed:** 2026-03-18

## Summary

Added K8S HTTPRoute awareness to `krp`. Users can now narrow the output to a specific HTTPRoute (and optionally a specific rule index) by passing `--route`, `--route-ns`, and `--rule` flags to `krp dump`. The tool uses the deterministic Kgateway route naming convention (`httproute-<name>-<ns>-`) embedded in Envoy route names to filter the snapshot without making any K8S API calls.

## Design & Implementation Docs

- `docs/plans/2026-03-17-phase-2-httproute-selector-design.md`
- `docs/plans/2026-03-17-phase-2-httproute-selector-implementation.md`

## Deliverables

- New `internal/filter` package with `Filter(snapshot, FilterOptions)` — pure function, no K8S calls.
- `FilterOptions` carries `HTTPRouteName`, `HTTPRouteNamespace` (separate from Gateway namespace), and `RuleIndex`.
- Substring matching on the kgateway route name convention `httproute-<name>-<namespace>-`.
- Three new `krp dump` flags: `--route`, `--route-ns`, `--rule`.
- `--deployment` flag as alternative to `--gateway` for port-forwarding (podFinder strategy).
- 7 unit tests + 2 E2E tests against real config dumps.

## Deferred / Open Issues

- **#11** (P2) — `prefix_rewrite` route action not captured (carried from Phase 1)
- **#13** (P3) — `weighted_clusters` not captured (carried from Phase 1)
- **#16** — RFE: support `--rule <name>` when HTTPRoute rule name field is promoted to standard channel
- **#17** — Collect multi-route config dumps for E2E filter tests (scenarios 03, 04, 05)
