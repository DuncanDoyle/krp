# Phase 3 — Filter Config Detail Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `--interactive`/`-i` flag to `krp dump` that launches a bubbletea TUI where the user can navigate to any HTTP filter and expand its typed config inline as pretty-printed JSON; static output remains the default and is unchanged.

**Architecture:** The parser/model/filter packages are not touched. `internal/renderer` gains an interactive render path via a new `renderer_interactive.go` file and a small refactor to `renderer.go` (adding an optional `*interactiveContext` parameter to `renderHTTPFilters` and `renderHCMContent`; nil = static, string-equal output to today). A new `internal/tui` package wraps bubbletea state and calls the renderer. The CLI branches at the output step: `--interactive` calls `tui.Run(snapshot)`, otherwise `renderer.Render(snapshot)` as before.

**Tech Stack:** Go 1.25, `github.com/charmbracelet/bubbletea` v1.x, `github.com/charmbracelet/bubbles/viewport` (bubbles v0.21.x), `github.com/charmbracelet/lipgloss` v1.1.0 (already in go.mod), `github.com/spf13/cobra` (already in go.mod).

---

## File Map

| Action | File | Responsibility |
|--------|------|---------------|
| Modify | `internal/model/envoy.go` | Fix stale comment on `HTTPFilter.TypedConfig` |
| Modify | `internal/renderer/renderer.go` | Refactor `renderListener`, `renderFilterChain`, `renderHCMContent`, `renderHTTPFilters` to accept `*interactiveContext`; nil = static |
| Create | `internal/renderer/renderer_interactive.go` | `FilterRef`, `RenderOpts`, `interactiveContext`, `RenderInteractive` |
| Modify | `internal/renderer/renderer_test.go` | Add `RenderInteractive` tests |
| Create | `internal/tui/tui.go` | bubbletea `model`, `buildItems`, `Init`, `Update`, `View`, `Run` |
| Create | `internal/tui/tui_test.go` | `buildItems` unit tests |
| Modify | `cmd/krp/main.go` | Add `--interactive`/`-i` flag; branch in `runDump` |
| Modify | `go.mod` / `go.sum` | Add bubbletea + bubbles direct dependencies |

---

## Task 1: Add dependencies and fix stale comment

**Files:**
- Modify: `go.mod`, `go.sum` (via `go get`)
- Modify: `internal/model/envoy.go:45`

- [ ] **Step 1: Add bubbletea and bubbles**

```bash
cd /Users/ddoyle/Development/claude/kgateway-filterchain-printer-cli-claude
go get github.com/charmbracelet/bubbletea@latest
go get github.com/charmbracelet/bubbles@latest
go mod tidy
```

Expected: both packages added to `go.mod`. Verify `go.mod` now contains entries for `github.com/charmbracelet/bubbletea` (v1.x) and `github.com/charmbracelet/bubbles` (v0.21.x or later). `go mod tidy` removes any unused indirect deps and ensures `go.sum` is consistent.

- [ ] **Step 2: Verify existing tests still pass**

```bash
go test ./...
```

Expected: all five packages pass — `internal/envoy`, `internal/filter`, `internal/model`, `internal/parser`, `internal/renderer`.

- [ ] **Step 3: Fix stale comment in model/envoy.go**

In `internal/model/envoy.go`, line 45, change:
```go
TypedConfig map[string]any `json:"typedConfig,omitempty"` // raw typed config (for Phase 2)
```
to:
```go
TypedConfig map[string]any `json:"typedConfig,omitempty"` // raw typed config; displayed in interactive mode (Phase 3)
```

- [ ] **Step 4: Verify model tests still pass**

```bash
go test ./internal/model/... -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum internal/model/envoy.go
git commit -m "feat: add bubbletea/bubbles deps; fix stale TypedConfig comment"
```

---

## Task 2: Define interactiveContext and refactor renderer helpers

This is a pure nil-safe refactor. No behaviour changes — all existing tests must continue to pass after this task.

**Files:**
- Create: `internal/renderer/renderer_interactive.go` (types only, no RenderInteractive yet)
- Modify: `internal/renderer/renderer.go`

### What changes in renderer.go

Four internal functions gain an `ctx *interactiveContext` parameter. When `ctx == nil` the body is identical to today. Callers inside `Render` always pass `nil`.

New signatures:

```go
func renderListener(l model.Listener, ctx *interactiveContext) string
func renderFilterChain(b *strings.Builder, fc model.NetworkFilterChain, idx int, isLast bool, ctx *interactiveContext)
func renderHCMContent(b *strings.Builder, hcm *model.HCMConfig, indent string, ctx *interactiveContext)
func renderHTTPFilters(b *strings.Builder, filters []model.HTTPFilter, typedPerFilterConfig map[string]any, indent string, ctx *interactiveContext)
```

Coordinate tracking: before each call to a deeper helper, the outer function updates the relevant field of `ctx.ref` (only when `ctx != nil`):

```go
// In Render (unchanged public API — passes nil throughout):
for _, listener := range snapshot.Listeners {
    panels = append(panels, renderListener(listener, nil))
}

// In renderListener: update FilterChainIdx before each renderFilterChain call
for i, fc := range l.FilterChains {
    isLast := i == len(l.FilterChains)-1
    if ctx != nil {
        ctx.ref.FilterChainIdx = i
    }
    renderFilterChain(&b, fc, i, isLast, ctx)
}

// In renderHCMContent: update VirtualHostIdx and RouteIdx in the nested loops
for i, vh := range hcm.RouteConfig.VirtualHosts {
    if ctx != nil { ctx.ref.VirtualHostIdx = i }
    // ...
    for j, route := range vh.Routes {
        if ctx != nil { ctx.ref.RouteIdx = j }
        // ...
        renderHTTPFilters(b, hcm.HTTPFilters, route.TypedPerFilterConfig, filterIndent, ctx)
    }
}
```

