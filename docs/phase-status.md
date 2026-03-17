Current Phase: Phase 2 — HTTPRoute Selector (COMPLETE)

## Allowed Work
- new filter package and CLI flags
- route-name substring matching
- tests for the filter layer

## Disallowed Work
- interactive TUI (bubbletea) — Phase 3
- K8S API calls — Phase 4
- architectural changes unrelated to the selector feature

## Issues

No GitHub issues were opened for Phase 2 — all work was captured in the implementation plan.

## Notes

Phase 2 is complete. See `docs/plans/2026-03-17-phase-2-httproute-selector-design.md` and
`docs/plans/2026-03-17-phase-2-httproute-selector-implementation.md` for the full record.

### What was delivered

- New `internal/filter` package with `Filter(snapshot, FilterOptions)` — pure function, no K8S calls.
- `FilterOptions` carries `HTTPRouteName`, `HTTPRouteNamespace` (separate from Gateway namespace), and `RuleIndex`.
- Substring matching on the kgateway route name convention `httproute-<name>-<namespace>-`.
- Three new `krp dump` flags: `--route`, `--route-ns`, `--rule`.
- 7 unit tests + 2 E2E tests against real config dumps.

### Deferred / open

- **#11** (P2) — `prefix_rewrite` route action not captured (carried from Phase 1)
- **#13** (P3) — `weighted_clusters` not captured (carried from Phase 1)
- **#16** — RFE: support `--rule <name>` when HTTPRoute rule name field is promoted to standard channel
