# Phase 2 — HTTPRoute Selector: Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `--route <name>` (and optional `--rule <index>`) to `krp dump` to filter the rendered snapshot to only routes belonging to the specified HTTPRoute.

**Architecture:** A new `internal/filter` package provides a pure `Filter(snapshot, opts)` function that returns a pruned `*model.EnvoySnapshot` by substring-matching the HTTPRoute identity embedded in Envoy route names. The CLI calls it between `parser.Parse` and `renderer.Render` when `--route` is set.

> **Note:** The CLI flags were renamed after initial implementation: `--httproute` → `--route`, `--httproute-namespace` → `--route-ns`. Code snippets below reflect the original names used during development.

**Tech Stack:** Go stdlib only (no new dependencies). Existing lipgloss renderer is unchanged.

---

## File Map

| Action | Path | Responsibility |
|--------|------|----------------|
| Create | `internal/filter/filter.go` | `FilterOptions` type and `Filter()` function |
| Create | `internal/filter/filter_test.go` | Unit tests for filter logic (synthetic model data) |
| Create | `internal/filter/e2e_test.go` | E2E tests against real config dump fixtures |
| Modify | `cmd/kfp/main.go` | Add `--httproute` / `--rule` flags; wire filter into pipeline |

Model, parser, renderer, and envoy packages are **not modified**.

---

## Task 1: Create `internal/filter/filter.go` with `FilterOptions` type

**Files:**
- Create: `internal/filter/filter.go`

- [ ] **Step 1.1: Write the file with the `FilterOptions` type and a stub `Filter` function that returns the snapshot unchanged**

```go
// Package filter provides route-level filtering of an [model.EnvoySnapshot]
// by HTTPRoute identity, as encoded in Envoy route names by kgateway.
package filter

import "github.com/DuncanDoyle/krp/internal/model"

// FilterOptions controls how [Filter] selects routes from a snapshot.
type FilterOptions struct {
	HTTPRouteName      string // HTTPRoute name, e.g. "api-example-com"
	HTTPRouteNamespace string // HTTPRoute namespace — NOT the Gateway namespace
	RuleIndex          int    // rule index within the HTTPRoute; -1 means all rules
}

// Filter returns a new [model.EnvoySnapshot] containing only the routes
// whose Envoy route name embeds the given HTTPRoute identity.
// Listeners and virtual hosts with no remaining routes are pruned.
// If opts.HTTPRouteName is empty, the snapshot is returned unchanged.
func Filter(snapshot *model.EnvoySnapshot, opts FilterOptions) *model.EnvoySnapshot {
	return snapshot
}
```

- [ ] **Step 1.2: Build to confirm no compile errors**

```bash
go build ./...
```

Expected: no output (clean build).

- [ ] **Step 1.3: Commit the stub**

```bash
git add internal/filter/filter.go
git commit -m "feat: add filter package stub with FilterOptions type"
```

---

## Task 2: Implement route-matching helper

**Files:**
- Modify: `internal/filter/filter.go`
- Create: `internal/filter/filter_test.go`

The matching logic: a route matches when `route.Name` contains the substring `httproute-<name>-<namespace>-`. When `RuleIndex >= 0`, it must also contain `httproute-<name>-<namespace>-<rule_idx>-`.

- [ ] **Step 2.1: Write the failing tests**

Create `internal/filter/filter_test.go` (note: `routeName` helper is defined here and shared with `e2e_test.go` since both are `package filter_test` in the same directory):