`renderHTTPFilters` updates `ctx.ref.FilterIdx = i` for each filter (used in Task 3).

- [ ] **Step 1: Add the interactiveContext struct to renderer_interactive.go**

Create `internal/renderer/renderer_interactive.go` with only the types (no `RenderInteractive` function yet):

```go
// Package renderer provides static and interactive rendering of an
// [model.EnvoySnapshot].
package renderer

import "github.com/DuncanDoyle/krp/internal/model"

// FilterRef uniquely identifies a single HTTP filter instance as rendered
// under a specific route. The Envoy path is:
// Listener[ListenerIdx] → FilterChain[FilterChainIdx] → HCM → RouteConfig →
// VirtualHost[VirtualHostIdx] → Route[RouteIdx] → hcm.HTTPFilters[FilterIdx].
//
// Because the same HCM filter list is rendered once per route, the full path
// is required to distinguish instances.
type FilterRef struct {
	ListenerIdx    int
	FilterChainIdx int
	VirtualHostIdx int
	RouteIdx       int
	FilterIdx      int // zero-based index into hcm.HTTPFilters
}

// RenderOpts carries the cursor position and expansion state for
// [RenderInteractive]. Both fields are optional: a nil Cursor means no item
// is highlighted; an empty or nil Expanded map means no items are expanded.
type RenderOpts struct {
	Cursor   *FilterRef
	Expanded map[FilterRef]bool
}

// interactiveContext carries cursor and expansion state into the internal
// render helpers. A nil pointer means static mode — behaviour is identical
// to the pre-Phase-3 code path.
//
// ref is updated at each level of the render traversal so that
// renderHTTPFilters can identify the current filter's coordinates and compare
// them against cursor and expanded.
type interactiveContext struct {
	ref      FilterRef
	cursor   *FilterRef
	expanded map[FilterRef]bool
}

// RenderInteractive produces the same styled tree as [Render] with two
// additions driven by opts. renderHTTPFilters and renderHCMContent are
// refactored to accept an optional *interactiveContext; passing nil (as
// [Render] does) preserves the existing string-equal output.
//
// The cursor item (opts.Cursor) is highlighted with a reversed foreground/
// background style. Expanded items (opts.Expanded) show their typed config
// inline as indented JSON (json.MarshalIndent with two-space indent).
//
// Config resolution for an expanded filter: Route.TypedPerFilterConfig[filter.Name]
// is shown if non-nil (per-route override, including empty map {}); otherwise
// HTTPFilter.TypedConfig (HCM-level config) is shown; if neither is set,
// "(no typed config)" is printed.
//
// RenderInteractive is a pure function — it performs no I/O.
func RenderInteractive(snapshot *model.EnvoySnapshot, opts RenderOpts) string {
	// Implemented in Task 3
	panic("not implemented")
}
```

- [ ] **Step 2: Verify it compiles**

```bash
go build ./internal/renderer/...
```

Expected: compiles (the panic body is valid Go).

- [ ] **Step 3: Refactor renderer.go — update the four function signatures**

Update `renderListener`, `renderFilterChain`, `renderHCMContent`, and `renderHTTPFilters` in `renderer.go` to accept `ctx *interactiveContext`. Add coordinate-tracking nil-guards. The `Render` function passes `nil` at every call site — its body does not change beyond calling `renderListener(listener, nil)`.

The full updated body of each function is shown below. These are mechanical additions — the only new logic is `if ctx != nil { ctx.ref.XxxIdx = i }` in the outer loops, and `if ctx != nil { ctx.ref.FilterIdx = i }` in `renderHTTPFilters`. No rendering output changes.

`renderListener`:
```go
func renderListener(l model.Listener, ctx *interactiveContext) string {
	var b strings.Builder

	title := listenerTitleStyle.Render(fmt.Sprintf("Listener: %s", l.Name))
	addr := domainStyle.Render(l.Address)
	b.WriteString(fmt.Sprintf("%s %s\n", title, addr))

	for i, fc := range l.FilterChains {
		isLast := i == len(l.FilterChains)-1
		if ctx != nil {
			ctx.ref.FilterChainIdx = i
		}
		renderFilterChain(&b, fc, i, isLast, ctx)
	}

	return listenerStyle.Render(b.String())
}
```

`renderFilterChain`:
```go
func renderFilterChain(b *strings.Builder, fc model.NetworkFilterChain, idx int, isLast bool, ctx *interactiveContext) {
	prefix := treeT
	childPrefix := treeI
	if isLast {
		prefix = treeL
		childPrefix = treeSpc
	}

	label := filterChainLabelStyle.Render(fmt.Sprintf("FilterChain[%d]", idx))
	if fc.Name != "" {
		label += " " + domainStyle.Render(fc.Name)
	}
	if fc.TLS != nil && len(fc.TLS.SNIHosts) > 0 {
		label += " " + tlsStyle.Render(fmt.Sprintf("TLS: %s", strings.Join(fc.TLS.SNIHosts, ", ")))
	}
	b.WriteString(fmt.Sprintf("%s %s\n", prefix, label))

	if fc.HCM == nil {
		b.WriteString(fmt.Sprintf("%s  %s\n", childPrefix, warningStyle.Render("[no HCM]")))
		return
	}

	b.WriteString(fmt.Sprintf("%s  %s HCM %s RDS: %s\n",
		childPrefix, treeL, domainStyle.Render("→"), fc.HCM.RouteConfigName))

	renderHCMContent(b, fc.HCM, childPrefix+treeSpc+"  ", ctx)
}
```

