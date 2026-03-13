package renderer

import (
	"fmt"
	"strings"

	"github.com/DuncanDoyle/kfp/internal/model"
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
		panels = append(panels, renderListener(listener))
	}

	return strings.Join(panels, "\n")
}

func renderListener(l model.Listener) string {
	var b strings.Builder

	// Title line
	title := listenerTitleStyle.Render(fmt.Sprintf("Listener: %s", l.Name))
	addr := domainStyle.Render(l.Address)
	b.WriteString(fmt.Sprintf("%s %s\n", title, addr))

	for i, fc := range l.FilterChains {
		isLast := i == len(l.FilterChains)-1
		renderFilterChain(&b, fc, i, isLast)
	}

	return listenerStyle.Render(b.String())
}

func renderFilterChain(b *strings.Builder, fc model.NetworkFilterChain, idx int, isLast bool) {
	prefix := treeT
	childPrefix := treeI
	if isLast {
		prefix = treeL
		childPrefix = treeSpc
	}

	// Filter chain label with optional TLS info
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

	// HCM → RDS reference
	b.WriteString(fmt.Sprintf("%s  %s HCM %s RDS: %s\n",
		childPrefix, treeL, domainStyle.Render("→"), fc.HCM.RouteConfigName))

	renderHCMContent(b, fc.HCM, childPrefix+treeSpc+"  ")
}

func renderHCMContent(b *strings.Builder, hcm *model.HCMConfig, indent string) {
	if hcm.RouteConfig == nil {
		b.WriteString(fmt.Sprintf("%s%s\n", indent, warningStyle.Render("[RDS not found: "+hcm.RouteConfigName+"]")))
		// No route context here, so no per-route filter config available
		renderHTTPFilters(b, hcm.HTTPFilters, nil, indent)
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

			matchStr := formatMatch(route.Match)
			b.WriteString(fmt.Sprintf("%s%s Route: %s\n",
				routeIndent, routePrefix, matchStyle.Render(matchStr)))

			filterIndent := routeIndent + routeChildPrefix + "  "

			// HTTP filters for this route (pass typed_per_filter_config so disabled-at-HCM
			// filters that are active on this route are not shown as disabled)
			renderHTTPFilters(b, hcm.HTTPFilters, route.TypedPerFilterConfig, filterIndent)

			// Route-level policies (HTTPRouteFilter header/mirror configs)
			renderRoutePolicies(b, route, filterIndent)

			// Backend cluster
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
func renderHTTPFilters(b *strings.Builder, filters []model.HTTPFilter, typedPerFilterConfig map[string]any, indent string) {
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

		// A filter disabled at HCM level may still be active on this route via
		// typed_per_filter_config — in that case, do not render it as disabled.
		activeOnRoute := typedPerFilterConfig != nil && typedPerFilterConfig[f.Name] != nil
		label := filterStyle.Render(f.Name)
		if f.Disabled && !activeOnRoute {
			label = disabledStyle.Render(f.Name + " (disabled)")
		}

		b.WriteString(fmt.Sprintf("%s%s [%d] %s\n", indent, prefix, i+1, label))
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

func formatMatch(m model.RouteMatch) string {
	var parts []string
	if m.Prefix != "" {
		parts = append(parts, m.Prefix+" (prefix)")
	}
	if m.Path != "" {
		parts = append(parts, m.Path+" (exact)")
	}
	if m.Regex != "" {
		parts = append(parts, m.Regex+" (regex)")
	}
	for _, h := range m.Headers {
		parts = append(parts, fmt.Sprintf("header(%s=%s)", h.Name, h.Value))
	}
	if len(parts) == 0 {
		return "/"
	}
	return strings.Join(parts, " + ")
}
