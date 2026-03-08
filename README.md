# kfp — Kgateway Filter Chain Printer

`kfp` is a CLI tool that visualizes the [Envoy](https://www.envoyproxy.io/) filter chain configuration for [Kgateway](https://github.com/kgateway-dev/kgateway) API Gateway deployments.

Kgateway translates Kubernetes [Gateway API](https://gateway-api.sigs.k8s.io/) resources (Gateway, HTTPRoute, TrafficPolicy, etc.) into Envoy configuration. Understanding the exact filter chain that processes your traffic — including filter ordering, applied policies, and backend routing — is difficult without digging through raw JSON config dumps. `kfp` makes this visible.

> **Status:** Active development — Phase 1 (Envoy Config Viewer) in progress.

---

## Features

**Phase 1 (in progress) — Envoy Config Viewer**
- Parse a live or file-based Envoy config dump
- Visualize the complete configuration: listeners, filter chains, HTTP filter pipeline, virtual hosts, routes, and backend clusters
- Auto port-forward to the gateway-proxy pod (no manual setup needed)

**Planned**
- Phase 2: Expand individual filters to see their typed configuration
- Phase 3: Select by HTTPRoute name to filter the view to a specific route
- Phase 4: Correlate Envoy filters back to the originating K8S Gateway API resources (Gateway, HTTPRoute, TrafficPolicy, EKTP)
- Phase 5: Interactive detail view with side-by-side Envoy config + K8S manifest

---

## Installation

```bash
go install github.com/kgateway-dev/kfp/cmd/kfp@latest
```

Or build from source:

```bash
git clone https://github.com/DuncanDoyle/kfp.git
cd kfp
go build -o kfp ./cmd/kfp
```

---

## Usage

### Visualize from a config dump file

```bash
kfp dump --file path/to/config_dump.json
```

### Visualize from a live cluster

```bash
# kfp automatically port-forwards to the gateway-proxy pod
kfp dump --gateway gw -n kgateway-system

# With an explicit kubeconfig context
kfp dump --gateway gw -n kgateway-system --context my-cluster
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

### Live cluster mode (`--gateway`)

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
cmd/kfp/          CLI entrypoint (cobra)
internal/
  model/          EnvoySnapshot data model
  parser/         Envoy config dump JSON parser
  envoy/          Admin API client + port-forwarder
  renderer/       Terminal UI (lipgloss)
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
| `03-multi-policy` | Multiple policies on one route |
| `04-multi-rule` | One HTTPRoute, multiple routing rules |
| `05-multi-listener` | Gateway with HTTP + HTTPS listeners |

---

## Contributing

This project is in early development. Contributions, bug reports, and feature suggestions are welcome via GitHub Issues.

---

## License

Apache 2.0
