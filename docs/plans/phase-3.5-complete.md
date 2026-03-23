# Phase 3.5 — Smooth Scrolling (Complete)

**Completed:** 2026-03-23
**Issue:** #24

## Summary

Replaced the abrupt viewport jump introduced in sub-phase 3.4 with smooth line-by-line scrolling. When the cursor reaches item 0, pressing ↑ again now decrements `YOffset` by one per keypress instead of jumping the viewport instantly to offset 0.

## Root Cause

Two problems combined to cause the jump:

1. `setContent()` special-cased `cursor == 0` and called `SetYOffset(0)` unconditionally — a single keypress teleported the viewport to the top.
2. The ↑ key handler was a no-op when `cursor == 0`, so there was no way to reach lines above the first navigable filter gradually.

## Changes

**`internal/tui/tui.go`**

- `setContent()`: removed the `cursor == 0` branch; `scrollToCursor` is now used for all cursor positions uniformly.
- `Update()` / `"up"/"k"` case: added `else if m.viewport.YOffset > 0 { SetYOffset(YOffset - 1) }` so each ↑ press scrolls one line when the cursor is already at item 0.

**`internal/tui/tui_test.go`**

- Replaced `TestSetContent_CursorAtFirstItem_ResetsOffset` (which tested the now-removed jump-to-0 behaviour) with:
  - `TestUpdate_UpKey_AtFirstItem_ScrollsViewportUp` — ↑ at cursor=0 with YOffset>0 decrements by 1.
  - `TestUpdate_UpKey_AtFirstItem_AtTop_IsNoop` — ↑ at cursor=0 with YOffset=0 is a no-op.

## Test Results

All tests pass (`go test ./...`):

- `internal/tui` — 12 tests, PASS
- All other packages — cached, PASS

## Deferred Issues

- **#11** (P2) — `prefix_rewrite` route action not captured
- **#13** (P3) — `weighted_clusters` not captured
- **#16** — RFE: `--rule <name>` when HTTPRoute rule name promoted to standard channel
- **#17** — Collect multi-route config dumps for E2E filter tests
