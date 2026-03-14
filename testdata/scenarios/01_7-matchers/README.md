# Scenario 01_7 — Query Parameter Matcher (Exact Value)

## What this tests

An HTTPRoute with a single rule combining a **PathPrefix** matcher (`/search`) with an **Exact** query parameter matcher (`format=json`). No filters. Traffic is routed to the httpbin backend.

## What to observe in the Envoy config dump

Envoy has a dedicated `query_parameters` array in the route match config for query parameter matching. For an exact value match, look for:

```json
"match": {
  "prefix": "/search",
  "query_parameters": [
    {
      "name": "format",
      "string_match": {
        "exact": "json"
      }
    }
  ]
}
```

Query parameter matchers in Envoy are parallel in structure to header matchers — they sit alongside the path condition and are ANDed together. One thing to note is that query parameters are matched against the raw query string, so `format=json` in the URL must match exactly (including case) for the route to be selected.

## Envoy dump

- `envoy/config_dump.json` — Full output of `curl localhost:19000/config_dump`

## How to collect

```bash
# Apply the K8S resources
./testdata/scenarios/01_7-matchers/k8s/setup.sh

# Wait for the HTTPBin application to be ready
kubectl wait --for=condition=Ready pod -l app=httpbin -n httpbin --timeout=120s

# Wait for the gateway pod to be ready
kubectl wait --for=condition=Ready pod -l gateway.networking.k8s.io/gateway-name=gw -n kgateway-system --timeout=60s

# Grab the Envoy config dump: port-forward admin port to localhost, wait for readiness, dump config, then kill the port-forward
kubectl port-forward -n kgateway-system deploy/gw 19000:19000 & sleep 2 && curl -s localhost:19000/config_dump | jq . > testdata/scenarios/01_7-matchers/envoy/config_dump.json; kill %1

# Tear down the K8S resources
./testdata/scenarios/01_7-matchers/k8s/setup.sh
```