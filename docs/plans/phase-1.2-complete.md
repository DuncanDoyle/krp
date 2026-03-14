# Phase 1.2 — Complete

**Date:** 2026-03-14
**Branch:** main
**All tests passing:** yes

---

## Issues Resolved

### #4 — URLRewrite HTTPRouteFilter not displayed in route output

**Type:** Missing feature

**Root cause:** The `regex_rewrite` field in the Envoy route action was not captured in the raw JSON struct, not stored in the model, and not rendered.

**Changes:**
- `model/envoy.go`: Added `RouteRewrite` type (`RegexPattern`, `Substitution`). Added `Rewrite *RouteRewrite` field to `Route`.
- `parser/parser.go`: Added `RegexRewrite` struct to `rawRoute.Route`. Populated `route.Rewrite` from `regex_rewrite.pattern.regex` / `regex_rewrite.substitution`.
- `renderer/renderer.go`: Added rewrite line to `renderRoutePolicies`: `rewrite: <pattern> → <substitution>`.

---

### #3 — Parser does not capture `path_separated_prefix` or `safe_regex` match types

**Type:** Missing feature (broader than stated — also covered query_parameters and regex header matching for the 01_x scenarios)

**Root cause:** `rawRoute.Match` only had `prefix` and `path` fields. `path_separated_prefix`, `safe_regex` (path), `safe_regex` (header), and `query_parameters` were silently dropped.

**Changes:**
- `model/envoy.go`: Added `PathSeparatedPrefix string` and `QueryParams []QueryParamMatch` to `RouteMatch`. Added `Regex bool` to `HeaderMatch`. Added new `QueryParamMatch` type.
- `parser/parser.go`: Extended `rawRoute.Match` with `PathSeparatedPrefix`, `SafeRegex`, `QueryParameters` fields and `safe_regex` inside header/query param `StringMatch`. Populated all new model fields. Header parsing now distinguishes exact vs regex via `safe_regex` presence.
- `renderer/renderer.go`: `formatMatch` now displays `path-prefix` (path_separated_prefix), `regex`, regex headers (`~` separator), and query params.

---

### #5 — Add parser tests for scenarios 02_5, 02_6, and 02_8

**Type:** Missing tests

**Changes:**
- `parser/parser_test.go`: Added `TestParse_URLRewrite` (02_5 — path_separated_prefix + regex_rewrite), `TestParse_CORSPolicy` (02_6 — cors typed_per_filter_config), `TestParse_RateLimit` (02_8 — ratelimit_ee/default typed_per_filter_config).
- Also fixed pre-existing bug: `TestParse_SimpleHTTP` referenced `01-simple` instead of `00-simple`.

---

### #6 — Deep-copy RouteConfig when assigning to multiple HCMs

**Type:** Bug (defensive fix)

**Root cause:** `parseListener` assigned the same `*model.RouteConfig` pointer to all HCMs sharing a `route_config_name`. Future mutations would corrupt shared state.

**Changes:**
- `parser/parser.go`: Added `cloneRouteConfig(*model.RouteConfig) *model.RouteConfig` helper. All slice fields (Domains, Routes, Headers, QueryParams, MirrorClusters, etc.) and the `Rewrite` pointer are deep-copied. Called in `parseListener` when joining HCM to route config.

---

### e2e — Matcher scenario tests for 01_1 through 01_9

**Type:** New tests

**Changes:**
- `parser/e2e_test.go`: Added `parseMatcherScenario` helper (skips if dump missing), `firstRoute` helper, and 9 scenario-specific test functions (`TestMatcherScenario_01_1_PathPrefix` through `TestMatcherScenario_01_9_ExactMethodQueryParam`). All 9 passed against real config dumps.

**What each test verifies:**

| Test | Scenario | Key assertions |
|------|----------|----------------|
| 01_1 | PathPrefix + URLRewrite | `PathSeparatedPrefix="/api/v1"`, `Rewrite.Substitution="/"` |
| 01_2 | Exact path + URLRewrite | `Path="/api/v1/users"`, `Rewrite != nil` |
| 01_3 | RegEx path + URLRewrite | `Regex != ""`, `Rewrite != nil` |
| 01_4 | PathPrefix + Exact header | `PathSeparatedPrefix != ""`, header `x-api-version=v1`, `Regex=false` |
| 01_5 | PathPrefix + Regex header | header `x-client-id`, `Regex=true` |
| 01_6 | Method GET | `:method=GET` header match present |
| 01_7 | PathPrefix + QueryParam | `QueryParams[0].Name="format"`, `Value="json"` |
| 01_8 | PathPrefix + 2 headers | `PathSeparatedPrefix != ""`, `len(Headers) >= 2` |
| 01_9 | Exact + Method + QueryParam | `Path != ""`, `:method=GET` header, `len(QueryParams) > 0` |
