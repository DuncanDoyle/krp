# Scenario 01_4 — Header Matcher (Exact Value)

## What this tests

An HTTPRoute with a single rule combining a **PathPrefix** matcher (`/api`) with an **Exact** header matcher (`x-api-version: v1`). No filters are applied. Traffic is routed to the httpbin backend.

## What to observe in the Envoy config dump

Envoy encodes header matchers as a `headers` array alongside the path match condition in the route entry. For an Exact header match, look for:

```json
"match": {
  "prefix": "/api",
  "headers": [
    {
      "name": "x-api-version",
      "string_match": {
        "exact": "v1"
      }
    }
  ]
}
```

All conditions in the `match` block are ANDed together — a request must match both the path prefix and the header value to be routed. This makes header matchers a good way to demonstrate Envoy's multi-condition AND semantics in route matching.

## Envoy dump

- `envoy/config_dump.json` — Full output of `curl localhost:19000/config_dump`

## How to collect

```bash
# Apply the K8S resources
./testdata/scenarios/01_4-matchers/k8s/setup.sh

# Wait for the HTTPBin application to be ready
kubectl wait --for=condition=Ready pod -l app=httpbin -n httpbin --timeout=120s

# Wait for the gateway pod to be ready
kubectl wait --for=condition=Ready pod -l gateway.networking.k8s.io/gateway-name=gw -n kgateway-system --timeout=60s

# Grab the Envoy config dump: port-forward admin port to localhost, wait for readiness, dump config, then kill the port-forward
kubectl port-forward -n kgateway-system deploy/gw 19000:19000 & sleep 2 && curl -s localhost:19000/config_dump | jq . > testdata/scenarios/01_4-matchers/envoy/config_dump.json; kill %1

# Tear down the K8S resources
./testdata/scenarios/01_4-matchers/k8s/setup.sh
```