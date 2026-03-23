Current Phase: Phase 3.3 — Interactive Mode Viewport Scroll Fix

## Allowed Work
- bug fixes
- missing features
- test improvements

## Disallowed Work
- architectural changes
- new major dependencies

## Issues

### In Scope (Phase 3.3)
1. **#22** — `fix: in interactive mode, the top of the config dump is not visible and cannot be scrolled to` ← **CURRENT**

### Carry-over (existing open issues, deferred to Phase 4+)
- **#11** (P2) — `prefix_rewrite` route action not captured
- **#13** (P3) — `weighted_clusters` not captured (traffic splitting)
- **#16** — RFE: `--rule <name>` when HTTPRoute rule name promoted to standard channel
- **#17** — Collect multi-route config dumps for E2E filter tests

## Notes

Phase 3.3 fixes a correctness bug in the interactive TUI (`internal/tui/tui.go`): the viewport was unconditionally pinned to the cursor line, hiding all content rendered above the first navigable filter (Listener, FilterChain, HCM headers). The fix replaces the "pin cursor to top" strategy with "scroll only when cursor leaves the visible area".
