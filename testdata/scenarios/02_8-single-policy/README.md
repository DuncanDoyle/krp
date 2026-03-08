# Scenario 02_8: Single Policy — RateLimit (EnterpriseKgatewayTrafficPolicy)

HTTP Gateway with one HTTPRoute and a single `EnterpriseKgatewayTrafficPolicy` applying rate limiting via `entRateLimit`.

## What to observe

- How does the `envoy.filters.http.ratelimit` filter appear in the filter chain?
- What cluster is created for the rate limit service?
- How are the rate limit descriptors/actions encoded in the per-route config?
- Is there a separate rate limit service cluster in the Envoy cluster config?

## K8S resources

- `k8s/httpbin.yaml` — HTTPBin backend
- `k8s/gateway.yaml` — HTTP Gateway on port 80
- `k8s/api-example-com-httproute.yaml` — Simple HTTPRoute with no filters
- `k8s/rate-limit-ektp.yaml` — EnterpriseKgatewayTrafficPolicy with `entRateLimit`
- `k8s/httproute-kgateway-system_service-httpbin-rg.yaml` — ReferenceGrant from kgateway-system to httpbin namespace

## Envoy dump

- `envoy/config_dump.json` — Full output of `curl localhost:19000/config_dump`

## How to collect

```bash
# Apply the K8S resources
./testdata/scenarios/02_8-single-policy/k8s/setup.sh

# Wait for the HTTPBin application to be ready
kubectl wait --for=condition=Ready pod -l app=httpbin -n httpbin --timeout=120s

# Wait for the gateway pod to be ready
kubectl wait --for=condition=Ready pod -l gateway.networking.k8s.io/gateway-name=gw -n kgateway-system --timeout=60s

# Grab the Envoy config dump: port-forward admin port to localhost, wait for readiness, dump config, then kill the port-forward
kubectl port-forward -n kgateway-system deploy/gw 19000:19000 & sleep 2 && curl -s localhost:19000/config_dump | jq . > testdata/scenarios/02_8-single-policy/envoy/config_dump.json; kill %1

# Tear down the K8S resources
./testdata/scenarios/02_8-single-policy/k8s/teardown.sh
```