`renderHCMContent`:
```go
func renderHCMContent(b *strings.Builder, hcm *model.HCMConfig, indent string, ctx *interactiveContext) {
	if hcm.RouteConfig == nil {
		b.WriteString(fmt.Sprintf("%s%s\n", indent, warningStyle.Render("[RDS not found: "+hcm.RouteConfigName+"]")))
		renderHTTPFilters(b, hcm.HTTPFilters, nil, indent, ctx)
		return
	}

	for i, vh := range hcm.RouteConfig.VirtualHosts {
		isLastVH := i == len(hcm.RouteConfig.VirtualHosts)-1
		vhPrefix := treeT
		vhChildPrefix := treeI
		if isLastVH {
			vhPrefix = treeL
			vhChildPrefix = treeSpc
		}
		if ctx != nil {
			ctx.ref.VirtualHostIdx = i
		}

		domains := domainStyle.Render(fmt.Sprintf("[%s]", strings.Join(vh.Domains, ", ")))
		b.WriteString(fmt.Sprintf("%s%s VirtualHost: %s %s\n",
			indent, vhPrefix, vhStyle.Render(vh.Name), domains))

		routeIndent := indent + vhChildPrefix + "  "
		for j, route := range vh.Routes {
			isLastRoute := j == len(vh.Routes)-1
			routePrefix := treeT
			routeChildPrefix := treeI
			if isLastRoute {
				routePrefix = treeL
				routeChildPrefix = treeSpc
			}
			if ctx != nil {
				ctx.ref.RouteIdx = j
			}

			matchStr := formatMatch(route.Match)
			b.WriteString(fmt.Sprintf("%s%s Route: %s\n",
				routeIndent, routePrefix, matchStyle.Render(matchStr)))

			filterIndent := routeIndent + routeChildPrefix + "  "
			renderHTTPFilters(b, hcm.HTTPFilters, route.TypedPerFilterConfig, filterIndent, ctx)
			renderRoutePolicies(b, route, filterIndent)

			if route.Cluster != "" {
				b.WriteString(fmt.Sprintf("%sBackend: %s\n",
					filterIndent, clusterStyle.Render(route.Cluster)))
			}
		}
	}
}
```

`renderHTTPFilters` (adds `ctx.ref.FilterIdx = i` only — no cursor/expansion logic yet, that comes in Task 3):
```go
func renderHTTPFilters(b *strings.Builder, filters []model.HTTPFilter, typedPerFilterConfig map[string]any, indent string, ctx *interactiveContext) {
	if len(filters) == 0 {
		return
	}

	b.WriteString(fmt.Sprintf("%sHTTP Filters:\n", indent))
	for i, f := range filters {
		isLast := i == len(filters)-1
		prefix := treeT
		if isLast {
			prefix = treeL
		}
		if ctx != nil {
			ctx.ref.FilterIdx = i
		}

		activeOnRoute := typedPerFilterConfig != nil && typedPerFilterConfig[f.Name] != nil
		label := filterStyle.Render(f.Name)
		if f.Disabled && !activeOnRoute {
			label = disabledStyle.Render(f.Name + " (disabled)")
		}

		b.WriteString(fmt.Sprintf("%s%s [%d] %s\n", indent, prefix, i+1, label))
	}
}
```

Also update `Render` to pass `nil`:
```go
func Render(snapshot *model.EnvoySnapshot) string {
	if len(snapshot.Listeners) == 0 {
		return warningStyle.Render("No listeners found in config dump.")
	}

	var panels []string
	for _, listener := range snapshot.Listeners {
		panels = append(panels, renderListener(listener, nil))
	}

	return strings.Join(panels, "\n")
}
```

- [ ] **Step 4: Run all renderer tests — must all pass**

```bash
go test ./internal/renderer/... -v
```

Expected: all existing tests PASS. Zero output changes — this is a pure refactor.

- [ ] **Step 5: Commit**

```bash
git add internal/renderer/renderer.go internal/renderer/renderer_interactive.go
git commit -m "refactor: add *interactiveContext param to renderer helpers (nil = static)"
```

---

## Task 3: Implement RenderInteractive with tests

**Files:**
- Modify: `internal/renderer/renderer_test.go`
- Modify: `internal/renderer/renderer_interactive.go`

### Key implementation notes

**Cursor style:** `cursorStyle = lipgloss.NewStyle().Reverse(true)` — define as a package-level var in `renderer_interactive.go`. Name must not conflict with existing vars in `renderer.go` (`listenerStyle`, `filterChainLabelStyle`, `tlsStyle`, `filterStyle`, `disabledStyle`, `clusterStyle`, `matchStyle`, `warningStyle`, `domainStyle`, `vhStyle`). `cursorStyle` does not appear in that list, so the name is safe to use.

**Config resolution helper** (private, in `renderer_interactive.go`):
```go
// resolveFilterConfig returns the typed config to display for an expanded filter.
// It returns Route.TypedPerFilterConfig[filter.Name] if non-nil (per-route override),
// otherwise HTTPFilter.TypedConfig. Returns nil if neither is set.
func resolveFilterConfig(f model.HTTPFilter, typedPerFilterConfig map[string]any) any {
	if typedPerFilterConfig != nil {
		if v, ok := typedPerFilterConfig[f.Name]; ok && v != nil {
			return v
		}
	}
	if f.TypedConfig != nil {
		return f.TypedConfig
	}
	return nil
}
```

