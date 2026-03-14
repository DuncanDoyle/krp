# Scenario 01_2 — Exact Path Match

## What this tests

An HTTPRoute with a single rule using an **Exact** path matcher (`/api/v1/users`). No filters are applied. Traffic is routed to the httpbin backend.

## What to observe in the Envoy config dump

Envoy represents an Exact path match differently from a prefix match. In the route config, look for a route entry using `path` (exact string comparison) rather than `prefix` or `safe_regex`. The route entry should look like:

```json
"match": {
  "path": "/api/v1/users"
}
```

This is in contrast to a PathPrefix match, which uses `"prefix": "/api/v1"`, or a regex match which uses `"safe_regex"`. Observing the difference in these three scenarios (01_1, 01_2, 01_3) makes the Gateway API-to-Envoy path type translation explicit.

## Envoy dump

- `envoy/config_dump.json` — Full output of `curl localhost:19000/config_dump`

## How to collect

```bash
# Apply the K8S resources
./testdata/scenarios/01_2-matchers/k8s/setup.sh

# Wait for the HTTPBin application to be ready
kubectl wait --for=condition=Ready pod -l app=httpbin -n httpbin --timeout=120s

# Wait for the gateway pod to be ready
kubectl wait --for=condition=Ready pod -l gateway.networking.k8s.io/gateway-name=gw -n kgateway-system --timeout=60s

# Grab the Envoy config dump: port-forward admin port to localhost, wait for readiness, dump config, then kill the port-forward
kubectl port-forward -n kgateway-system deploy/gw 19000:19000 & sleep 2 && curl -s localhost:19000/config_dump | jq . > testdata/scenarios/01_2-matchers/envoy/config_dump.json; kill %1

# Tear down the K8S resources
./testdata/scenarios/01_2-matchers/k8s/setup.sh
```