# Scenario 02_3: Single Policy — ResponseHeaderModifier (K8S HTTPRoute filter)

HTTP Gateway with one HTTPRoute that contains a `ResponseHeaderModifier` filter.

Companion to 02_2 (RequestHeaderModifier), but on the response path. Useful for comparing where request vs response header manipulation lands in the Envoy config.

## What to observe

- Does response header modification appear in the same place as request header modification (per-route config), or somewhere else?
- Is there any Envoy filter involved, or is it purely route-level config?
- How does the filter chain compare to the no-policy baseline (scenario 01)?

## K8S resources

- `k8s/httpbin.yaml` — HTTPBin backend
- `k8s/gateway.yaml` — HTTP Gateway on port 80
- `k8s/api-example-com-httproute.yaml` — HTTPRoute with ResponseHeaderModifier filter
- `k8s/httproute-kgateway-system_service-httpbin-rg.yaml` — ReferenceGrant from kgateway-system to httpbin namespace

## Envoy dump

- `envoy/config_dump.json` — Full output of `curl localhost:19000/config_dump`

## How to collect

```bash
# Apply the K8S resources
./testdata/scenarios/02_3-single-policy/k8s/setup.sh

# Wait for the HTTPBin application to be ready
kubectl wait --for=condition=Ready pod -l app=httpbin -n httpbin --timeout=120s

# Wait for the gateway pod to be ready
kubectl wait --for=condition=Ready pod -l gateway.networking.k8s.io/gateway-name=gw -n kgateway-system --timeout=60s

# Grab the Envoy config dump: port-forward admin port to localhost, wait for readiness, dump config, then kill the port-forward
kubectl port-forward -n kgateway-system deploy/gw 19000:19000 & sleep 2 && curl -s localhost:19000/config_dump | jq . > testdata/scenarios/02_3-single-policy/envoy/config_dump.json; kill %1

# Tear down the K8S resources
./testdata/scenarios/02_3-single-policy/k8s/teardown.sh
```
