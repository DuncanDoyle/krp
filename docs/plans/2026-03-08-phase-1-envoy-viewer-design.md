# Phase 1 — Envoy Config Viewer: Design Document

**Date:** 2026-03-08
**Phase:** 1 of 5 (see `2026-03-08-kfp-roadmap.md`)

## Goal

Build a CLI that parses a raw Envoy config dump and renders the complete Envoy configuration structure as a rich terminal visualization: listeners, network-level filter chains, HCM configuration, RDS route configs, virtual hosts, routes, HTTP filter pipeline, and backend clusters. No K8S awareness — this is purely an Envoy config viewer.

---

## Input Modes

```bash
# From a file (offline analysis, testdata scenarios)
kfp dump --file testdata/scenarios/01-simple/envoy/config_dump.json

# From a live cluster (auto port-forward to gateway-proxy pod)
kfp dump --gateway gw -n kgateway-system

# Override kubeconfig context for live mode
kfp dump --gateway gw -n kgateway-system --context my-cluster
```

The `--file` mode is the priority — it lets us develop and test against real testdata without needing a live cluster. The `--gateway` mode finds the first ready pod with label `gateway.networking.k8s.io/gateway-name=<name>` in the given namespace, port-forwards to admin port 19000, and fetches `/config_dump`.

---

## Config Dump Parsing

The Envoy `/config_dump` response contains a `configs` array with typed sections. Phase 1 parses three:

### ListenersConfigDump
`@type: type.googleapis.com/envoy.admin.v3.ListenersConfigDump`

Contains `dynamic_listeners` array. Each listener has network-level filter chains, each containing an HCM (`envoy.filters.network.http_connection_manager`). The HCM contains:
- `http_filters` array — the HTTP filter pipeline (e.g. `io.solo.transformation`, `envoy.filters.http.cors`, `envoy.filters.http.router`)
- `rds.route_config_name` — reference to the route config (VirtualHosts are NOT inline)

For HTTPS listeners, each filter chain may have a `transport_socket` with TLS context for SNI-based routing.

### RoutesConfigDump
`@type: type.googleapis.com/envoy.admin.v3.RoutesConfigDump`

Contains `dynamic_route_configs` array. Each entry has a route config with:
- `name` — matches the HCM's `rds.route_config_name`
- `virtual_hosts` — array of VirtualHosts with `domains` and `routes`

Route names are deterministic and contain the HTTPRoute name: `<vh>-route-<idx>-httproute-<name>-<ns>-<rule>-<match>-matcher-<idx>`

### ClustersConfigDump
`@type: type.googleapis.com/envoy.admin.v3.ClustersConfigDump`

Contains `dynamic_active_clusters` array. Cluster names follow `kube_<ns>_<svc>_<port>`.

### Joining the sections

The parser joins listeners to route configs by matching `rds.route_config_name` in the HCM to the route config `name` in the RDS section. Clusters are referenced by name from route entries.

---

## Data Model

```go
package model

// EnvoySnapshot is the complete parsed Envoy configuration.
type EnvoySnapshot struct {
    Listeners []Listener
}

type Listener struct {
    Name         string
    Address      string              // e.g. "0.0.0.0:80"
    FilterChains []NetworkFilterChain
}

type NetworkFilterChain struct {
    Name string
    TLS  *TLSContext  // nil for plaintext listeners
    HCM  *HCMConfig   // extracted from the network filter list
}

type TLSContext struct {
    SNIHosts   []string // SAN entries from the certificate
    CertPath   string   // path to cert (informational)
}

type HCMConfig struct {
    RouteConfigName string       // rds.route_config_name
    HTTPFilters     []HTTPFilter // the filter pipeline
    RouteConfig     *RouteConfig // joined from RDS section
}

type RouteConfig struct {
    Name         string
    VirtualHosts []VirtualHost
}

type VirtualHost struct {
    Name    string
    Domains []string
    Routes  []Route
}

type Route struct {
    Name    string
    Match   RouteMatch
    Cluster string // backend cluster name
}

type RouteMatch struct {
    Prefix  string
    Path    string
    Regex   string
    Headers []HeaderMatch
}

type HeaderMatch struct {
    Name  string
    Value string
}

type HTTPFilter struct {
    Name        string         // e.g. "io.solo.transformation"
    TypedConfig map[string]any // raw config, stored but not rendered in Phase 1
}
```

