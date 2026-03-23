# Phase 3 — Filter Config Detail (Complete)

**Completed:** 2026-03-23

## Summary

Added an interactive `--interactive` / `-i` flag to `krp dump`. The flag launches a bubbletea TUI where the user can navigate HTTP filters with ↑/↓, expand typed configs inline as pretty-printed JSON (Enter/Space), toggle all expanded at once (a), and quit (q/Ctrl+C). Sub-phases 3.1–3.5 followed to fix review findings, close test gaps, and correct viewport scrolling behaviour.

## Design & Implementation Docs

- `docs/plans/2026-03-21-phase-3-filter-config-detail-design.md`
- `docs/plans/2026-03-21-phase-3-filter-config-detail-implementation.md`

## Sub-Phase Summary

| Sub-phase | Issues | Description |
|-----------|--------|-------------|
| 3 (main)  | —      | Core TUI implementation: `--interactive` flag, bubbletea model, expand/collapse, `FilterRef` navigation |
| 3.1       | doc fixes | Corrected inaccurate doc comments identified in Phase 3 review |
| 3.2       | #18 #19 #20 #21 | Closed test gaps: cursor+expanded, `resolveFilterConfig` unit tests, richer static-equivalence snapshot, empty HTTPFilters |
| 3.3       | #22    | Fix: top of config (Listener/FilterChain/HCM headers) was not visible on initial render — `scrollToCursor` replaces unconditional `SetYOffset` |
| 3.4       | #23    | Fix: scrolling back up did not reach the top — `cursor==0` resets `YOffset` to 0; added `TestMain` ANSI forcing to tui package |
| 3.5       | #24    | Fix: abrupt jump when arriving at item 0 replaced with line-by-line scrolling via ↑ when `cursor==0` and `YOffset>0` |

## Deliverables

- `internal/renderer/renderer_interactive.go` — `FilterRef`, `RenderOpts`, `RenderInteractive`
- `internal/tui/tui.go` — bubbletea `model`, `buildItems`, `scrollToCursor`, `setContent`, `findCursorLine`, `Run`
- `internal/tui/tui_test.go` — 12 tests covering navigation, scrolling, and edge cases
- `cmd/krp/main.go` — `--interactive` / `-i` flag wired
- Dependencies: `bubbletea v1.3.10`, `bubbles v1.0.0`
- 14 renderer tests + 12 tui tests

## Key Implementation Notes for Phase 4+

- `FilterRef` is a 5-int struct `{ListenerIdx, FilterChainIdx, VirtualHostIdx, RouteIdx, FilterIdx}` used as a map key for expanded state.
- `findCursorLine` scans for `\x1b[7m` (reverse-video ANSI); tui tests require `TestMain` with `lipgloss.SetColorProfile(termenv.ANSI)`.
- `scrollToCursor` adjusts `YOffset` only when the cursor leaves the visible area; ↑ at `cursor==0` continues scrolling the viewport line by line.
- Phase 5 polish item: `SetYOffset` is absolute; natural "scroll-follows-cursor" velocity is deferred.

## Deferred / Open Issues

- **#11** (P2) — `prefix_rewrite` route action not captured
- **#13** (P3) — `weighted_clusters` not captured (traffic splitting)
- **#16** — RFE: `--rule <name>` when HTTPRoute rule name promoted to standard channel
- **#17** — Collect multi-route config dumps for E2E filter tests
