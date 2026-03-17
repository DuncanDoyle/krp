# Scenario 03: Multiple Policies

HTTPS Gateway with one HTTPRoute and multiple policies attached — e.g., auth (ext_authz or JWT), rate limiting, and a transformation.

This scenario tests the Correlator's ability to match multiple Envoy HTTP filters back to their respective K8S policy resources.

## What to observe

- Do all policies produce distinct HTTP filters, or do some share a filter?
- What is the ordering of filters in the HCM — does it match the order policies were applied?
- Can each filter be individually correlated back to its K8S policy?
- Are there any per-route filter configs vs listener-level filter configs?

## K8S resources

- `k8s/gateway.yaml` — HTTPS Gateway
- `k8s/httproute.yaml` — Single rule with one backend
- `k8s/route-option.yaml` — RouteOption with transformation
- `k8s/virtual-host-option.yaml` — VirtualHostOption with rate limiting or auth

## Envoy dump

- `envoy/config_dump.json` — Full output of `curl localhost:19000/config_dump`

## How to collect

```bash
kubectl apply -f testdata/scenarios/03-multi-policy/k8s/
kubectl wait --for=condition=Ready pod -l gateway.networking.k8s.io/gateway-name=multi-policy-gw -n default --timeout=60s
kubectl exec deploy/multi-policy-gw -n default -- curl -s localhost:19000/config_dump | jq . > testdata/scenarios/03-multi-policy/envoy/config_dump.json
```