**Finding cursor line** (used by TUI in Task 4 — defined here for testing):
The cursor item is rendered with `\x1b[7m` (reverse video ANSI code). After `RenderInteractive` produces the content string, the TUI finds the cursor line by counting newlines before the first occurrence of `\x1b[7m`.

**Full `RenderInteractive` implementation:**

```go
func RenderInteractive(snapshot *model.EnvoySnapshot, opts RenderOpts) string {
	if len(snapshot.Listeners) == 0 {
		return warningStyle.Render("No listeners found in config dump.")
	}

	ctx := &interactiveContext{
		cursor:   opts.Cursor,
		expanded: opts.Expanded,
	}

	var panels []string
	for i, listener := range snapshot.Listeners {
		ctx.ref.ListenerIdx = i
		panels = append(panels, renderListener(listener, ctx))
	}

	return strings.Join(panels, "\n")
}
```

**Updated `renderHTTPFilters`** (replaces the Task 2 version — adds cursor + expansion logic):

```go
func renderHTTPFilters(b *strings.Builder, filters []model.HTTPFilter, typedPerFilterConfig map[string]any, indent string, ctx *interactiveContext) {
	if len(filters) == 0 {
		return
	}

	b.WriteString(fmt.Sprintf("%sHTTP Filters:\n", indent))
	for i, f := range filters {
		isLast := i == len(filters)-1
		prefix := treeT
		if isLast {
			prefix = treeL
		}
		if ctx != nil {
			ctx.ref.FilterIdx = i
		}

		activeOnRoute := typedPerFilterConfig != nil && typedPerFilterConfig[f.Name] != nil

		// Build the base label text (preserving disabled state).
		labelText := f.Name
		if f.Disabled && !activeOnRoute {
			labelText = f.Name + " (disabled)"
		}

		// Apply cursor highlight or normal style.
		var label string
		if ctx != nil && ctx.cursor != nil && ctx.ref == *ctx.cursor {
			label = cursorStyle.Render(labelText)
		} else if f.Disabled && !activeOnRoute {
			label = disabledStyle.Render(labelText)
		} else {
			label = filterStyle.Render(labelText)
		}

		b.WriteString(fmt.Sprintf("%s%s [%d] %s\n", indent, prefix, i+1, label))

		// Render expanded typed config if this item is in the expanded set.
		if ctx != nil && ctx.expanded[ctx.ref] {
			config := resolveFilterConfig(f, typedPerFilterConfig)
			var configLines []string
			if config != nil {
				jsonBytes, err := json.MarshalIndent(config, "", "  ")
				if err == nil {
					configLines = strings.Split(string(jsonBytes), "\n")
				}
			}
			if len(configLines) == 0 {
				configLines = []string{"(no typed config)"}
			}
			for _, line := range configLines {
				b.WriteString(fmt.Sprintf("%s    %s\n", indent, line))
			}
		}
	}
}
```

Note: `encoding/json` must be imported in `renderer_interactive.go` (or `renderer.go`). Add to the import block of the file where `resolveFilterConfig` is defined.

- [ ] **Step 1: Write the failing tests in renderer_test.go**

Add the following test functions after the existing tests. Use the shared `routeSnapshotWithMatch` helper already in the file for the simple cases, and build richer snapshots inline where needed.