```go
package filter_test

import (
	"fmt"
	"testing"

	"github.com/DuncanDoyle/krp/internal/filter"
	"github.com/DuncanDoyle/krp/internal/model"
)

// routeName builds a realistic Envoy route name for test cases.
// Format: listener~80~example-route-0-httproute-<name>-<ns>-<rule>-0-matcher-0
func routeName(name, ns string, rule int) string {
	return fmt.Sprintf("listener~80~example-route-0-httproute-%s-%s-%d-0-matcher-0", name, ns, rule)
}

func TestFilter_NoOptions_ReturnsUnchanged(t *testing.T) {
	snap := snapshotWithRoute("listener~80~x-route-0-httproute-foo-default-0-0-matcher-0")
	got := filter.Filter(snap, filter.FilterOptions{})
	if len(got.Listeners) != 1 {
		t.Fatalf("expected 1 listener, got %d", len(got.Listeners))
	}
}

func TestFilter_ByHTTPRoute_MatchingRoute(t *testing.T) {
	snap := snapshotWithRoute(routeName("api-example-com", "default", 0))
	opts := filter.FilterOptions{HTTPRouteName: "api-example-com", HTTPRouteNamespace: "default", RuleIndex: -1}
	got := filter.Filter(snap, opts)
	if len(got.Listeners) != 1 {
		t.Fatalf("expected 1 listener, got %d", len(got.Listeners))
	}
	routes := got.Listeners[0].FilterChains[0].HCM.RouteConfig.VirtualHosts[0].Routes
	if len(routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(routes))
	}
}

func TestFilter_ByHTTPRoute_NoMatch_EmptySnapshot(t *testing.T) {
	snap := snapshotWithRoute(routeName("api-example-com", "default", 0))
	opts := filter.FilterOptions{HTTPRouteName: "other-route", HTTPRouteNamespace: "default", RuleIndex: -1}
	got := filter.Filter(snap, opts)
	if len(got.Listeners) != 0 {
		t.Fatalf("expected 0 listeners, got %d", len(got.Listeners))
	}
}

func TestFilter_ByRule_MatchingRule(t *testing.T) {
	// Two routes from same HTTPRoute, different rules
	snap := snapshotWithRoutes([]string{
		routeName("api-example-com", "default", 0),
		routeName("api-example-com", "default", 1),
	})
	opts := filter.FilterOptions{HTTPRouteName: "api-example-com", HTTPRouteNamespace: "default", RuleIndex: 1}
	got := filter.Filter(snap, opts)
	routes := got.Listeners[0].FilterChains[0].HCM.RouteConfig.VirtualHosts[0].Routes
	if len(routes) != 1 {
		t.Fatalf("expected 1 route (rule 1 only), got %d", len(routes))
	}
	if routes[0].Name != routeName("api-example-com", "default", 1) {
		t.Errorf("wrong route selected: %s", routes[0].Name)
	}
}

func TestFilter_PrunesEmptyVirtualHost(t *testing.T) {
	// Two VHs in the same listener: only one has a matching route
	snap := snapshotWithTwoVHs(
		routeName("api-example-com", "default", 0),             // VH1: will match
		"listener~80~other-route-0-httproute-other-default-0-0-matcher-0", // VH2: won't match
	)
	opts := filter.FilterOptions{HTTPRouteName: "api-example-com", HTTPRouteNamespace: "default", RuleIndex: -1}
	got := filter.Filter(snap, opts)
	vhs := got.Listeners[0].FilterChains[0].HCM.RouteConfig.VirtualHosts
	if len(vhs) != 1 {
		t.Fatalf("expected 1 VH (non-matching pruned), got %d", len(vhs))
	}
}

func TestFilter_PrunesEmptyListener(t *testing.T) {
	// Two listeners: only one has a matching route
	snap := snapshotWithTwoListeners(
		routeName("api-example-com", "default", 0),                // L1: will match
		"listener~443~other-route-0-httproute-x-y-0-0-matcher-0", // L2: won't match
	)
	opts := filter.FilterOptions{HTTPRouteName: "api-example-com", HTTPRouteNamespace: "default", RuleIndex: -1}
	got := filter.Filter(snap, opts)
	if len(got.Listeners) != 1 {
		t.Fatalf("expected 1 listener (non-matching pruned), got %d", len(got.Listeners))
	}
}

func TestFilter_MultipleFilterChains_PartialMatch(t *testing.T) {
	// One listener with two filter chains: FC1 has a matching route, FC2 does not.
	// Per design doc rule 2: listener is kept; FC2 is kept with empty VirtualHosts.
	matchingRoute := routeName("api-example-com", "default", 0)
	nonMatchingRoute := "listener~80~other-route-0-httproute-other-default-0-0-matcher-0"
	snap := &model.EnvoySnapshot{
		Listeners: []model.Listener{
			{Name: "listener~80", FilterChains: []model.NetworkFilterChain{
				{Name: "fc1", HCM: &model.HCMConfig{RouteConfig: &model.RouteConfig{
					VirtualHosts: []model.VirtualHost{{Name: "vh1", Routes: []model.Route{{Name: matchingRoute}}}},
				}}},
				{Name: "fc2", HCM: &model.HCMConfig{RouteConfig: &model.RouteConfig{
					VirtualHosts: []model.VirtualHost{{Name: "vh2", Routes: []model.Route{{Name: nonMatchingRoute}}}},
				}}},
			}},
		},
	}
	opts := filter.FilterOptions{HTTPRouteName: "api-example-com", HTTPRouteNamespace: "default", RuleIndex: -1}
	got := filter.Filter(snap, opts)

	// Listener kept (FC1 has a match)
	if len(got.Listeners) != 1 {
		t.Fatalf("expected 1 listener, got %d", len(got.Listeners))
	}
	// Both filter chains present
	if len(got.Listeners[0].FilterChains) != 2 {
		t.Fatalf("expected 2 filter chains, got %d", len(got.Listeners[0].FilterChains))
	}
	// FC1 has its VH; FC2 has empty VHs (pruned)
	fc1VHs := got.Listeners[0].FilterChains[0].HCM.RouteConfig.VirtualHosts
	fc2VHs := got.Listeners[0].FilterChains[1].HCM.RouteConfig.VirtualHosts
	if len(fc1VHs) != 1 {
		t.Errorf("FC1: expected 1 VH, got %d", len(fc1VHs))
	}
	if len(fc2VHs) != 0 {
		t.Errorf("FC2: expected 0 VHs (pruned), got %d", len(fc2VHs))
	}
}

// --- helpers ---

func snapshotWithRoute(name string) *model.EnvoySnapshot {
	return snapshotWithRoutes([]string{name})
}

func snapshotWithRoutes(names []string) *model.EnvoySnapshot {
	routes := make([]model.Route, len(names))
	for i, n := range names {
		routes[i] = model.Route{Name: n}
	}
	return &model.EnvoySnapshot{
		Listeners: []model.Listener{
			{Name: "listener~80", FilterChains: []model.NetworkFilterChain{
				{HCM: &model.HCMConfig{RouteConfig: &model.RouteConfig{
					VirtualHosts: []model.VirtualHost{{Name: "vh1", Routes: routes}},
				}}},
			}},
		},
	}
}

func snapshotWithTwoVHs(route1, route2 string) *model.EnvoySnapshot {
	return &model.EnvoySnapshot{
		Listeners: []model.Listener{
			{Name: "listener~80", FilterChains: []model.NetworkFilterChain{
				{HCM: &model.HCMConfig{RouteConfig: &model.RouteConfig{
					VirtualHosts: []model.VirtualHost{
						{Name: "vh1", Routes: []model.Route{{Name: route1}}},
						{Name: "vh2", Routes: []model.Route{{Name: route2}}},
					},
				}}},
			}},
		},
	}
}

func snapshotWithTwoListeners(route1, route2 string) *model.EnvoySnapshot {
	return &model.EnvoySnapshot{
		Listeners: []model.Listener{
			{Name: "listener~80", FilterChains: []model.NetworkFilterChain{
				{HCM: &model.HCMConfig{RouteConfig: &model.RouteConfig{
					VirtualHosts: []model.VirtualHost{{Name: "vh1", Routes: []model.Route{{Name: route1}}}},
				}}},
			}},
			{Name: "listener~443", FilterChains: []model.NetworkFilterChain{
				{HCM: &model.HCMConfig{RouteConfig: &model.RouteConfig{
					VirtualHosts: []model.VirtualHost{{Name: "vh2", Routes: []model.Route{{Name: route2}}}},
				}}},
			}},
		},
	}
}
```

