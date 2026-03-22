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
