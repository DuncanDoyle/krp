# Scenario 01_6 — Method Matcher

## What this tests

An HTTPRoute with a single rule using only a **Method** matcher for `GET`. No path matcher is specified (so it matches all paths). No filters. Traffic is routed to the httpbin backend.

## What to observe in the Envoy config dump

This is one of the more interesting translations to observe. The Gateway API has a first-class `method` field on HTTPRouteMatch, but HTTP methods are not a first-class match type in Envoy's route config. Instead, Envoy translates the method matcher into a **header match on the pseudo-header `:method`**:

```json
"match": {
  "prefix": "/",
  "headers": [
    {
      "name": ":method",
      "string_match": {
        "exact": "GET"
      }
    }
  ]
}
```

The `:method` header is an HTTP/2 pseudo-header that is also used by Envoy for HTTP/1.1 method matching internally. This translation is invisible to the end user at the Gateway API level but becomes visible when inspecting the Envoy route config — a good example of where the abstraction layers diverge.

## Envoy dump

- `envoy/config_dump.json` — Full output of `curl localhost:19000/config_dump`

## How to collect

```bash
# Apply the K8S resources
./testdata/scenarios/01_6-matchers/k8s/setup.sh

# Wait for the HTTPBin application to be ready
kubectl wait --for=condition=Ready pod -l app=httpbin -n httpbin --timeout=120s

# Wait for the gateway pod to be ready
kubectl wait --for=condition=Ready pod -l gateway.networking.k8s.io/gateway-name=gw -n kgateway-system --timeout=60s

# Grab the Envoy config dump: port-forward admin port to localhost, wait for readiness, dump config, then kill the port-forward
kubectl port-forward -n kgateway-system deploy/gw 19000:19000 & sleep 2 && curl -s localhost:19000/config_dump | jq . > testdata/scenarios/01_6-matchers/envoy/config_dump.json; kill %1

# Tear down the K8S resources
./testdata/scenarios/01_6-matchers/k8s/setup.sh
```