- [ ] **Step 2.2: Run tests to confirm they fail (Filter is still a stub)**

```bash
go test ./internal/filter/... -v
```

Expected: most tests FAIL because Filter returns the snapshot unchanged.

- [ ] **Step 2.3: Implement `routeMatches` and the full `Filter` function**

Replace the stub in `internal/filter/filter.go` with the full implementation:

```go
// Package filter provides route-level filtering of an [model.EnvoySnapshot]
// by HTTPRoute identity, as encoded in Envoy route names by kgateway.
package filter

import (
	"fmt"
	"strings"

	"github.com/DuncanDoyle/krp/internal/model"
)

// FilterOptions controls how [Filter] selects routes from a snapshot.
type FilterOptions struct {
	HTTPRouteName      string // HTTPRoute name, e.g. "api-example-com"
	HTTPRouteNamespace string // HTTPRoute namespace — NOT the Gateway namespace
	RuleIndex          int    // rule index within the HTTPRoute; -1 means all rules
}

// Filter returns a new [model.EnvoySnapshot] containing only the routes
// whose Envoy route name embeds the given HTTPRoute identity.
// Listeners and virtual hosts with no remaining routes are pruned.
// Filter chains with no matching routes are kept with an empty VirtualHosts slice
// as long as another chain in the same listener has matches.
// If opts.HTTPRouteName is empty, the snapshot is returned unchanged.
func Filter(snapshot *model.EnvoySnapshot, opts FilterOptions) *model.EnvoySnapshot {
	if opts.HTTPRouteName == "" {
		return snapshot
	}

	result := &model.EnvoySnapshot{}
	for _, l := range snapshot.Listeners {
		if filtered := filterListener(l, opts); filtered != nil {
			result.Listeners = append(result.Listeners, *filtered)
		}
	}
	return result
}

// filterListener returns a copy of l with only matching routes, or nil if no
// filter chain in the listener has any matching routes.
func filterListener(l model.Listener, opts FilterOptions) *model.Listener {
	out := l
	out.FilterChains = nil
	for _, fc := range l.FilterChains {
		out.FilterChains = append(out.FilterChains, filterChain(fc, opts))
	}
	// Drop the listener entirely if no filter chain has any matching VHs
	for _, fc := range out.FilterChains {
		if fc.HCM != nil && fc.HCM.RouteConfig != nil && len(fc.HCM.RouteConfig.VirtualHosts) > 0 {
			return &out
		}
	}
	return nil
}

// filterChain returns a copy of fc with only matching virtual hosts.
// A filter chain with no matches is returned with an empty VirtualHosts slice
// (the HCM is preserved so the renderer can still show the filter pipeline).
func filterChain(fc model.NetworkFilterChain, opts FilterOptions) model.NetworkFilterChain {
	out := fc
	if fc.HCM == nil || fc.HCM.RouteConfig == nil {
		return out
	}
	hcmCopy := *fc.HCM
	rcCopy := *fc.HCM.RouteConfig
	rcCopy.VirtualHosts = nil
	for _, vh := range fc.HCM.RouteConfig.VirtualHosts {
		if filtered := filterVirtualHost(vh, opts); filtered != nil {
			rcCopy.VirtualHosts = append(rcCopy.VirtualHosts, *filtered)
		}
	}
	hcmCopy.RouteConfig = &rcCopy
	out.HCM = &hcmCopy
	return out
}

// filterVirtualHost returns a copy of vh with only matching routes, or nil if none match.
func filterVirtualHost(vh model.VirtualHost, opts FilterOptions) *model.VirtualHost {
	out := vh
	out.Routes = nil
	for _, r := range vh.Routes {
		if routeMatches(r.Name, opts) {
			out.Routes = append(out.Routes, r)
		}
	}
	if len(out.Routes) == 0 {
		return nil
	}
	return &out
}

// routeMatches reports whether the Envoy route name encodes the HTTPRoute
// identity from opts. It uses substring matching on the kgateway convention:
//
//	...-httproute-<name>-<namespace>-<rule_idx>-<backend_idx>-matcher-<matcher_idx>
//
// Note: HTTPRouteNamespace is the HTTPRoute namespace, which may differ from
// the Gateway namespace used for port-forwarding.
func routeMatches(name string, opts FilterOptions) bool {
	// Base marker identifies the HTTPRoute by name and namespace
	base := fmt.Sprintf("httproute-%s-%s-", opts.HTTPRouteName, opts.HTTPRouteNamespace)
	if !strings.Contains(name, base) {
		return false
	}
	// Optional rule filter narrows to a specific rule index
	if opts.RuleIndex >= 0 {
		ruleMarker := fmt.Sprintf("%s%d-", base, opts.RuleIndex)
		return strings.Contains(name, ruleMarker)
	}
	return true
}
```