---

## Package Structure

```
cmd/kfp/main.go              CLI entrypoint (cobra)
internal/
  model/                      EnvoySnapshot and related types
  parser/                     Envoy config dump JSON parser
  envoy/                      Port-forwarder (admin API access)
  renderer/                   lipgloss static TUI renderer
```

---

## TUI Output

Static lipgloss panels (no interactivity in Phase 1). Multiple listeners render as separate panels stacked vertically.

```
╭─ Listener: listener~443 ──────────────────────────────── 0.0.0.0:443 ─╮
│                                                                         │
│  FilterChain[0]  TLS: api.example.com                                   │
│  └─ HCM → RDS: https-api                                               │
│     └─ VirtualHost: https-api~api_example_com [api.example.com]         │
│        └─ Route: / (prefix)                                             │
│           HTTP Filters:                                                 │
│           ├─ [1] io.solo.transformation                                 │
│           └─ [2] envoy.filters.http.router                              │
│           Backend: kube_httpbin_httpbin_8000                             │
│                                                                         │
│  FilterChain[1]  TLS: developer.example.com                             │
│  └─ HCM → RDS: https-developer                                         │
│     └─ VirtualHost: https-developer~developer_example_com [...]         │
│        └─ Route: / (prefix)                                             │
│           HTTP Filters:                                                 │
│           ├─ [1] io.solo.transformation                                 │
│           └─ [2] envoy.filters.http.router                              │
│           Backend: kube_httpbin_httpbin_8000                             │
╰─────────────────────────────────────────────────────────────────────────╯

╭─ Listener: listener~80 ───────────────────────────────── 0.0.0.0:80 ──╮
│                                                                         │
│  FilterChain[0]                                                         │
│  └─ HCM → RDS: listener~80                                             │
│     └─ VirtualHost: listener~80~api_example_com [api.example.com]       │
│        └─ Route: / (prefix)                                             │
│           HTTP Filters:                                                 │
│           ├─ [1] envoy.filters.http.router                              │
│           Backend: kube_httpbin_httpbin_8000                             │
╰─────────────────────────────────────────────────────────────────────────╯
```

---

## Error Handling

| Situation | Behavior |
|---|---|
| Malformed JSON | Fail with parse error |
| `--file` not found | Fail with file-not-found error |
| Port-forward fails (`--gateway` mode) | Fail fast with error + RBAC hint |
| No gateway pod found | Fail with "no ready pod found" + label selector hint |
| Config dump missing listeners | Warn "no listeners found in config dump" |
| Config dump missing RDS | Render listeners without route detail, warn about missing RDS |
| HCM route_config_name not found in RDS | Show `[RDS not found: <name>]` inline |

---

## Dependencies

| Package | Purpose |
|---|---|
| `github.com/spf13/cobra` | CLI framework |
| `k8s.io/client-go` | Port-forward + kubeconfig (for `--gateway` mode) |
| `github.com/charmbracelet/lipgloss` | TUI styling |

Phase 1 does NOT need: `controller-runtime`, `gateway-api`, `bubbletea` (static output only), `go-control-plane`.

`bubbletea` will be added in Phase 2 when interactivity is introduced.

---

## Testing Strategy

Unit tests use the real config dump fixtures from `testdata/scenarios/`:
- `01-simple/envoy/config_dump.json` — single HTTP listener, no policies
- `02_1-single-policy/envoy/config_dump.json` — HTTPS listener with two filter chains (SNI), transformation filter
- `02_7-single-policy/envoy/config_dump.json` — ext_authz filter (multiple HCM filters)

Tests verify:
- Parser correctly extracts listeners, filter chains, HCM filters, and joins RDS
- Renderer output contains expected listener names, VirtualHost domains, filter names, cluster names
- Error cases: malformed JSON, missing RDS references
