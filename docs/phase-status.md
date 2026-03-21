Current Phase: Phase 3 — Filter Config Detail (IN PROGRESS)

## Allowed Work
- Expanding/collapsing typed filter config in the rendered output
- Parsing `typed_per_filter_config` on route entries
- Parsing filter-level config from HCM `http_filters`
- Interactive TUI using bubbletea (select mode and all-expand mode)
- Tests for the new interaction and rendering logic

## Disallowed Work
- K8S API calls — Phase 4
- K8S resource correlation — Phase 4
- Architectural changes unrelated to filter config detail

## Issues

- **#11** (P2) — `prefix_rewrite` route action not captured (carry-over)
- **#13** (P3) — `weighted_clusters` not captured (carry-over)
- **#16** — RFE: `--rule <name>` when HTTPRoute rule name promoted to standard channel
- **#17** — Collect multi-route config dumps for E2E filter tests

## Notes

Phase 3 goal: surface what each HTTP filter does by expanding its typed config inline in the TUI.

Two interaction modes:
- **Select mode** — navigate to a filter with arrow keys, press Enter to expand/collapse its typed config.
- **All mode** — press `a` to expand/collapse config for all filters at once.

Typed config sources:
- `typed_per_filter_config` on individual route entries (per-route filter overrides/activations)
- Filter-level config in the HCM `http_filters` array (global filter config)

See `docs/plans/krp-roadmap.md` for full phase context.