```go
// --- RenderInteractive tests ---

// interactiveSnapshot returns a minimal snapshot with one listener, one filter
// chain, one virtual host, one route, and two HTTP filters. The first filter
// has TypedConfig set (HCM-level config); the second has none.
func interactiveSnapshot() *model.EnvoySnapshot {
	return &model.EnvoySnapshot{
		Listeners: []model.Listener{
			{
				Name:    "listener~80",
				Address: "[::]:80",
				FilterChains: []model.NetworkFilterChain{
					{
						HCM: &model.HCMConfig{
							RouteConfigName: "listener~80",
							HTTPFilters: []model.HTTPFilter{
								{
									Name:        "io.solo.transformation",
									TypedConfig: map[string]any{"@type": "type.googleapis.com/envoy.api.v2.filter.http.RouteTransformations"},
								},
								{Name: "envoy.filters.http.router"},
							},
							RouteConfig: &model.RouteConfig{
								Name: "listener~80",
								VirtualHosts: []model.VirtualHost{
									{
										Name:    "vh",
										Domains: []string{"api.example.com"},
										Routes: []model.Route{
											{
												Match:   model.RouteMatch{Prefix: "/"},
												Cluster: "kube_httpbin_httpbin_8000",
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// TestRenderInteractive_NoOpts verifies that RenderInteractive with zero-value
// opts produces string-equal output to Render (same string, no ANSI differences).
func TestRenderInteractive_NoOpts(t *testing.T) {
	snapshot := interactiveSnapshot()
	static := renderer.Render(snapshot)
	interactive := renderer.RenderInteractive(snapshot, renderer.RenderOpts{})
	if static != interactive {
		t.Errorf("expected RenderInteractive with empty opts to equal Render output\nRender:\n%s\nRenderInteractive:\n%s", static, interactive)
	}
}

// TestRenderInteractive_CursorOnFirstFilter verifies that a cursor on the first
// filter changes the output (the filter name is still present but styled differently).
func TestRenderInteractive_CursorOnFirstFilter(t *testing.T) {
	snapshot := interactiveSnapshot()
	cursor := renderer.FilterRef{ListenerIdx: 0, FilterChainIdx: 0, VirtualHostIdx: 0, RouteIdx: 0, FilterIdx: 0}
	output := renderer.RenderInteractive(snapshot, renderer.RenderOpts{Cursor: &cursor})

	if !strings.Contains(output, "io.solo.transformation") {
		t.Errorf("expected filter name to still be present in output\nOutput:\n%s", output)
	}
	static := renderer.Render(snapshot)
	if output == static {
		t.Errorf("expected cursor output to differ from static output")
	}
}

// TestRenderInteractive_ExpandedPerRouteConfig verifies that a filter with per-route
// TypedPerFilterConfig shows that config inline when expanded.
func TestRenderInteractive_ExpandedPerRouteConfig(t *testing.T) {
	snapshot := &model.EnvoySnapshot{
		Listeners: []model.Listener{
			{
				Name:    "listener~80",
				Address: "[::]:80",
				FilterChains: []model.NetworkFilterChain{
					{
						HCM: &model.HCMConfig{
							RouteConfigName: "listener~80",
							HTTPFilters: []model.HTTPFilter{
								{Name: "io.solo.transformation", Disabled: true},
								{Name: "envoy.filters.http.router"},
							},
							RouteConfig: &model.RouteConfig{
								Name: "listener~80",
								VirtualHosts: []model.VirtualHost{
									{
										Name:    "vh",
										Domains: []string{"api.example.com"},
										Routes: []model.Route{
											{
												Match:   model.RouteMatch{Prefix: "/"},
												Cluster: "kube_httpbin_httpbin_8000",
												TypedPerFilterConfig: map[string]any{
													"io.solo.transformation": map[string]any{
														"@type":        "type.googleapis.com/solo.transformation",
														"request_body": "passthrough",
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	ref := renderer.FilterRef{ListenerIdx: 0, FilterChainIdx: 0, VirtualHostIdx: 0, RouteIdx: 0, FilterIdx: 0}
	output := renderer.RenderInteractive(snapshot, renderer.RenderOpts{
		Expanded: map[renderer.FilterRef]bool{ref: true},
	})

	// Per-route config takes precedence; its keys should appear in the output.
	if !strings.Contains(output, "request_body") {
		t.Errorf("expected per-route config key 'request_body' in output\nOutput:\n%s", output)
	}
	if !strings.Contains(output, "passthrough") {
		t.Errorf("expected per-route config value 'passthrough' in output\nOutput:\n%s", output)
	}
}

// TestRenderInteractive_ExpandedHCMFallback verifies that a filter with no per-route
// TypedPerFilterConfig falls back to showing HTTPFilter.TypedConfig when expanded.
func TestRenderInteractive_ExpandedHCMFallback(t *testing.T) {
	snapshot := interactiveSnapshot() // first filter has TypedConfig at HCM level
	ref := renderer.FilterRef{ListenerIdx: 0, FilterChainIdx: 0, VirtualHostIdx: 0, RouteIdx: 0, FilterIdx: 0}
	output := renderer.RenderInteractive(snapshot, renderer.RenderOpts{
		Expanded: map[renderer.FilterRef]bool{ref: true},
	})

	if !strings.Contains(output, "RouteTransformations") {
		t.Errorf("expected HCM-level TypedConfig key 'RouteTransformations' in output\nOutput:\n%s", output)
	}
}

// TestRenderInteractive_ExpandedNoConfig verifies that expanding a filter with no
// typed config at either level shows "(no typed config)".
func TestRenderInteractive_ExpandedNoConfig(t *testing.T) {
	snapshot := interactiveSnapshot() // second filter (envoy.filters.http.router) has no TypedConfig
	ref := renderer.FilterRef{ListenerIdx: 0, FilterChainIdx: 0, VirtualHostIdx: 0, RouteIdx: 0, FilterIdx: 1}
	output := renderer.RenderInteractive(snapshot, renderer.RenderOpts{
		Expanded: map[renderer.FilterRef]bool{ref: true},
	})

	if !strings.Contains(output, "(no typed config)") {
		t.Errorf("expected '(no typed config)' in output\nOutput:\n%s", output)
	}
}

// TestRenderInteractive_ExpandedEmptyMap verifies that an empty map TypedPerFilterConfig
// entry is treated as "has config" and renders as "{}".
func TestRenderInteractive_ExpandedEmptyMap(t *testing.T) {
	snapshot := &model.EnvoySnapshot{
		Listeners: []model.Listener{
			{
				Name:    "listener~80",
				Address: "[::]:80",
				FilterChains: []model.NetworkFilterChain{
					{
						HCM: &model.HCMConfig{
							RouteConfigName: "listener~80",
							HTTPFilters:     []model.HTTPFilter{{Name: "envoy.filters.http.cors", Disabled: true}},
							RouteConfig: &model.RouteConfig{
								Name: "listener~80",
								VirtualHosts: []model.VirtualHost{
									{
										Name:    "vh",
										Domains: []string{"api.example.com"},
										Routes: []model.Route{
											{
												Match:   model.RouteMatch{Prefix: "/"},
												Cluster: "kube_httpbin_httpbin_8000",
												TypedPerFilterConfig: map[string]any{
													"envoy.filters.http.cors": map[string]any{}, // empty map
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	ref := renderer.FilterRef{ListenerIdx: 0, FilterChainIdx: 0, VirtualHostIdx: 0, RouteIdx: 0, FilterIdx: 0}
	output := renderer.RenderInteractive(snapshot, renderer.RenderOpts{
		Expanded: map[renderer.FilterRef]bool{ref: true},
	})

	if !strings.Contains(output, "{}") {
		t.Errorf("expected '{}' for empty map config\nOutput:\n%s", output)
	}
}

// TestRenderInteractive_AllExpanded verifies that expanding all items shows
// config for all of them.
func TestRenderInteractive_AllExpanded(t *testing.T) {
	snapshot := interactiveSnapshot()
	expanded := map[renderer.FilterRef]bool{
		{ListenerIdx: 0, FilterChainIdx: 0, VirtualHostIdx: 0, RouteIdx: 0, FilterIdx: 0}: true,
		{ListenerIdx: 0, FilterChainIdx: 0, VirtualHostIdx: 0, RouteIdx: 0, FilterIdx: 1}: true,
	}
	output := renderer.RenderInteractive(snapshot, renderer.RenderOpts{Expanded: expanded})

	// First filter has HCM-level config; second has none.
	if !strings.Contains(output, "RouteTransformations") {
		t.Errorf("expected first filter's config in output\nOutput:\n%s", output)
	}
	if !strings.Contains(output, "(no typed config)") {
		t.Errorf("expected '(no typed config)' for second filter\nOutput:\n%s", output)
	}
}

// TestRenderInteractive_NilHCM verifies that a filter chain with a nil HCM does
// not crash and produces no cursor or expansion output.
func TestRenderInteractive_NilHCM(t *testing.T) {
	snapshot := &model.EnvoySnapshot{
		Listeners: []model.Listener{
			{
				Name:    "listener~80",
				Address: "[::]:80",
				FilterChains: []model.NetworkFilterChain{
					{Name: "no-hcm", HCM: nil}, // nil HCM — no navigable filters
				},
			},
		},
	}

	// Should not panic even with a cursor set.
	ref := renderer.FilterRef{}
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("RenderInteractive panicked on nil-HCM snapshot: %v", r)
		}
	}()
	output := renderer.RenderInteractive(snapshot, renderer.RenderOpts{Cursor: &ref})

	if !strings.Contains(output, "[no HCM]") {
		t.Errorf("expected '[no HCM]' in output for nil-HCM filter chain\nOutput:\n%s", output)
	}
}
```

