# Scenario 02_6: Single Policy — CORS (EnterpriseKgatewayTrafficPolicy)

HTTP Gateway with one HTTPRoute and a single `EnterpriseKgatewayTrafficPolicy` applying CORS configuration via `entCors`.

Contrast with an HTTPRoute-level CORS implementation to see where CORS ends up in the Envoy config when driven by an EKTP policy.

## What to observe

- Does CORS appear as an HTTP filter in the filter chain, as per-route typed config, or both?
- What is the Envoy filter name for CORS (`envoy.filters.http.cors`)?
- How are the CORS allow-origins, allow-methods, etc. encoded in the Envoy config?
- Is the CORS filter placed before or after other filters in the chain?

## K8S resources

- `k8s/httpbin.yaml` — HTTPBin backend
- `k8s/gateway.yaml` — HTTP Gateway on port 80
- `k8s/api-example-com-httproute.yaml` — Simple HTTPRoute with no filters (policy attached separately)
- `k8s/cors-ektp.yaml` — EnterpriseKgatewayTrafficPolicy with `entCors`
- `k8s/httproute-kgateway-system_service-httpbin-rg.yaml` — ReferenceGrant from kgateway-system to httpbin namespace

## Envoy dump

- `envoy/config_dump.json` — Full output of `curl localhost:19000/config_dump`

## How to collect

```bash
# Apply the K8S resources
./testdata/scenarios/02_6-single-policy/k8s/setup.sh

# Wait for the HTTPBin application to be ready
kubectl wait --for=condition=Ready pod -l app=httpbin -n httpbin --timeout=120s

# Wait for the gateway pod to be ready
kubectl wait --for=condition=Ready pod -l gateway.networking.k8s.io/gateway-name=gw -n kgateway-system --timeout=60s

# Grab the Envoy config dump: port-forward admin port to localhost, wait for readiness, dump config, then kill the port-forward
kubectl port-forward -n kgateway-system deploy/gw 19000:19000 & sleep 2 && curl -s localhost:19000/config_dump | jq . > testdata/scenarios/02_6-single-policy/envoy/config_dump.json; kill %1

# Tear down the K8S resources
./testdata/scenarios/02_6-single-policy/k8s/teardown.sh
```
