# krp Design Review — Improvement Plan

**Date:** 2026-03-07
**Reviewing:** `2026-03-06-krp-design.md` and `2026-03-06-krp-implementation.md`

---

## High Priority

### 1. Correlator Is Based on Fictional Metadata Format

**Problem:** The design assumes kgateway embeds `io.solo.kgateway` metadata with a structured `policy_ref` in the Envoy route metadata. This is not how kgateway actually works. The test fixture in Task 5 is hand-crafted around this assumption, so the "primary" correlation layer (Metadata) won't work on a real cluster.

The `policyKindToFilterName` mapping is also a guess — e.g., `TrafficPolicy` → `header_to_metadata` is incorrect since TrafficPolicy can map to multiple Envoy filters.

**Action:**
- [ ] Dump a real kgateway Envoy config (`kubectl exec <pod> -- curl localhost:19000/config_dump`) and study the actual structure
- [ ] Identify what metadata, naming conventions, or structural patterns kgateway actually uses
- [ ] Redesign the Correlator around observed reality
- [ ] Rebuild the test fixture from a real (sanitized) config dump
- [ ] Likely outcome: structural matching (VirtualHost domains, cluster names) becomes the primary strategy, not metadata

### 2. VirtualHosts Are Not Inline in the Listener Config Dump

**Problem:** The test fixture embeds `virtual_hosts` directly inside the HCM `typed_config`. In a real Envoy config dump, route configuration is in a separate `RoutesConfigDump` section, referenced by the HCM's `rds.route_config_name`. The Correlator as written will fail to find any virtual hosts on a real cluster.

**Action:**
- [ ] Parse `RoutesConfigDump` as a separate config section from the dump
- [ ] Correlate the HCM to its route config via `rds.route_config_name`
- [ ] Update the Correlator's JSON parsing structs and logic accordingly
- [ ] Update the test fixture to reflect this two-part structure

### 3. No Policy Discovery in the Resolver

**Problem:** The Resolver walks HTTPRoute → Gateway → backends but never discovers any attached policies. Kgateway policies (`RouteOption`, `VirtualHostOption`, `ListenerOption`) are attached via `targetRef` or `extensionRef`. Without discovering these on the K8S side, the Correlator has no K8S policy data to merge — `PolicyRef` on `FilterNode` can only ever come from Envoy-side matching.

**Action:**
- [ ] Extend the Resolver to check `spec.rules[].filters[].extensionRef` on the HTTPRoute
- [ ] List `RouteOption` / `VirtualHostOption` / `ListenerOption` resources that target the HTTPRoute or Gateway via `targetRef`
- [ ] Pass discovered policies into the `RouteGraph` (new field or structure) so the Correlator can do a proper K8S ↔ Envoy merge
- [ ] Add Gateway API policy types to the scheme registration

---

## Medium Priority

### 4. Single Listener Assumption

**Problem:** The Resolver always picks the first listener on the Gateway (`primaryListener`). A Gateway commonly has both HTTP (port 80) and HTTPS (port 443) listeners. The HTTPRoute's `parentRef` can specify a `sectionName` to target a specific listener. The current code will silently pick the wrong listener.

**Action:**
- [ ] Match the listener using `parentRef.sectionName` from the HTTPRoute when present
- [ ] If `sectionName` is not specified, match by protocol/port compatibility
- [ ] Fail with a clear error if no matching listener is found

### 5. Multiple Rules Not Properly Modeled

**Problem:** `RouteGraph` has `Rule: int` and the CLI has `--rule`, but the model only holds a single `ListenerNode` with a single `FilterChain`. When `rule = -1` (all rules), there's no way to represent multiple rules that have different backends and potentially different per-route filters.

**Action:** Choose one of:
- [ ] **Option A:** Make `RouteGraph` hold multiple filter chains (one per rule) — a `Rules []RuleNode` field where each has its own filters and backends
- [ ] **Option B:** Default to requiring `--rule` and remove the `-1 = all` ambiguity; add a `krp routes <name>` summary command that lists rules without detail