- [ ] **Step 2.4: Run tests — all should pass**

```bash
go test ./internal/filter/... -v
```

Expected: all `TestFilter_*` tests PASS.

- [ ] **Step 2.5: Commit**

```bash
git add internal/filter/filter.go internal/filter/filter_test.go
git commit -m "feat: implement filter.Filter with route name substring matching"
```

---

## Task 3: Add `--httproute` and `--rule` flags to `cmd/kfp/main.go`

**Files:**
- Modify: `cmd/kfp/main.go`

Note: CLI-level validation logic (`--rule requires --httproute`, `--rule must be >= 0`, the empty-snapshot warning) is covered by manual smoke tests in the verification checklist. Automated CLI tests are deferred as a follow-up.

- [ ] **Step 3.1: Add flags to the `dump` command in `main()`**

After the existing flag declarations, add:

```go
dump.Flags().String("httproute", "", "Filter output to routes belonging to this HTTPRoute name")
dump.Flags().String("httproute-namespace", "", "Namespace of the HTTPRoute (required with --httproute; may differ from the Gateway namespace)")
dump.Flags().Int("rule", -1, "Rule index within the HTTPRoute (-1 = all rules, used with --httproute)")
```

- [ ] **Step 3.2: Read the flags and add validation in `runDump`**