- [ ] **Step 2: Run the new tests — they must FAIL**

```bash
go test ./internal/renderer/... -v -run TestRenderInteractive
```

Expected: FAIL — `TestRenderInteractive_NoOpts` fails because `RenderInteractive` panics.

- [ ] **Step 3: Implement RenderInteractive in renderer_interactive.go**

Replace the `panic("not implemented")` body with the full implementation. Add the `cursorStyle` var, `resolveFilterConfig` helper, and the full `RenderInteractive` body as shown in the implementation notes above. Add `"encoding/json"` and `"strings"` to the imports.

Also update `renderHTTPFilters` in `renderer.go` to replace the Task 2 version with the full version that includes cursor + expansion logic (as shown in the implementation notes).

- [ ] **Step 4: Run the new tests — they must all PASS**

```bash
go test ./internal/renderer/... -v -run TestRenderInteractive
```

Expected: all 8 `TestRenderInteractive_*` tests PASS.

- [ ] **Step 5: Run the full renderer test suite — no regressions**

```bash
go test ./internal/renderer/... -v
```

Expected: all tests PASS (existing + new).

- [ ] **Step 6: Commit**

```bash
git add internal/renderer/renderer_interactive.go internal/renderer/renderer.go internal/renderer/renderer_test.go
git commit -m "feat: implement RenderInteractive with cursor highlight and typed config expansion"
```

---

## Task 4: Implement internal/tui package

**Files:**
- Create: `internal/tui/tui.go`
- Create: `internal/tui/tui_test.go`

### Package overview

`internal/tui/tui.go` contains:
- `model` struct (unexported bubbletea model)
- `buildItems(snapshot) []renderer.FilterRef` (unexported, tested directly)
- `Init()`, `Update()`, `View()` (bubbletea interface)
- `findCursorLine(content string) int` (unexported helper)
- `Run(snapshot) error` (exported entry point)

The `helpBar` is a package-level constant string rendered with a dimmed style.

### buildItems traversal

Follows the canonical order from the spec (matching `RenderInteractive`):
1. Listeners by index
2. FilterChains by index — skip if `HCM == nil`
3. Skip if `HCM.RouteConfig == nil`
4. VirtualHosts by index
5. Routes by index
6. HTTPFilters by index — emit one `FilterRef` per filter per route

### findCursorLine

```go
// findCursorLine finds the line number of the cursor item in the rendered
// content by locating the ANSI reverse-video code emitted by cursorStyle.
// Returns 0 if no cursor item is present (no cursor set or ANSI disabled).
func findCursorLine(content string) int {
	const reverseCode = "\x1b[7m"
	idx := strings.Index(content, reverseCode)
	if idx < 0 {
		return 0
	}
	return strings.Count(content[:idx], "\n")
}
```

### Full tui.go

