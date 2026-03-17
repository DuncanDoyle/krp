# Scenario 04: Multiple Rules

One HTTPRoute with multiple rules targeting different backends (e.g., path-based routing: `/api` → api-svc, `/web` → web-svc).

This scenario tests whether the tool can correctly represent and correlate multiple rules within a single HTTPRoute, each with its own match conditions and backend refs.

## What to observe

- How are multiple rules represented in the Envoy route config? Separate route entries in the same VirtualHost?
- Do per-rule policies (if any) appear as per-route filter configs or as separate filter instances?
- What is the naming pattern for clusters when multiple backends are involved?
- How does the route match config (prefix, headers, etc.) map back to HTTPRoute rule matches?

## K8S resources

- `k8s/gateway.yaml` — HTTP or HTTPS Gateway
- `k8s/httproute.yaml` — Multiple rules with different matches and backends

## Envoy dump

- `envoy/config_dump.json` — Full output of `curl localhost:19000/config_dump`

## How to collect

```bash
kubectl apply -f testdata/scenarios/04-multi-rule/k8s/
kubectl wait --for=condition=Ready pod -l gateway.networking.k8s.io/gateway-name=multi-rule-gw -n default --timeout=60s
kubectl exec deploy/multi-rule-gw -n default -- curl -s localhost:19000/config_dump | jq . > testdata/scenarios/04-multi-rule/envoy/config_dump.json
```