After reading the existing flags, add:

```go
httprouteName, _      := cmd.Flags().GetString("httproute")
httprouteNamespace, _ := cmd.Flags().GetString("httproute-namespace")
ruleIndex, _          := cmd.Flags().GetInt("rule")

// --httproute and --httproute-namespace must be used together
if httprouteName != "" && httprouteNamespace == "" {
    return fmt.Errorf("--httproute-namespace is required when --httproute is set")
}
if httprouteNamespace != "" && httprouteName == "" {
    return fmt.Errorf("--httproute-namespace requires --httproute")
}
if ruleIndex < -1 {
    return fmt.Errorf("--rule must be >= 0 (use -1 for all rules, which is the default)")
}
if ruleIndex >= 0 && httprouteName == "" {
    return fmt.Errorf("--rule requires --httproute")
}
```

- [ ] **Step 3.3: Wire the filter between parser and renderer**

Replace the current parse+render block with:

```go
// Parse the config dump into an EnvoySnapshot
result, err := parser.Parse(data)
if err != nil {
    return err
}

// Print any non-fatal parse warnings to stderr
for _, w := range result.Warnings {
    fmt.Fprintf(os.Stderr, "warning: %s\n", w)
}

// Apply HTTPRoute filter if requested
snapshot := result.Snapshot
if httprouteName != "" {
    snapshot = filter.Filter(snapshot, filter.FilterOptions{
        HTTPRouteName:      httprouteName,
        HTTPRouteNamespace: httprouteNamespace,
        RuleIndex:          ruleIndex,
    })
    if len(snapshot.Listeners) == 0 {
        fmt.Fprintf(os.Stderr,
            "warning: no routes found for HTTPRoute %s/%s — check name, namespace, and that the HTTPRoute is attached to this Gateway\n",
            httprouteNamespace, httprouteName)
    }
}

// Render and print
fmt.Println(renderer.Render(snapshot))
return nil
```

