# Scenario 01_3 — RegularExpression Path Match

## What this tests

An HTTPRoute with a single rule using a **RegularExpression** path matcher (`/api/v[0-9]+/.*`). No filters are applied. Traffic is routed to the httpbin backend.

## What to observe in the Envoy config dump

Envoy represents regex path matches using `safe_regex_match` in the route entry. Unlike `prefix` or `path` (exact), a regex match is encoded as:

```json
"match": {
  "safe_regex": {
    "regex": "/api/v[0-9]+/.*"
  }
}
```

The `safe_regex` wrapper is Envoy's protection against regex expressions that could cause catastrophic backtracking (ReDoS). Any regex provided by the Gateway API is passed through into this structure. This is worth observing because it is the least-obvious translation from the Gateway API spec — `RegularExpression` in K8s becomes `safe_regex` in Envoy, not a plain `regex` field.

## Envoy dump

- `envoy/config_dump.json` — Full output of `curl localhost:19000/config_dump`

## How to collect

```bash
# Apply the K8S resources
./testdata/scenarios/01_3-matchers/k8s/setup.sh

# Wait for the HTTPBin application to be ready
kubectl wait --for=condition=Ready pod -l app=httpbin -n httpbin --timeout=120s

# Wait for the gateway pod to be ready
kubectl wait --for=condition=Ready pod -l gateway.networking.k8s.io/gateway-name=gw -n kgateway-system --timeout=60s

# Grab the Envoy config dump: port-forward admin port to localhost, wait for readiness, dump config, then kill the port-forward
kubectl port-forward -n kgateway-system deploy/gw 19000:19000 & sleep 2 && curl -s localhost:19000/config_dump | jq . > testdata/scenarios/01_3-matchers/envoy/config_dump.json; kill %1

# Tear down the K8S resources
./testdata/scenarios/01_3-matchers/k8s/setup.sh
```