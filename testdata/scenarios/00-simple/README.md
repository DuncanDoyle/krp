# Scenario 01: Simple

HTTP Gateway with a single listener, one HTTPRoute, one backend, no policies.

This is the baseline scenario — the minimum viable kgateway setup. Useful for understanding the default Envoy filter pipeline when no user-defined policies are attached.

## What to observe

- Which HTTP filters does kgateway inject by default (even with no policies)?
- What does the cluster name look like for the backend service?
- How is the HTTPRoute represented in the route config (VirtualHost name, domains, route match)?
- Is the route config inline in the HCM or referenced via RDS?

## K8S resources

- `k8s/httpbin.yaml` — Single rule routing to one backend service
- `k8s/gateway.yaml` — HTTP Gateway on port 80
- `k8s/api-example-com-httproute.yaml` — Single rule routing to one backend service
- `k8s/httproute-kgateway-system_service-httpbin-rg.yaml` — Reference Grant granting access from the HTTPRoute in the kgateway-system namespace to Services in the httpbin namespace

## Envoy dump

- `envoy/config_dump.json` — Full output of `curl localhost:19000/config_dump`

## How to collect

```bash
# Apply the K8S resources
./testdata/scenarios/00-simple/k8s/setup.sh

# Wait for the HTTPBin application to be ready
kubectl wait --for=condition=Ready pod -l app=httpbin -n httpbin --timeout=120s

# Wait for the gateway pod to be ready
kubectl wait --for=condition=Ready pod -l gateway.networking.k8s.io/gateway-name=gw -n kgateway-system --timeout=60s

# Grab the Envoy config dump: port-forward admin port to localhost, wait for readiness, dump config, then kill the port-forward
kubectl port-forward -n kgateway-system deploy/gw 19000:19000 & sleep 2 && curl -s localhost:19000/config_dump | jq . > testdata/scenarios/00-simple/envoy/config_dump.json; kill %1

# Tear down the K8S resources
./testdata/scenarios/00-simple/k8s/setup.sh
```