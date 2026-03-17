# Phase 2 — HTTPRoute Selector: Design Document

**Date:** 2026-03-17
**Status:** Approved

---

## Goal

Allow the user to pass `--route <name>` (plus optional `--rule <index>`) to `krp dump` to filter the rendered output to only the listeners, virtual hosts, and routes relevant to the selected HTTPRoute. No K8S API calls — filtering is done purely by matching the HTTPRoute identity embedded in Envoy route names.

---

## Two-Namespace Model

`krp dump` already accepts `-n / --namespace` for the **Gateway namespace** — this is used to locate the gateway-proxy pod for port-forwarding and is unrelated to where HTTPRoutes live. The Gateway API explicitly supports cross-namespace attachment: an HTTPRoute in `team-a` can attach to a Gateway in `kgateway-system`.

Because the two namespaces are independent, `--route` requires a dedicated `--route-ns` flag. Reusing `-n` for the HTTPRoute namespace would silently produce wrong results in any cross-namespace setup.

---

## Background: Envoy Route Naming Convention

Kgateway encodes the HTTPRoute identity in each Envoy route name:

```
<listener>-route-<N>-httproute-<httproute-name>-<namespace>-<rule_idx>-<backend_idx>-matcher-<matcher_idx>
```

Example (HTTPRoute `api-example-com` in namespace `default`, rule 0, matcher 0):
```
listener~80~api_example_com-route-0-httproute-api-example-com-default-0-0-matcher-0
```

The `httproute-<name>-<namespace>` segment is always present. The user supplies `--route <name>` and `--route-ns <ns>` (the HTTPRoute namespace, **not** the Gateway namespace). Together they form an unambiguous substring match: two HTTPRoutes with the same name in different namespaces produce different substrings (`httproute-foo-ns1-` vs `httproute-foo-ns2-`).

Routes that do not follow this convention (e.g. older kgateway versions, raw Envoy config) simply won't match — the filter returns an empty snapshot and the renderer shows nothing, which is a clear signal to the user.

---

## Architecture

Phase 2 adds a **filter layer** between parser and renderer:

```
CLI → [Envoy Parser] → [Route Filter] → [Renderer]
```

The filter takes a `*model.EnvoySnapshot` and a `FilterOptions` struct, and returns a new `*model.EnvoySnapshot` containing only the matching routes (and the listeners / virtual hosts that contain them). Empty virtual hosts and empty listeners are pruned.

### New package: `internal/filter`

```
internal/filter/
  filter.go       — Filter() function and FilterOptions type
  filter_test.go  — unit tests using synthetic model data
```

The filter package has no dependencies on the parser, renderer, or envoy packages. It takes and returns `*model.EnvoySnapshot`.

---

## FilterOptions

```go
type FilterOptions struct {
    HTTPRouteName      string // required: HTTPRoute name, e.g. "api-example-com"
    Namespace          string // required: namespace, e.g. "default"
    RuleIndex          int    // optional: -1 means all rules
}
```

`RuleIndex == -1` means no rule filter (match all rules for the HTTPRoute). This is the default when `--rule` is not passed.

---

## Matching Logic

A route matches when its `Name` field contains the substring:

```
httproute-<name>-<namespace>-
```

If `RuleIndex >= 0`, the route must also contain:

```
httproute-<name>-<namespace>-<rule_idx>-
```

Substring matching is intentionally simple — it avoids the need to fully parse the route name (which is complex because both name and namespace can contain dashes). The risk of a false positive match (e.g. httproute `foo-bar` namespace `baz` vs httproute `foo` namespace `bar-baz`) is accepted as a known limitation of the convention, not a bug in this tool.

---

## Snapshot Pruning

The filter walks the snapshot tree and produces a new `*model.EnvoySnapshot`:

1. For each `Listener` → for each `NetworkFilterChain` → for each `VirtualHost` in the HCM's `RouteConfig`:
   - Keep only routes that match.
   - If the virtual host has no remaining routes → drop it.
2. If a `NetworkFilterChain`'s `RouteConfig` has no remaining virtual hosts → the filter chain is kept (the HCM still exists) but its `RouteConfig.VirtualHosts` is empty.
3. If a `Listener` has no filter chains with any matching routes → the listener is dropped.

This gives the renderer a fully valid `*model.EnvoySnapshot` to work with — no nil checks needed in the renderer.

---

## CLI Changes

New flags on `krp dump`:

| Flag | Type | Description |
|------|------|-------------|
| `--route` | string | HTTPRoute name to filter by |
| `--route-ns` | string | Namespace of the HTTPRoute (required when `--route` is set) |
| `--rule` | int | Rule index within the HTTPRoute (default: -1, all rules) |

`--route` is optional. When absent, no filtering is applied and behaviour is identical to Phase 1.

`-n / --namespace` remains the **Gateway namespace** (used for port-forwarding) and is not involved in route filtering. `--route-ns` is a separate, required flag whenever `--route` is provided.

---

## Error Handling

| Condition | Behaviour |
|-----------|-----------|
| `--route` given, no routes match | Render empty snapshot; print hint to stderr: `"no routes found for HTTPRoute <ns>/<name> — check name, namespace, and that the HTTPRoute is attached to this Gateway"` |
| `--route` given without `--route-ns` | Return error: `"--route-ns is required when --route is set"` |
| `--route-ns` given without `--route` | Return error: `"--route-ns requires --route"` |
| `--rule` given without `--route` | Return error: `"--rule requires --route"` |
| `--rule` is negative (other than -1 sentinel) | Return error: `"--rule must be >= 0 (use -1 for all rules, which is the default)"` |

## Known Limitation: Dash Ambiguity

The kgateway route name convention concatenates name and namespace with dashes, but both name and namespace can themselves contain dashes. This means HTTPRoute `foo-bar` in namespace `baz` and HTTPRoute `foo` in namespace `bar-baz` both produce the substring `httproute-foo-bar-baz-` — they are indistinguishable without K8S API calls. This is an inherent limitation of the naming convention. In practice, Kubernetes name/namespace combinations that create this ambiguity are extremely rare.

---

## Non-goals (deferred to later phases)

- K8S API calls to validate the HTTPRoute exists.
- Interactive navigation to select an HTTPRoute from a list (Phase 3+).
- Resolving HTTPRoute name from a Deployment or Gateway name.

---

## Test Strategy

- Unit tests in `internal/filter/filter_test.go` with synthetic `*model.EnvoySnapshot` data covering:
  - Match by name + namespace
  - Match by name + namespace + rule index
  - No match → empty snapshot returned
  - Multiple listeners, multiple VHs, multiple routes — correct pruning
- E2E-style test: parse a real config dump, apply filter, check resulting snapshot.
- Renderer is unchanged — existing renderer tests remain valid.
