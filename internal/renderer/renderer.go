// Package renderer converts an [model.EnvoySnapshot] into a styled, human-readable
// terminal string using the lipgloss library. The output is a tree of:
//
//	Listener → FilterChain → HCM (with RDS reference)
//	  └─ VirtualHost → Route → HTTP Filters + Route Policies + Backend
//
// All rendering is pure (no I/O); callers print the returned string themselves.
package renderer

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DuncanDoyle/krp/internal/model"
	"github.com/charmbracelet/lipgloss"
)

var (
	// Listener panel border
	listenerStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			Padding(0, 1).
			MarginBottom(1)

	listenerTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("12")) // bright blue

	// Filter chain
	filterChainLabelStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("14")) // cyan

	tlsStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("11")) // yellow

	// Route config / VirtualHost
	vhStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("15")) // white

	domainStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")) // gray

	// HTTP filters
	filterStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("13")) // magenta

	disabledStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")). // gray
			Italic(true)

	// Backend cluster
	clusterStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("10")). // green
			Bold(true)

	// Route match
	matchStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("15")) // white

	// Warning/empty
	warningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("9")) // red

	// Tree characters
	treeT   = "├─"
	treeL   = "└─"
	treeI   = "│ "
	treeSpc = "  "
)

// Render produces a styled string representation of the EnvoySnapshot.
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

// renderListener renders a single listener and all its filter chains as a
// lipgloss-bordered panel. ctx carries cursor/expansion state for interactive
// mode; nil means static mode (identical output to pre-Phase-3).
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

// renderFilterChain appends the tree representation of a single NetworkFilterChain
// to b. idx is the zero-based position within the listener and is used for the
// display label (FilterChain[N]). isLast controls which tree connector character
// is used (└─ for the last item, ├─ for others). ctx carries cursor/expansion
// state; nil means static mode.
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

// renderHCMContent renders the HTTP filter pipeline and route tree for a single
// HCMConfig. indent is the prefix string accumulated from the parent tree nodes
// and is passed deeper with additional spacing at each level.
//
// The HCM-level HTTP filter list is shared across all routes; per-route filter
// overrides are expressed via each route's TypedPerFilterConfig, which is passed
// to [renderHTTPFilters] so disabled-at-HCM filters can be shown as active where
// the route opts in. ctx carries cursor/expansion state; nil means static mode.
func renderHCMContent(b *strings.Builder, hcm *model.HCMConfig, indent string, ctx *interactiveContext) {
	if hcm.RouteConfig == nil {
		b.WriteString(fmt.Sprintf("%s%s\n", indent, warningStyle.Render("[RDS not found: "+hcm.RouteConfigName+"]")))
		// No route context here, so no per-route filter config available.
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

// renderHTTPFilters renders the HCM-level HTTP filter pipeline for a specific route.
// typedPerFilterConfig is the route's per-filter config: if a filter is disabled at HCM
// level but has an entry here, it is actually active on this route and shown as enabled.
// ctx carries cursor/expansion state; nil means static mode (no behaviour change).
//
// In interactive mode, the item whose coordinates match ctx.cursor is highlighted
// with cursorStyle (ANSI reverse-video). Items in ctx.expanded have their typed
// config appended inline as indented JSON, with resolveFilterConfig selecting
// the per-route override over the HCM-level config.
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

// renderRoutePolicies renders route-level policy configurations that come from
// HTTPRouteFilters (header modifiers, request mirroring) rather than HCM filters.
func renderRoutePolicies(b *strings.Builder, route model.Route, indent string) {
	// Collect all policy lines so we can apply the correct tree prefix to the last one.
	type policyLine struct{ text string }
	var lines []policyLine

	for _, h := range route.RequestHeadersToAdd {
		lines = append(lines, policyLine{fmt.Sprintf("add-req-header: %s = %s",
			filterStyle.Render(h.Key), domainStyle.Render(h.Value))})
	}
	for _, h := range route.ResponseHeadersToAdd {
		lines = append(lines, policyLine{fmt.Sprintf("add-res-header: %s = %s",
			filterStyle.Render(h.Key), domainStyle.Render(h.Value))})
	}
	for _, name := range route.ResponseHeadersToRemove {
		lines = append(lines, policyLine{fmt.Sprintf("remove-res-header: %s",
			filterStyle.Render(name))})
	}
	for _, cluster := range route.MirrorClusters {
		lines = append(lines, policyLine{fmt.Sprintf("mirror: %s",
			clusterStyle.Render(cluster))})
	}
	// URLRewrite HTTPRouteFilter — expressed as regex_rewrite in Envoy route action
	if route.Rewrite != nil {
		lines = append(lines, policyLine{fmt.Sprintf("rewrite: %s → %s",
			filterStyle.Render(route.Rewrite.RegexPattern),
			domainStyle.Render(route.Rewrite.Substitution))})
	}

	if len(lines) == 0 {
		return
	}

	b.WriteString(fmt.Sprintf("%sRoute Policies:\n", indent))
	for i, line := range lines {
		pfx := treeT
		if i == len(lines)-1 {
			pfx = treeL
		}
		b.WriteString(fmt.Sprintf("%s%s %s\n", indent, pfx, line.text))
	}
}

// formatMatch converts a RouteMatch into a compact human-readable string.
// Multiple match conditions are joined with " + " (e.g. a route that matches
// on both a path prefix and a header). When no conditions are set the route
// matches all traffic and is displayed as "/".
//
// The distinction between prefix and path-prefix matters: path_separated_prefix
// (shown as "path-prefix") treats "/" as a segment boundary, so /api matches
// /api/v1 but not /api-v2. A plain prefix matches any byte prefix.
func formatMatch(m model.RouteMatch) string {
	var parts []string
	if m.Prefix != "" {
		parts = append(parts, m.Prefix+" (prefix)")
	}
	if m.PathSeparatedPrefix != "" {
		// path_separated_prefix behaves like prefix but treats / as a segment boundary
		parts = append(parts, m.PathSeparatedPrefix+" (path-prefix)")
	}
	if m.Path != "" {
		parts = append(parts, m.Path+" (exact)")
	}
	if m.Regex != "" {
		parts = append(parts, m.Regex+" (regex)")
	}
	for _, h := range m.Headers {
		if h.Regex {
			parts = append(parts, fmt.Sprintf("header(%s~%s)", h.Name, h.Value))
		} else {
			parts = append(parts, fmt.Sprintf("header(%s=%s)", h.Name, h.Value))
		}
	}
	for _, qp := range m.QueryParams {
		if qp.Regex {
			parts = append(parts, fmt.Sprintf("query(%s~%s)", qp.Name, qp.Value))
		} else {
			parts = append(parts, fmt.Sprintf("query(%s=%s)", qp.Name, qp.Value))
		}
	}
	if len(parts) == 0 {
		return "/"
	}
	return strings.Join(parts, " + ")
}
