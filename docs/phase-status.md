Current Phase: Phase 3.5 — Interactive Mode Smooth Scrolling

## Allowed Work
- bug fixes
- missing features
- test improvements

## Disallowed Work
- architectural changes
- new major dependencies

## Issues

### In Scope (Phase 3.5)
1. **#24** — `fix: smooth scrolling when navigating back to the top in interactive mode` ← **CURRENT**

### Carry-over (existing open issues, deferred to Phase 4+)
- **#11** (P2) — `prefix_rewrite` route action not captured
- **#13** (P3) — `weighted_clusters` not captured (traffic splitting)
- **#16** — RFE: `--rule <name>` when HTTPRoute rule name promoted to standard channel
- **#17** — Collect multi-route config dumps for E2E filter tests

## Notes

Phase 3.5 replaces the abrupt jump-to-top from 3.4 with smooth line-by-line scrolling. `setContent()` reverts to pure `scrollToCursor` for all cursor positions; the ↑ key at cursor=0 with YOffset>0 decrements YOffset by 1 instead of being a no-op.
