package renderer

import (
	"strings"

	"github.com/DuncanDoyle/krp/internal/model"
	"github.com/charmbracelet/lipgloss"
)

// cursorStyle highlights the filter row that currently holds the navigation cursor.
// Reverse(true) emits the ANSI reverse-video escape (\x1b[7m), which the TUI
// cursor-line finder in Task 4 uses to locate the highlighted row.
var cursorStyle = lipgloss.NewStyle().Reverse(true)

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

// resolveFilterConfig returns the typed config to display for an expanded filter.
// It returns Route.TypedPerFilterConfig[filter.Name] if non-nil (per-route override),
// otherwise HTTPFilter.TypedConfig. Returns nil if neither is set.
//
// An empty map[string]any{} is NOT nil in Go, so a route that sets an empty
// per-route config entry is treated as "has config" and will render as "{}".
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
