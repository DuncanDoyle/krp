# Phase 1.4 — Complete Record

**Date:** 2026-03-15
**Status:** Complete

## Summary

Phase 1.4 was the fourth patch cycle for Phase 1 (Envoy Config Viewer). Two issues (#5, #6) were already implemented in earlier phases but never closed on GitHub — verified and closed. One issue (#12) received a new implementation: the parser now surfaces non-fatal warnings instead of silently dropping malformed config sections.

---

## Issues Resolved

### #5 — Add parser tests for scenarios 02_5, 02_6, and 02_8 (already implemented)

**Status at phase start:** OPEN on GitHub, but implementation was complete since Phase 1.2 (commit 106ff8f).

**Verified:** `TestParse_URLRewrite`, `TestParse_CORSPolicy`, `TestParse_RateLimit` all present and passing.

**Action:** Closed with reference to Phase 1.2 commit.

---

### #6 — Deep-copy RouteConfig when assigning to multiple HCMs (already implemented)

**Status at phase start:** OPEN on GitHub, but implementation was complete since Phase 1.2 (commit 106ff8f), with further improvement in Phase 1.3 (commit 866ad0e / issue #8 adding map deep-copy).

**Verified:** `TestCloneRouteConfig_MapsAreIndependent`, `TestCloneRouteConfig_NilMaps` both passing.

**Action:** Closed with reference to Phase 1.2 and 1.3 commits.

---

### #12 — Parser silently drops malformed config sections

**Type:** Bug / quality-of-life.

**Root cause:** Two `continue` statements in `Parse()` swallowed unmarshal errors for individual config sections. A valid overall config dump with one corrupted section (e.g. wrong field type for `dynamic_listeners`) would return a partial snapshot with no indication that data was skipped.

**Fix:** Changed `Parse()` return type from `(*model.EnvoySnapshot, error)` to `(ParseResult, error)` where:
```go
type ParseResult struct {
    Snapshot *model.EnvoySnapshot
    Warnings []string
}
```
Non-fatal section failures now append to `Warnings` with a descriptive message. The CLI (`cmd/kfp/main.go`) prints warnings to stderr before rendering. Hard errors (malformed outer JSON) still return an error.

**Files changed:**
- `internal/parser/parser.go` — `ParseResult` type, updated `Parse` signature, warnings collection
- `cmd/kfp/main.go` — consume `ParseResult`, print warnings to stderr
- `internal/parser/parser_test.go` — all 10 `parser.Parse` call sites updated; added `TestParse_MalformedSection` and `TestParse_WarningForUnreadableType`
- `internal/parser/e2e_test.go` — 2 call sites updated

**No design doc or implementation plan changes required** — the error handling table in the design doc already reflects the spirit of surfacing problems to the user.

**Safety check:** `Parse()` is only called from `main.go`, `parser_test.go`, and `e2e_test.go` — all within this repo. Future phases call `Parse` the same way; they gain access to warnings for free.

---

## Test Results

All tests pass (`go test ./...`):
- `internal/parser` — 0.278s
- `internal/renderer` — cached
- `internal/model` — cached
- `internal/envoy` — cached

## Deferred Issues (future cycles)

- **#11** (P2) — `prefix_rewrite` route action not captured
- **#13** (P3) — `weighted_clusters` not captured (traffic splitting)
