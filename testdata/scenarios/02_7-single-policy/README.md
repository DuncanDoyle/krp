# Scenario 02_7: Single Policy — ExtAuth (EnterpriseKgatewayTrafficPolicy)

HTTP Gateway with one HTTPRoute and a single `EnterpriseKgatewayTrafficPolicy` enabling external authorization via `entExtAuth`.

ExtAuth is one of the most complex single-policy cases because it requires an out-of-process gRPC auth server. This scenario uses a minimal AuthConfig with basic auth (no external server required) to observe how kgateway configures the Envoy ext_authz filter.

## What to observe

- How does the `envoy.filters.http.ext_authz` filter appear in the filter chain?
- What cluster is created for the ext auth service?
- Is there per-route config that enables/disables ext authz?
- Does the AuthConfig reference appear anywhere in the Envoy metadata?

## K8S resources

- `k8s/httpbin.yaml` — HTTPBin backend
- `k8s/gateway.yaml` — HTTP Gateway on port 80
- `k8s/api-example-com-httproute.yaml` — Simple HTTPRoute with no filters
- `k8s/auth-config.yaml` — AuthConfig defining the ext auth configuration (basic auth)
- `k8s/ext-auth-ektp.yaml` — EnterpriseKgatewayTrafficPolicy referencing the AuthConfig
- `k8s/httproute-kgateway-system_service-httpbin-rg.yaml` — ReferenceGrant from kgateway-system to httpbin namespace

## Envoy dump

- `envoy/config_dump.json` — Full output of `curl localhost:19000/config_dump`

## How to collect

```bash
# Apply the K8S resources
./testdata/scenarios/02_7-single-policy/k8s/setup.sh

# Wait for the HTTPBin application to be ready
kubectl wait --for=condition=Ready pod -l app=httpbin -n httpbin --timeout=120s

# Wait for the gateway pod to be ready
kubectl wait --for=condition=Ready pod -l gateway.networking.k8s.io/gateway-name=gw -n kgateway-system --timeout=60s

# Grab the Envoy config dump: port-forward admin port to localhost, wait for readiness, dump config, then kill the port-forward
kubectl port-forward -n kgateway-system deploy/gw 19000:19000 & sleep 2 && curl -s localhost:19000/config_dump | jq . > testdata/scenarios/02_7-single-policy/envoy/config_dump.json; kill %1

# Tear down the K8S resources
./testdata/scenarios/02_7-single-policy/k8s/teardown.sh
```