Option A is more useful long-term. Option B is simpler to ship first.

### 6. Filter Chain vs HTTP Filter Pipeline Terminology

**Problem:** The design conflates two Envoy concepts: "filter chains" (network-level, selected by SNI/ALPN) and "HTTP filters" (inside the HCM — jwt_authn, ext_authz, etc.). The TUI says "FILTER CHAIN" when it means "HTTP filter pipeline." This will confuse users familiar with Envoy. Additionally, a real HTTPS listener may have multiple filter chains for different SNI hosts.

**Action:**
- [ ] Rename "FILTER CHAIN" in the TUI to "HTTP FILTER PIPELINE" or similar
- [ ] Handle the case where the listener has multiple network-level filter chains (pick the right one based on SNI/hostname matching)
- [ ] Update the design doc terminology to be precise

### 7. No `--json` Output Flag

**Problem:** For a debugging tool, piping output to `jq` or other tools is essential. The `RouteGraph` is already JSON-serializable but there's no CLI flag to get JSON output.

**Action:**
- [ ] Add `--output json` (or `--json`) flag that bypasses the TUI and prints the `RouteGraph` as indented JSON to stdout
- [ ] This is trivial to implement since `RouteGraph` already has JSON tags

---

## Low Priority

### 8. Single Gateway Assumption

**Problem:** The Resolver takes only the first accepted Gateway from `status.parents`. An HTTPRoute can be attached to multiple Gateways simultaneously.

**Action:**
- [ ] Add a `--gateway` flag to let the user specify which Gateway to inspect
- [ ] If not specified and multiple Gateways accept the route, list them and prompt the user to pick one (or error with a helpful message)

### 9. TUI Interactivity Is Incomplete

**Problem:** The renderer has a bubbletea `Model` with `Init`/`Update`/`View` but `View()` just calls `RenderSummary()` and ignores `selected` and `expanded` state. Task 6 delivers a static renderer wrapped in an interactive shell that doesn't actually do anything interactive.

**Action:**
- [ ] Consider shipping static output first (works in pipes, CI, non-TTY environments)
- [ ] Add real TUI interactivity as a follow-up task — selection highlighting, expand/collapse, side-by-side Envoy config + K8S manifest view
- [ ] The `--json` flag (item 7) provides immediate non-interactive utility

### 10. `go-control-plane` Listed but Not Used

**Problem:** The dependency table lists `go-control-plane` for Envoy proto types, but the Correlator defines its own raw JSON structs and never imports it.

**Action:**
- [ ] Remove `go-control-plane` from the dependency list and `go get` commands
- [ ] The raw JSON approach is simpler for v1 and avoids a heavy transitive dependency tree
- [ ] Revisit if proto-level parsing becomes necessary later (e.g., for typed config extraction in verbose mode)

### 11. Bug in `acceptedGateway` Function

**Problem:** Line 513-514 in the implementation plan checks `cond.Type == string(metav1.StatusReasonMethodNotAllowed)` which evaluates to `"MethodNotAllowed"` — a Kubernetes API status reason, not a Gateway API condition type. This is dead code that never matches anything.

**Action:**
- [ ] Remove the `MethodNotAllowed` check entirely
- [ ] The function should simply iterate conditions looking for `Type: "Accepted"` with `Status: "True"`

---

## Suggested Order of Work

1. **Dump a real config** (prerequisite for everything — items 1, 2)
2. **Fix the data model** (items 5, 6 — get the structure right before building on it)
3. **Fix the Resolver** (items 3, 4, 11 — correct K8S-side data collection)
4. **Redesign the Correlator** (items 1, 2 — based on real config dump analysis)
5. **Add `--json` output** (item 7 — immediate utility)
6. **Ship static output** (item 9 — defer TUI interactivity)
7. **Multi-gateway support** (item 8 — nice to have)
8. **Clean up deps** (item 10)