- [ ] **Step 3.4: Add the filter package import**

Add to the import block in `cmd/kfp/main.go`:

```go
"github.com/DuncanDoyle/krp/internal/filter"
```

- [ ] **Step 3.5: Build**

```bash
go build ./...
```

Expected: clean build.

- [ ] **Step 3.6: Run all tests**

```bash
go test ./...
```

Expected: all tests pass.

- [ ] **Step 3.7: Commit**

```bash
git add cmd/kfp/main.go
git commit -m "feat: add --httproute and --rule flags to krp dump"
```

---

## Task 4: E2E test — filter against a real config dump

**Files:**
- Create: `internal/filter/e2e_test.go`

This test parses a real config dump and verifies that filtering by a known HTTPRoute name returns only the expected routes. The `routeName` helper and snapshot helpers are defined in `filter_test.go` (same package, same directory) and are available here.

The `00-simple` scenario contains HTTPRoute `api-example-com` in namespace `default`, producing route names of the form `listener~80~api_example_com-route-0-httproute-api-example-com-default-0-0-matcher-0`.

- [ ] **Step 4.1: Write the E2E tests**

Create `internal/filter/e2e_test.go`:

```go
package filter_test

import (
	"os"
	"strings"
	"testing"

	"github.com/DuncanDoyle/krp/internal/filter"
	"github.com/DuncanDoyle/krp/internal/parser"
)

// testdataPath returns the path to a testdata file relative to the project root.
func testdataPath(scenario, file string) string {
	return "../../testdata/scenarios/" + scenario + "/" + file
}

func TestFilter_E2E_SimpleHTTP_Match(t *testing.T) {
	data, err := os.ReadFile(testdataPath("00-simple", "envoy/config_dump.json"))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	result, err := parser.Parse(data)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	// 00-simple: one HTTPRoute "api-example-com" in namespace "default"
	opts := filter.FilterOptions{
		HTTPRouteName:      "api-example-com",
		HTTPRouteNamespace: "default",
		RuleIndex:          -1,
	}
	filtered := filter.Filter(result.Snapshot, opts)

	if len(filtered.Listeners) == 0 {
		t.Fatal("expected at least one listener after filter")
	}
	// Every remaining route must embed the httproute marker
	for _, l := range filtered.Listeners {
		for _, fc := range l.FilterChains {
			if fc.HCM == nil || fc.HCM.RouteConfig == nil {
				continue
			}
			for _, vh := range fc.HCM.RouteConfig.VirtualHosts {
				for _, r := range vh.Routes {
					if !strings.Contains(r.Name, "httproute-api-example-com-default-") {
						t.Errorf("unexpected route in filtered snapshot: %s", r.Name)
					}
				}
			}
		}
	}
}

func TestFilter_E2E_SimpleHTTP_NoMatch(t *testing.T) {
	data, err := os.ReadFile(testdataPath("00-simple", "envoy/config_dump.json"))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	result, err := parser.Parse(data)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	opts := filter.FilterOptions{
		HTTPRouteName:      "does-not-exist",
		HTTPRouteNamespace: "default",
		RuleIndex:          -1,
	}
	filtered := filter.Filter(result.Snapshot, opts)

	if len(filtered.Listeners) != 0 {
		t.Fatalf("expected empty snapshot, got %d listeners", len(filtered.Listeners))
	}
}
```

