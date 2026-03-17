# Phase 1 — Envoy Config Viewer (Complete)

**Date completed:** 2026-03-17
**Status:** Complete

## Summary

Phase 1 delivered a working CLI (`krp dump`) that fetches and visualises the complete Envoy filter chain configuration for a kgateway-managed Gateway, either from a local config_dump JSON file or live via kubectl port-forward. No K8S resource awareness — raw Envoy config only.

Phase 1 ran across four patch cycles (1.1–1.4). All planned features were delivered, all GitHub issues resolved.

---

## Patch Cycle Outcomes

| Cycle | Commit | Key deliverables |
|-------|--------|-----------------|
| 1.1 | a8fa710 | Route-level policies, disabled filter display fix |
| 1.2 | 106ff8f | Matcher support (path_separated_prefix, safe_regex, regex headers, query params), URLRewrite display, deep-copy fix, parser tests |
| 1.3 | 866ad0e | Design doc sync, TypedPerFilterConfig + Metadata deep-copy, renderer tests for all match types |
| 1.4 | 49e5025 | Parser warnings for malformed sections (ParseResult type), deferred issue cleanup |

---

## Delivered Features

- Parse Envoy config_dump JSON: listeners, HCM, RDS route configs, virtual hosts, routes, HTTP filters, clusters
- Live port-forward to gateway-proxy pod via `--gateway <name> -n <ns>`
- Port-forward to any Deployment pod via `--deployment <name> -n <ns>` (added during phase transition)
- Static TUI rendering with lipgloss
- Parser warnings surfaced to stderr for malformed sections (non-fatal)
- Route match types: prefix, exact path, path_separated_prefix, safe_regex, regex headers, query params, method
- Route actions: cluster, regex_rewrite, host_rewrite_literal, request/response header modifiers, mirror
- Filter pipeline: HCM-level http_filters with disabled-filter detection

## Test Scenarios

All 9 matcher scenarios (01_1–01_9) and all 9 single-policy scenarios (02_1–02_8) have config dumps and passing parser + renderer tests.

---

## Open / Deferred Issues

- **#11** (P2) — `prefix_rewrite` route action not captured
- **#13** (P3) — `weighted_clusters` not captured (traffic splitting)

---

## Sub-cycle Complete Records

- `docs/plans/phase-1.2-complete.md`
- `docs/plans/phase-1.3-complete.md`
- `docs/plans/phase-1.4-complete.md`
