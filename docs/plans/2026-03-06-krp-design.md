# krp — Kgateway Filter Chain Printer: Design Document

**Date:** 2026-03-06

## Overview

`krp` is a CLI tool that takes an HTTPRoute on a running Kubernetes cluster and produces a rich terminal visualization of the exact Envoy filter chain that traffic traverses — from the Gateway listener, through each filter (auth, rate limiting, transformations, etc.), to the backend service. Each filter in the visualization is annotated with a reference to the originating Kubernetes Gateway API resource.

The primary use case is debugging and understanding the runtime behavior of a Kgateway-managed gateway, bridging the gap between the declarative K8S configuration and the actual Envoy configuration programmed by the control plane.

---

## Architecture

The tool is structured as a sequential pipeline:

```
CLI input → Resolver → Envoy Fetcher → Correlator → Renderer
```

```
cmd/kfp/main.go          CLI entrypoint (cobra)
internal/
  resolver/              K8S resource graph walker
  envoy/                 Envoy admin API client + port-forwarder
  correlator/            Merges K8S + Envoy data into unified RouteGraph
  model/                 Shared data structures (RouteGraph)
  renderer/              TUI rendering (bubbletea + lipgloss)
```

### Pipeline stages

1. **Resolver** — Uses `client-go` and the Gateway API types to walk the resource graph: HTTPRoute → Gateway → attached policies (via ExtensionRef and TargetRef) → backend Services.

2. **Envoy Fetcher** — Locates the gateway-proxy pod(s) by inspecting the Gateway's `spec.gatewayClassName`, opens a port-forward to the Envoy admin port (`19000`), and fetches `/config_dump`. Fails fast with a clear error message if the port-forward cannot be established (pod not ready, RBAC issue, etc.).

3. **Correlator** — Merges the K8S resource graph and the Envoy config dump into a `RouteGraph` using a layered matching strategy:
   - **Metadata** (primary): kgateway embeds K8S resource references directly in the Envoy config; use these where present.
   - **Structural** (secondary): Match by VirtualHost domains ↔ HTTPRoute hostnames, route match config ↔ HTTPRoute rule matches, cluster names ↔ backend Services.
   - **Convention** (last resort): Match by filter type and name (e.g. `envoy.filters.http.jwt_authn` → AuthPolicy).

4. **Renderer** — Takes the `RouteGraph` and renders an interactive TUI using `bubbletea` and `lipgloss`.

---

## Data Model

The `RouteGraph` is the central artifact produced by the Correlator. It is designed to be clean enough to later serialize to JSON for a future REST API without changes to any other component.

```go
type RouteGraph struct {
    HTTPRoute K8SRef
    Rule      int  // -1 = all rules
    Gateway   GatewayNode
}

type GatewayNode struct {
    Ref      K8SRef
    Listener ListenerNode
}

type ListenerNode struct {
    Name     string
    Protocol string // HTTP, HTTPS, TLS
    Port     int
    Chain    FilterChain
}

type FilterChain struct {
    Filters []FilterNode
    Backend BackendNode
}

type FilterNode struct {
    // Envoy identity
    EnvoyName   string // e.g. "envoy.filters.http.jwt_authn"
    EnvoyConfig any    // raw typed config for detail/verbose view

    // K8S origin
    PolicyRef   *K8SRef     // nil if no K8S source found
    MatchMethod MatchMethod // Metadata | Structural | Convention

    // TUI display
    Label   string   // e.g. "JWT Auth"
    Details []string // e.g. ["issuer: https://...", "audience: my-api"]
}

type BackendNode struct {
    Refs []BackendRef
}

type BackendRef struct {
    ServiceRef K8SRef
    Port       int
    Weight     int
}

type K8SRef struct {
    Kind      string
    Name      string
    Namespace string
}

type MatchMethod int
const (
    MatchMetadata   MatchMethod = iota
    MatchStructural
    MatchConvention
)
```

`MatchMethod` is retained internally for verbose/debug output but does not surface in the default TUI view.

---

## CLI Interface

```bash
# Inspect all rules of an HTTPRoute
krp route <name> -n <namespace>

# Pin to a specific rule (zero-indexed)
krp route <name> -n <namespace> --rule 0

# Override kubeconfig context
krp route <name> -n <namespace> --context my-cluster

# Show raw Envoy typed config and MatchMethod in expanded panels
krp route <name> -n <namespace> --verbose
```

---

## TUI Behavior

- Renders a static set of lipgloss panels by default.
- `j`/`k` or arrow keys to move between filter nodes.
- `Enter` to expand a selected filter — shows Envoy typed config and the originating K8S policy manifest side by side.
- `q` / `Esc` to collapse or exit.

Example layout:

```
┌─ HTTPRoute: my-api-route ──────────────────────── namespace: default ─┐
│  Gateway: prod-gateway  │  Listener: https:443                         │
└───────────────────────────────────────────────────────────────────────┘

  FILTER CHAIN
  ┌───────────────────────────────────────────────────────────────────┐
  │  1  JWT Auth                              [AuthPolicy: jwt-policy] │
  │     issuer:   https://auth.example.com                            │
  │     audience: my-api                                              │
  ├───────────────────────────────────────────────────────────────────┤
  │  2  Rate Limiting                   [RateLimitPolicy: rl-policy]  │
  │     10 req/s per IP                                               │
  ├───────────────────────────────────────────────────────────────────┤
  │  3  Header Transform           [TrafficPolicy: transform-policy]  │
  │     add: x-request-id  │  remove: x-internal-token               │
  ├───────────────────────────────────────────────────────────────────┤
  │  4  ▶ Backend: my-api-svc:8080                       weight: 100% │
  └───────────────────────────────────────────────────────────────────┘
```

---

## Error Handling

| Situation | Behavior |
|---|---|
| Envoy port-forward fails | Fail fast with clear error message + hint (required RBAC, pod readiness) |
| HTTPRoute not attached to any Gateway | Display clear error, do not attempt resolution |
| Gateway not found or not ready | Fail fast with clear error |
| Policy defined in K8S but absent from Envoy config | Inline warning in TUI: `⚠ Policy defined but not found in Envoy config` |
| Multiple gateway-proxy pod replicas | Use first ready pod (all replicas carry identical config) |

---

## Dependencies

| Package | Purpose |
|---|---|
| `github.com/spf13/cobra` | CLI framework |
| `sigs.k8s.io/controller-runtime` | K8S client with Gateway API types |
| `sigs.k8s.io/gateway-api` | HTTPRoute, Gateway, policy types |
| `k8s.io/client-go` | Port-forward + kubeconfig handling |
| `github.com/charmbracelet/bubbletea` | TUI framework |
| `github.com/charmbracelet/lipgloss` | TUI styling |
| `github.com/charmbracelet/bubbles` | Pre-built TUI components (viewport, list) |
| `github.com/envoyproxy/go-control-plane` | Typed Envoy config structs for config_dump parsing |

---

## Future Considerations

- **REST API / server mode:** The `RouteGraph` struct is intentionally designed as a clean, serializable data model. Adding a `--serve` flag that exposes a JSON endpoint requires only a thin HTTP handler on top of the existing pipeline — no architectural changes needed.
- **Web UI:** The server mode above would feed directly into a Kgateway UI that renders the filter chain visually.
- **Rename:** `krp` is a working name; can be renamed without any design impact.
