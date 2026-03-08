# Scenario 02_5: Single Policy — URLRewrite (K8S HTTPRoute filter)

HTTP Gateway with one HTTPRoute that uses a `URLRewrite` filter to rewrite the request path before forwarding to the backend.

## What to observe

- How does path rewriting appear in the Envoy route config? As a `prefix_rewrite` or `regex_rewrite` on the route?
- Is there any HTTP filter involved, or is it purely route-level config?
- How does the route match interact with the rewrite (prefix match required for prefix rewrite)?

## K8S resources

- `k8s/httpbin.yaml` — HTTPBin backend
- `k8s/gateway.yaml` — HTTP Gateway on port 80
- `k8s/api-example-com-httproute.yaml` — HTTPRoute with URLRewrite filter (path prefix rewrite)
- `k8s/httproute-kgateway-system_service-httpbin-rg.yaml` — ReferenceGrant from kgateway-system to httpbin namespace

## Envoy dump

- `envoy/config_dump.json` — Full output of `curl localhost:19000/config_dump`

## How to collect

```bash
# Apply the K8S resources
./testdata/scenarios/02_5-single-policy/k8s/setup.sh

# Wait for the HTTPBin application to be ready
kubectl wait --for=condition=Ready pod -l app=httpbin -n httpbin --timeout=120s

# Wait for the gateway pod to be ready
kubectl wait --for=condition=Ready pod -l gateway.networking.k8s.io/gateway-name=gw -n kgateway-system --timeout=60s

# Grab the Envoy config dump: port-forward admin port to localhost, wait for readiness, dump config, then kill the port-forward
kubectl port-forward -n kgateway-system deploy/gw 19000:19000 & sleep 2 && curl -s localhost:19000/config_dump | jq . > testdata/scenarios/02_5-single-policy/envoy/config_dump.json; kill %1

# Tear down the K8S resources
./testdata/scenarios/02_5-single-policy/k8s/teardown.sh
```
