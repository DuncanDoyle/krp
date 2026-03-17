# krp — Kgateway Route Printer

`krp` is a CLI tool that visualizes the [Envoy](https://www.envoyproxy.io/) route configuration for [Kgateway](https://github.com/kgateway-dev/kgateway) API Gateway deployments.

Kgateway translates Kubernetes [Gateway API](https://gateway-api.sigs.k8s.io/) resources (Gateway, HTTPRoute, TrafficPolicy, etc.) into Envoy configuration. Understanding the exact filter chain that processes your traffic — including filter ordering, applied policies, and backend routing — is difficult without digging through raw JSON config dumps. `krp` makes this visible.

> **Status:** Active development — Phase 2 (HTTPRoute Selector) complete.

---

## Features

**Phase 1 (complete) — Envoy Config Viewer**
- Parse a live or file-based Envoy config dump
- Visualize the complete configuration: listeners, filter chains, HTTP filter pipeline, virtual hosts, routes, and backend clusters
- Auto port-forward to the gateway-proxy pod (no manual setup needed)

**Phase 2 (complete) — HTTPRoute Selector**
- Filter the rendered output to routes belonging to a specific HTTPRoute (`--route`, `--route-ns`)
- Optionally narrow to a single rule within the HTTPRoute (`--rule`)
- Port-forward to any Deployment pod via `--deployment` (alternative to `--gateway`)

**Planned**
- Phase 3: Expand individual filters to see their typed configuration
- Phase 4: Correlate Envoy filters back to the originating K8S Gateway API resources (Gateway, HTTPRoute, TrafficPolicy, EKTP)
- Phase 5: Interactive detail view with side-by-side Envoy config + K8S manifest

---

## Installation

> **Note:** The module is not yet published. Install by building from source.

Build from source:

```bash
git clone https://github.com/DuncanDoyle/krp.git
cd krp
go build -o krp ./cmd/krp
```

---

## Usage

### Visualize from a config dump file

```bash
krp dump --file path/to/config_dump.json
```

### Visualize from a live cluster

```bash
# krp automatically port-forwards to the gateway-proxy pod
krp dump --gateway gw -n kgateway-system

# Port-forward to any pod in a Deployment
krp dump --deployment gw -n kgateway-system

# With an explicit kubeconfig context
krp dump --gateway gw -n kgateway-system --context my-cluster
```

### Filter by HTTPRoute

```bash
# Show only routes for a specific HTTPRoute
krp dump --file config_dump.json --route my-route --route-ns default

# Narrow to a single rule within the HTTPRoute (zero-based index)
krp dump --file config_dump.json --route my-route --route-ns default --rule 0
```

### Collecting a config dump manually

```bash
kubectl port-forward -n kgateway-system deploy/gw 19000:19000 &
sleep 2
curl -s localhost:19000/config_dump | jq . > config_dump.json
kill %1
```

---

## Requirements

### Live cluster mode (`--gateway` / `--deployment`)

- A running Kubernetes cluster with Kgateway installed
- RBAC: `get pods` and `create pods/portforward` in the Gateway namespace
- Your kubeconfig context pointing at the target cluster

### File mode (`--file`)

No cluster access required — works fully offline.

---

## Example Output

```
╭─ Listener: listener~443 ─────────────────────────── [::]:443 ─╮
│                                                                 │
│  ├─ FilterChain[0] https-api  TLS: api.example.com             │
│  │  └─ HCM → RDS: https-api                                    │
│  │     └─ VirtualHost: https-api~api_example_com               │
│  │           [api.example.com]                                  │
│  │        └─ Route: / (prefix)                                  │
│  │           HTTP Filters:                                      │
│  │           ├─ [1] io.solo.transformation (disabled)           │
│  │           └─ [2] envoy.filters.http.router                   │
│  │           Backend: kube_httpbin_httpbin_8000                 │
│                                                                 │
│  └─ FilterChain[1] https-developer  TLS: developer.example.com │
│     └─ HCM → RDS: https-developer                              │
│        └─ VirtualHost: https-developer~developer_example_com   │
│              [developer.example.com]                            │
│           └─ Route: / (prefix)                                  │
│              HTTP Filters:                                      │
│              ├─ [1] io.solo.transformation (disabled)           │
│              └─ [2] envoy.filters.http.router                   │
│              Backend: kube_httpbin_httpbin_8000                 │
╰─────────────────────────────────────────────────────────────────╯
```

---

## Project Structure

```
cmd/krp/          CLI entrypoint (cobra)
internal/
  model/          EnvoySnapshot data model
  parser/         Envoy config dump JSON parser
  envoy/          Admin API client + port-forwarder
  renderer/       Terminal UI (lipgloss)
  filter/         HTTPRoute snapshot filter
docs/plans/       Design documents and implementation plans
testdata/
  scenarios/      Real K8S + Envoy config dump pairs for testing
```

---

## Testdata Scenarios

The `testdata/scenarios/` directory contains real Kubernetes manifests and Envoy config dumps collected from a live Kgateway cluster. These are used as parser test fixtures.

| Scenario | Description |
|---|---|
| `01-simple` | HTTP Gateway, one route, no policies |
| `02_1-single-policy` | HTTPS Gateway, transformation (EKTP) |
| `02_2-single-policy` | HTTP, RequestHeaderModifier (native K8S filter) |
| `02_3-single-policy` | HTTP, ResponseHeaderModifier |
| `02_4-single-policy` | HTTP, RequestMirror |
| `02_5-single-policy` | HTTP, URLRewrite |
| `02_6-single-policy` | HTTP, CORS (EKTP) |
| `02_7-single-policy` | HTTP, ExtAuth (EKTP) |
| `02_8-single-policy` | HTTP, RateLimit (EKTP) |

---

## Contributing

This project is in early development. Contributions, bug reports, and feature suggestions are welcome via GitHub Issues.

---

## License

Apache 2.0
