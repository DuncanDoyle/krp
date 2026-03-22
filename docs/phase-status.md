Current Phase: Phase 4 — K8S Correlation (NOT STARTED)

## Allowed Work
- Reading K8S resources (Gateway, HTTPRoute, TrafficPolicy, EnterpriseKgatewayTrafficPolicy) via the K8S API
- Correlating Envoy filters and routes back to K8S source resources using:
  - kgateway `filter_metadata` cross-references
  - `typed_per_filter_config` key matching
  - Cluster naming convention (`kube_<ns>_<svc>_<port>`)
  - Route name conventions (embeds HTTPRoute name/namespace)
- Annotating filters and routes in the TUI with their K8S source resource
- Tests for the new correlation logic

## Disallowed Work
- Side-by-side K8S manifest view — Phase 5
- `--json` output flag — Phase 5
- Search within the TUI — Phase 5
- Architectural changes unrelated to K8S correlation

## Issues

### Phase 4 scope
- **#11** (P2) — `prefix_rewrite` route action not captured (carry-over)
- **#13** (P3) — `weighted_clusters` not captured (carry-over)
- **#16** — RFE: `--rule <name>` when HTTPRoute rule name promoted to standard channel
- **#17** — Collect multi-route config dumps for E2E filter tests

### Phase 3.2 (test gaps — resolve before Phase 4 feature work)
- **#18** — test: add RenderInteractive test covering cursor + expanded on same item
- **#19** — test: add direct unit tests for resolveFilterConfig
- **#20** — test: add richer static-equivalence regression test for RenderInteractive
- **#21** — test: add buildItems test for HCM with empty HTTPFilters slice

## Notes

Phase 4 goal: annotate each HTTP filter and route in the TUI output with the K8S resource that produced it. Uses a layered matching strategy (metadata → structural → naming convention) so the tool works without requiring a live cluster for filters that embed sufficient metadata.

See `docs/plans/krp-roadmap.md` for full phase context.
