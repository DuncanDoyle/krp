Current Phase: Phase 1.1 — Envoy Config Viewer Patch Cycle

## Allowed Work
- bug fixes
- missing features
- test improvements

## Disallowed Work
- architectural changes
- new major dependencies

## Issues (in order)

- [x] **#1** — Support displaying "policies" that are configured on a route instead of in a filter
- [x] **#2** — Transformation policy shows as "disabled"

## Notes

Issues #1 and #2 were resolved together:
- **#1**: Added `RequestHeadersToAdd`, `ResponseHeadersToAdd`, `ResponseHeadersToRemove`, and `MirrorClusters` to `model.Route`. Parser extracts these from `request_headers_to_add`, `response_headers_to_add`, `response_headers_to_remove`, and `route.request_mirror_policies`. Renderer displays them under "Route Policies".
- **#2**: `renderHTTPFilters` now receives the route's `TypedPerFilterConfig`. A filter marked `disabled: true` at HCM level is not rendered as "(disabled)" when the route has a matching entry in `typed_per_filter_config`.
