# Scenario 02_4: Single Policy — RequestMirror (K8S HTTPRoute filter)

HTTP Gateway with one HTTPRoute that uses a `RequestMirror` filter to shadow traffic to a second backend.

Interesting from a filter chain perspective because mirroring is a cluster-level concern in Envoy, not a header manipulation — useful for seeing how kgateway translates a K8S filter into Envoy route config.

## What to observe

- How does request mirroring appear in the Envoy route config? Is it a `request_mirror_policies` entry on the route?
- Does a separate cluster get created for the mirror backend?
- Is there any HTTP filter involved, or is it purely route-level config?

## K8S resources

- `k8s/httpbin.yaml` — Primary HTTPBin backend (`httpbin` service)
- `k8s/httpbin-mirror.yaml` — Mirror HTTPBin backend (`httpbin-mirror` service)
- `k8s/gateway.yaml` — HTTP Gateway on port 80
- `k8s/api-example-com-httproute.yaml` — HTTPRoute with RequestMirror filter pointing at httpbin-mirror
- `k8s/httproute-kgateway-system_service-httpbin-rg.yaml` — ReferenceGrant from kgateway-system to httpbin namespace (covers both services)

## Envoy dump

- `envoy/config_dump.json` — Full output of `curl localhost:19000/config_dump`

## How to collect

```bash
# Apply the K8S resources
./testdata/scenarios/02_4-single-policy/k8s/setup.sh

# Wait for the HTTPBin applications to be ready
kubectl wait --for=condition=Ready pod -l app=httpbin -n httpbin --timeout=120s
kubectl wait --for=condition=Ready pod -l app=httpbin-mirror -n httpbin --timeout=120s

# Wait for the gateway pod to be ready
kubectl wait --for=condition=Ready pod -l gateway.networking.k8s.io/gateway-name=gw -n kgateway-system --timeout=60s

# Grab the Envoy config dump: port-forward admin port to localhost, wait for readiness, dump config, then kill the port-forward
kubectl port-forward -n kgateway-system deploy/gw 19000:19000 & sleep 2 && curl -s localhost:19000/config_dump | jq . > testdata/scenarios/02_4-single-policy/envoy/config_dump.json; kill %1

# Tear down the K8S resources
./testdata/scenarios/02_4-single-policy/k8s/teardown.sh
```
