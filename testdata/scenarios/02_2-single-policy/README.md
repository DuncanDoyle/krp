# Scenario 02_2: Single Policy — RequestHeaderModifier (K8S HTTPRoute filter)

HTTPS Gateway with one HTTPRoute that contains a RequestHeaderModifier filter.

This is the on the first scenarios where we can observe how kgateway translates a K8S policy into an Envoy filter, and what metadata or naming conventions link the two.

## What to observe

- How does the HTTPRoute Filter appear in the Envoy config? As an HTTP filter, a per-route config, or both?
- Is there any metadata in the Envoy config that references the originating K8S resource?
- How does the filter chain differ from the no-policy baseline (scenario 01)?


## K8S resources

- `k8s/httpbin.yaml` — Single rule routing to one backend service
- `k8s/gateway.yaml` — HTTP Gateway on port 80
- `k8s/api_developer-example-com-httproute.yaml` — Single rule routing to one backend service
- `k8s/httproute-kgateway-system_service-httpbin-rg.yaml` — Reference Grant granting access from the HTTPRoute in the kgateway-system namespace to Services in the httpbin namespace

## Envoy dump

- `envoy/config_dump.json` — Full output of `curl localhost:19000/config_dump`

## How to collect

```bash
# Apply the K8S resources
./testdata/scenarios/02_2-single-policy/k8s/setup.sh

# Wait for the HTTPBin application to be ready
kubectl wait --for=condition=Ready pod -l app=httpbin -n httpbin --timeout=120s

# Wait for the gateway pod to be ready
kubectl wait --for=condition=Ready pod -l gateway.networking.k8s.io/gateway-name=gw -n kgateway-system --timeout=60s

# Grab the Envoy config dump: port-forward admin port to localhost, wait for readiness, dump config, then kill the port-forward
kubectl port-forward -n kgateway-system deploy/gw 19000:19000 & sleep 2 && curl -s localhost:19000/config_dump | jq . > testdata/scenarios/02_2-single-policy/envoy/config_dump.json; kill %1

# Tear down the K8S resources
./testdata/scenarios/02_2-single-policy/k8s/teardown.sh
```
