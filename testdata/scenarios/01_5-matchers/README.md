# Scenario 01_5 — Header Matcher (RegularExpression Value)

## What this tests

An HTTPRoute with a single rule combining a **PathPrefix** matcher (`/api`) with a **RegularExpression** header matcher (`x-client-id` matching `[a-z]+-[0-9]+`). No filters are applied. Traffic is routed to the httpbin backend.

## What to observe in the Envoy config dump

For a regex header match, Envoy uses `safe_regex` inside the header matcher, just as it does for regex path matches. Look for:

```json
"match": {
  "prefix": "/api",
  "headers": [
    {
      "name": "x-client-id",
      "string_match": {
        "safe_regex": {
          "regex": "[a-z]+-[0-9]+"
        }
      }
    }
  ]
}
```

The key observation is consistency: Envoy always uses `safe_regex` for regex matching, whether the match target is a path, a header, or a query parameter. This wrapping is a Envoy-level safety mechanism, not something visible in the Gateway API spec itself.

## Envoy dump

- `envoy/config_dump.json` — Full output of `curl localhost:19000/config_dump`

## How to collect

```bash
# Apply the K8S resources
./testdata/scenarios/01_5-matchers/k8s/setup.sh

# Wait for the HTTPBin application to be ready
kubectl wait --for=condition=Ready pod -l app=httpbin -n httpbin --timeout=120s

# Wait for the gateway pod to be ready
kubectl wait --for=condition=Ready pod -l gateway.networking.k8s.io/gateway-name=gw -n kgateway-system --timeout=60s

# Grab the Envoy config dump: port-forward admin port to localhost, wait for readiness, dump config, then kill the port-forward
kubectl port-forward -n kgateway-system deploy/gw 19000:19000 & sleep 2 && curl -s localhost:19000/config_dump | jq . > testdata/scenarios/01_5-matchers/envoy/config_dump.json; kill %1

# Tear down the K8S resources
./testdata/scenarios/01_5-matchers/k8s/setup.sh
```