- [ ] **Step 4.2: Run the E2E tests — expect PASS (filter is already implemented)**

```bash
go test ./internal/filter/... -run TestFilter_E2E -v
```

Expected: both E2E tests PASS. If `TestFilter_E2E_SimpleHTTP_Match` fails, inspect the actual route names in the fixture to confirm the HTTPRoute name/namespace:

```bash
python3 -c "
import json
with open('testdata/scenarios/00-simple/envoy/config_dump.json') as f:
    d = json.load(f)
for s in d['configs']:
    if 'RoutesConfigDump' in s.get('@type','') and 'Scoped' not in s.get('@type',''):
        for drc in s.get('dynamic_route_configs',[]):
            for vh in drc['route_config']['virtual_hosts']:
                for r in vh['routes']:
                    print(r.get('name'))
"
```

Update the expected `HTTPRouteName` / `Namespace` in the test to match actual values if the fixture differs.

- [ ] **Step 4.3: Run all tests**

```bash
go test ./...
```

Expected: all pass.

- [ ] **Step 4.4: Commit**

```bash
git add internal/filter/e2e_test.go
git commit -m "test: add e2e filter tests against real config dumps"
```

---

## Task 5: Update docs and phase status

**Files:**
- Modify: `docs/phase-status.md`
- Modify: `docs/plans/kfp-roadmap.md`

- [ ] **Step 5.1: Verify the Phase 2 heading exists in the roadmap**

```bash
grep "Phase 2" docs/plans/kfp-roadmap.md
```

Expected: line containing `### Phase 2 — HTTPRoute Selector`.

- [ ] **Step 5.2: Update roadmap Phase 2 status**

In `docs/plans/kfp-roadmap.md`, update the Phase 2 section:

```markdown
### Phase 2 — HTTPRoute Selector
**Status:** Complete
**Docs:** `2026-03-17-phase-2-httproute-selector-design.md`, `2026-03-17-phase-2-httproute-selector-implementation.md`
```

- [ ] **Step 5.3: Update `docs/phase-status.md`**

Replace content with Phase 2 completion record (list all issues resolved, note deferred items).

- [ ] **Step 5.4: Commit docs**

```bash
git add docs/phase-status.md docs/plans/kfp-roadmap.md
git commit -m "docs: mark Phase 2 complete in roadmap and phase-status"
```

---

## Verification Checklist

Before declaring Phase 2 complete:

- [ ] `go build ./...` — clean
- [ ] `go test ./...` — all pass
- [ ] Manual: `krp dump --file testdata/scenarios/00-simple/envoy/config_dump.json --route api-example-com --route-ns default` — shows only routes matching that HTTPRoute
- [ ] Manual: `krp dump --file testdata/scenarios/00-simple/envoy/config_dump.json --route does-not-exist --route-ns default` — prints warning to stderr, no routes rendered
- [ ] Manual: `krp dump --file testdata/scenarios/00-simple/envoy/config_dump.json` — unfiltered output unchanged from Phase 1
- [ ] Manual: `krp dump --file testdata/scenarios/00-simple/envoy/config_dump.json --route api-example-com` — prints error `--route-ns is required when --route is set`
- [ ] Manual: `krp dump --file testdata/scenarios/00-simple/envoy/config_dump.json --route-ns default` — prints error `--route-ns requires --route`
- [ ] Manual: `krp dump --file testdata/scenarios/00-simple/envoy/config_dump.json --rule 0` — prints error `--rule requires --route`
- [ ] Manual: `krp dump --file testdata/scenarios/00-simple/envoy/config_dump.json --route api-example-com --route-ns default --rule -5` — prints error `--rule must be >= 0 (use -1 for all rules, which is the default)`