```go
// Package tui provides the interactive bubbletea TUI for krp.
// It wraps an [model.EnvoySnapshot] in a scrollable, navigable terminal
// UI where the user can expand HTTP filter typed configs inline.
package tui

import (
	"fmt"
	"os"
	"strings"

	"github.com/DuncanDoyle/krp/internal/model"
	"github.com/DuncanDoyle/krp/internal/renderer"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	defaultWidth  = 80
	defaultHeight = 23 // 24 rows minus 1 for the help bar
)

var helpBarStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

const helpText = "↑/↓ navigate • Enter/Space expand • a toggle all • q quit"

// model holds all mutable state for the interactive TUI session.
type model struct {
	snapshot *model.EnvoySnapshot
	items    []renderer.FilterRef
	cursor   int
	expanded map[renderer.FilterRef]bool
	viewport viewport.Model
}

// buildItems returns the flat ordered list of all navigable FilterRefs in the
// snapshot. It follows the canonical traversal order so that items[N] always
// corresponds to the N-th filter rendered on screen by [renderer.RenderInteractive].
//
// Filter chains with a nil HCM or a nil RouteConfig are skipped — they produce
// no navigable filters (same as the renderer, which shows "[no HCM]" / "[RDS not found]").
func buildItems(snapshot *model.EnvoySnapshot) []renderer.FilterRef {
	var items []renderer.FilterRef
	for lIdx, listener := range snapshot.Listeners {
		for fcIdx, fc := range listener.FilterChains {
			if fc.HCM == nil || fc.HCM.RouteConfig == nil {
				continue
			}
			for vhIdx, vh := range fc.HCM.RouteConfig.VirtualHosts {
				for rIdx := range vh.Routes {
					for fIdx := range fc.HCM.HTTPFilters {
						items = append(items, renderer.FilterRef{
							ListenerIdx:    lIdx,
							FilterChainIdx: fcIdx,
							VirtualHostIdx: vhIdx,
							RouteIdx:       rIdx,
							FilterIdx:      fIdx,
						})
					}
				}
			}
		}
	}
	return items
}

// findCursorLine finds the line number of the cursor item in the rendered
// content by locating the ANSI reverse-video code emitted by cursorStyle.
// Returns 0 if no cursor item is present.
func findCursorLine(content string) int {
	const reverseCode = "\x1b[7m"
	idx := strings.Index(content, reverseCode)
	if idx < 0 {
		return 0
	}
	return strings.Count(content[:idx], "\n")
}

// renderOpts builds the RenderOpts for the current model state.
func (m model) renderOpts() renderer.RenderOpts {
	if len(m.items) == 0 {
		return renderer.RenderOpts{}
	}
	cursor := m.items[m.cursor]
	return renderer.RenderOpts{
		Cursor:   &cursor,
		Expanded: m.expanded,
	}
}

// setContent re-renders the snapshot with the current interactive state and
// sets the viewport content. Called on every state change.
func (m *model) setContent() {
	content := renderer.RenderInteractive(m.snapshot, m.renderOpts())
	m.viewport.SetContent(content)
	m.viewport.SetYOffset(findCursorLine(content))
}

func initialModel(snapshot *model.EnvoySnapshot) model {
	vp := viewport.New(defaultWidth, defaultHeight)
	m := model{
		snapshot: snapshot,
		items:    buildItems(snapshot),
		cursor:   0,
		expanded: make(map[renderer.FilterRef]bool),
		viewport: vp,
	}
	return m
}

// Init implements tea.Model. No initial commands are needed.
func (m model) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - 1 // reserve one line for the help bar
		m.setContent()
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m.setContent()
			}
			return m, nil

		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
				m.setContent()
			}
			return m, nil

		case "enter", " ":
			if len(m.items) > 0 {
				ref := m.items[m.cursor]
				if m.expanded[ref] {
					delete(m.expanded, ref)
				} else {
					m.expanded[ref] = true
				}
				m.setContent()
			}
			return m, nil

		case "a":
			if len(m.items) == 0 {
				return m, nil
			}
			if len(m.expanded) == len(m.items) {
				// All expanded — collapse all.
				m.expanded = make(map[renderer.FilterRef]bool)
			} else {
				// Some or none expanded — expand all.
				for _, ref := range m.items {
					m.expanded[ref] = true
				}
			}
			m.setContent()
			return m, nil
		}
	}

	// Forward remaining messages (e.g. mouse events) to the viewport.
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// View implements tea.Model.
func (m model) View() string {
	return m.viewport.View() + "\n" + helpBarStyle.Render(helpText)
}

// Run starts the interactive TUI for the given snapshot. It prints a warning
// to stderr and returns immediately (without launching the bubbletea program)
// if the snapshot contains no navigable filters. It blocks until the user quits
// (q or Ctrl+C) and returns any bubbletea program error.
func Run(snapshot *model.EnvoySnapshot) error {
	m := initialModel(snapshot)
	if len(m.items) == 0 {
		fmt.Fprintln(os.Stderr, "no expandable filters found")
		return nil
	}
	// Content is initialised in Update() when the first tea.WindowSizeMsg arrives.
	// This is the standard bubbletea pattern — do not call setContent() here.
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
```

- [ ] **Step 1: Write the failing buildItems tests**

Create `internal/tui/tui_test.go`:

