Current Phase: Phase 4 — Gateway API DAG Printer (NOT STARTED)

## Allowed Work
- New `krp graph` subcommand
- Reading Gateway API resources from a live cluster (kubeconfig) or a directory of YAML manifests
- Parsing GatewayClass, Gateway, HTTPRoute resources into an internal DAG model
- Walking the DAG from a Gateway or HTTPRoute starting point
- Static tree renderer for the DAG (lipgloss, matching existing style conventions)
- Tests for the new parser, DAG builder, and renderer

## Disallowed Work
- Policy attachment (ExtensionRef / TargetRef) — Phase 5
- Status field parsing and error detection — Phase 6
- Interactive expand/collapse on DAG nodes — Phase 7
- TCPRoute / TLSRoute support — future extension
- Envoy ↔ K8S correlation — deferred (see `docs/plans/possible-future-phases.md`)
- Architectural changes to the existing Envoy track (Phases 1–3)

## Issues

### Carry-over (existing open issues)
- **#11** (P2) — `prefix_rewrite` route action not captured
- **#13** (P3) — `weighted_clusters` not captured (traffic splitting)
- **#16** — RFE: `--rule <name>` when HTTPRoute rule name promoted to standard channel
- **#17** — Collect multi-route config dumps for E2E filter tests

## Notes

Phase 4 introduces a new `krp graph` command alongside the existing `krp dump`. The two commands are independent — `krp graph` does not depend on an Envoy config dump. The K8S reader, Gateway API parser, DAG builder, and DAG renderer are all new packages.

Starting point for traversal:
- `--gateway <name> -n <ns>` — walk from a Gateway outward to all attached HTTPRoutes
- `--route <name> -n <ns>` — walk from an HTTPRoute inward to its parent Gateways and outward to its backends

See `docs/plans/krp-roadmap.md` for full phase context.
