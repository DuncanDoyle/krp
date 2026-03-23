Current Phase: Phase 3.4 — Interactive Mode Viewport Scroll Fix (follow-up)

## Allowed Work
- bug fixes
- missing features
- test improvements

## Disallowed Work
- architectural changes
- new major dependencies

## Issues

### In Scope (Phase 3.4)
1. **#23** — `fix: in interactive mode, scrolling back up does not reach the top of the configuration` ← **CURRENT**

### Carry-over (existing open issues, deferred to Phase 4+)
- **#11** (P2) — `prefix_rewrite` route action not captured
- **#13** (P3) — `weighted_clusters` not captured (traffic splitting)
- **#16** — RFE: `--rule <name>` when HTTPRoute rule name promoted to standard channel
- **#17** — Collect multi-route config dumps for E2E filter tests

## Notes

Phase 3.4 is a follow-up to 3.3 (#22). After 3.3, navigating back up to cursor item 0 scrolls the viewport to show the cursor line but not the headers above it. The fix: when `cursor == 0`, reset viewport offset to 0 unconditionally.
