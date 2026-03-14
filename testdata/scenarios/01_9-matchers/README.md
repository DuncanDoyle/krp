# Scenario 01_9 — Combination: Exact Path + Method + Query Parameter

## What this tests

An HTTPRoute with a single rule combining three match types: an **Exact** path matcher (`/api/v1/search`), a **Method** matcher (`GET`), and an **Exact** query parameter matcher (`q=hello`). No filters. Traffic is routed to the httpbin backend.

This is the most complex matcher scenario in the 01_x series and is designed to show how Envoy ANDs together all three condition types in a single route entry.

## What to observe in the Envoy config dump

The resulting Envoy route entry pulls together all three translation patterns covered by earlier scenarios: exact path as `path`, method as a `:method` header match, and a query parameter matcher. Look for:

```json
"match": {
  "path": "/api/v1/search",
  "headers": [
    {
      "name": ":method",
      "string_match": {
        "exact": "GET"
      }
    }
  ],
  "query_parameters": [
    {
      "name": "q",
      "string_match": {
        "exact": "hello"
      }
    }
  ]
}
```

This is the most restrictive route in the series — a request must have exactly the right path, the right method, and the right query parameter to match. Comparing this route entry to 01_1 (prefix only, no additional conditions) shows clearly how each Gateway API match type is appended as an additional AND condition in Envoy's match block. The method-as-header translation (`:method`) is particularly notable since it is not obvious from the Gateway API spec that methods end up in the `headers` array rather than a dedicated `method` field.

## Envoy dump

- `envoy/config_dump.json` — Full output of `curl localhost:19000/config_dump`

## How to collect

```bash
# Apply the K8S resources
./testdata/scenarios/01_9-matchers/k8s/setup.sh

# Wait for the HTTPBin application to be ready
kubectl wait --for=condition=Ready pod -l app=httpbin -n httpbin --timeout=120s

# Wait for the gateway pod to be ready
kubectl wait --for=condition=Ready pod -l gateway.networking.k8s.io/gateway-name=gw -n kgateway-system --timeout=60s

# Grab the Envoy config dump: port-forward admin port to localhost, wait for readiness, dump config, then kill the port-forward
kubectl port-forward -n kgateway-system deploy/gw 19000:19000 & sleep 2 && curl -s localhost:19000/config_dump | jq . > testdata/scenarios/01_9-matchers/envoy/config_dump.json; kill %1

# Tear down the K8S resources
./testdata/scenarios/01_9-matchers/k8s/setup.sh
```