```go
package tui

import (
	"testing"

	"github.com/DuncanDoyle/krp/internal/model"
	"github.com/DuncanDoyle/krp/internal/renderer"
)

// TestBuildItems_SimpleSnapshot verifies that buildItems produces one FilterRef
// per filter per route, in the canonical traversal order.
func TestBuildItems_SimpleSnapshot(t *testing.T) {
	snapshot := &model.EnvoySnapshot{
		Listeners: []model.Listener{
			{
				Name: "listener~80",
				FilterChains: []model.NetworkFilterChain{
					{
						HCM: &model.HCMConfig{
							RouteConfigName: "listener~80",
							HTTPFilters: []model.HTTPFilter{
								{Name: "io.solo.transformation"},
								{Name: "envoy.filters.http.router"},
							},
							RouteConfig: &model.RouteConfig{
								VirtualHosts: []model.VirtualHost{
									{
										Routes: []model.Route{
											{Match: model.RouteMatch{Prefix: "/"}},
											{Match: model.RouteMatch{Prefix: "/api"}},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	items := buildItems(snapshot)

	// 2 routes × 2 filters = 4 items
	if len(items) != 4 {
		t.Fatalf("expected 4 items, got %d", len(items))
	}

	// Verify canonical order: route 0 filter 0, route 0 filter 1, route 1 filter 0, route 1 filter 1
	expected := []renderer.FilterRef{
		{ListenerIdx: 0, FilterChainIdx: 0, VirtualHostIdx: 0, RouteIdx: 0, FilterIdx: 0},
		{ListenerIdx: 0, FilterChainIdx: 0, VirtualHostIdx: 0, RouteIdx: 0, FilterIdx: 1},
		{ListenerIdx: 0, FilterChainIdx: 0, VirtualHostIdx: 0, RouteIdx: 1, FilterIdx: 0},
		{ListenerIdx: 0, FilterChainIdx: 0, VirtualHostIdx: 0, RouteIdx: 1, FilterIdx: 1},
	}
	for i, got := range items {
		if got != expected[i] {
			t.Errorf("items[%d]: got %+v, want %+v", i, got, expected[i])
		}
	}
}

// TestBuildItems_NilHCM verifies that filter chains with a nil HCM are skipped.
func TestBuildItems_NilHCM(t *testing.T) {
	snapshot := &model.EnvoySnapshot{
		Listeners: []model.Listener{
			{
				Name: "listener~80",
				FilterChains: []model.NetworkFilterChain{
					{Name: "no-hcm", HCM: nil},
				},
			},
		},
	}

	items := buildItems(snapshot)

	if len(items) != 0 {
		t.Errorf("expected 0 items for nil-HCM snapshot, got %d", len(items))
	}
}

// TestBuildItems_NilRouteConfig verifies that filter chains with a nil RouteConfig are skipped.
func TestBuildItems_NilRouteConfig(t *testing.T) {
	snapshot := &model.EnvoySnapshot{
		Listeners: []model.Listener{
			{
				Name: "listener~80",
				FilterChains: []model.NetworkFilterChain{
					{
						HCM: &model.HCMConfig{
							RouteConfigName: "listener~80",
							HTTPFilters:     []model.HTTPFilter{{Name: "envoy.filters.http.router"}},
							RouteConfig:     nil, // no route config
						},
					},
				},
			},
		},
	}

	items := buildItems(snapshot)

	if len(items) != 0 {
		t.Errorf("expected 0 items for nil-RouteConfig snapshot, got %d", len(items))
	}
}

// TestBuildItems_EmptySnapshot verifies that an empty snapshot produces no items.
func TestBuildItems_EmptySnapshot(t *testing.T) {
	items := buildItems(&model.EnvoySnapshot{})
	if len(items) != 0 {
		t.Errorf("expected 0 items for empty snapshot, got %d", len(items))
	}
}
```

- [ ] **Step 2: Run the new tests — they must FAIL**

```bash
go test ./internal/tui/... -v
```

Expected: FAIL — package `tui` does not exist yet.

- [ ] **Step 3: Create internal/tui/tui.go with the full implementation**

Write the complete `tui.go` file as shown above.

- [ ] **Step 4: Run the buildItems tests — they must PASS**

```bash
go test ./internal/tui/... -v
```

Expected: all 4 `TestBuildItems_*` tests PASS.

- [ ] **Step 5: Run the full test suite — no regressions**

```bash
go test ./...
```

Expected: all packages PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/tui.go internal/tui/tui_test.go
git commit -m "feat: implement internal/tui package with bubbletea model and buildItems"
```

---

## Task 5: Wire --interactive CLI flag

**Files:**
- Modify: `cmd/krp/main.go`

- [ ] **Step 1: Add the flag and branch**

In `main.go`, add the flag registration after the existing `--rule` flag:

```go
dump.Flags().BoolP("interactive", "i", false, "Launch the interactive TUI instead of printing static output")
```

In `runDump`, read the flag after the existing flag reads:

```go
interactive, _ := cmd.Flags().GetBool("interactive")
```

At the end of `runDump`, replace:

```go
fmt.Println(renderer.Render(snapshot))
return nil
```

with:

```go
if interactive {
    return tui.Run(snapshot)
}
fmt.Println(renderer.Render(snapshot))
return nil
```

Add `"github.com/DuncanDoyle/krp/internal/tui"` to the imports.

Update the package doc comment at the top of `main.go` to add a usage example for `--interactive`:

```go
//	krp dump --file <path> --interactive                                                   # interactive TUI
//	krp dump --gateway <name> -n <ns> --interactive                                       # live fetch + interactive TUI
```

- [ ] **Step 2: Build and verify it compiles**

```bash
go build ./cmd/krp/...
```

Expected: binary produced, no errors.

- [ ] **Step 3: Smoke test — static mode still works**

```bash
./krp dump --file testdata/scenarios/01-simple/config_dump.json 2>/dev/null | head -5
```

Expected: the first few lines of the rendered output (listener header, filter chain, etc.).

- [ ] **Step 4: Smoke test — empty items warning**

```bash
./krp dump --file testdata/scenarios/01-simple/config_dump.json --interactive 2>&1 | grep "no expandable"
```

If the 01-simple config has no expandable filters (i.e. no TypedConfig on any filter), this should print `"no expandable filters found"` and exit. If it does have filters, the TUI will launch — quit with `q`.

- [ ] **Step 5: Run the full test suite**

```bash
go test ./...
```

Expected: all packages PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/krp/main.go
git commit -m "feat: add --interactive/-i flag to krp dump for bubbletea TUI mode"
```

---

## Final Verification

- [ ] Run the complete test suite one last time:

```bash
go test ./... -count=1
```

Expected: all packages PASS with no cached results.

- [ ] Build the final binary:

```bash
go build -o krp ./cmd/krp/
```

- [ ] Run `krp dump --help` and verify `--interactive` appears in the flag list:

```bash
./krp dump --help
```

Expected: `--interactive` and `-i` are listed.
