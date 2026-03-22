# krp — Possible Future Phases

These phases were part of an earlier roadmap iteration but were superseded by a new direction focused on the Kubernetes Gateway API resource graph. They are preserved here as they may still be relevant in a later context.

---

## Deferred: Envoy ↔ K8S Correlation

Correlate Envoy filters and routes back to originating K8S resources (Gateway, HTTPRoute, TrafficPolicy, EnterpriseKgatewayTrafficPolicy, etc.). Uses a layered matching strategy:
1. kgateway metadata in Envoy config (`filter_metadata`, `typed_per_filter_config` references)
2. Structural matching (VirtualHost domains, route match config, cluster naming `kube_<ns>_<svc>_<port>`)
3. Route name conventions (embeds HTTPRoute name/namespace)

Annotates each filter in the Envoy TUI with its K8S source resource, providing a bridge between the low-level Envoy view (Phases 1–3) and the K8S resource view (Phases 4–7).

---

## Deferred: Envoy Side-by-Side Detail View

Side-by-side Envoy config + K8S manifest view when selecting a filter in the Envoy TUI. Search, `--json` output, and UX refinements to the existing interactive mode.

---

## Future Considerations

- **REST API / server mode:** `RouteGraph` model designed for JSON serialization. Adding `--serve` flag for a Web UI backend requires only a thin HTTP handler.
- **Config mismatch detection:** Surface K8S policies that exist but are absent from the Envoy config (translator bugs, xDS errors).
- **Global Policy Namespace:** Support policy attachment from a designated global namespace to targets in other namespaces.
