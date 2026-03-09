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
		renderHTTPFilters(b, hcm.HTTPFilters, indent)
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

			// HTTP filters for this route
			renderHTTPFilters(b, hcm.HTTPFilters, filterIndent)

			// Backend cluster
			if route.Cluster != "" {
				b.WriteString(fmt.Sprintf("%sBackend: %s\n",
					filterIndent, clusterStyle.Render(route.Cluster)))
			}
		}
	}
}

func renderHTTPFilters(b *strings.Builder, filters []model.HTTPFilter, indent string) {
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

		label := filterStyle.Render(f.Name)
		if f.Disabled {
			label = disabledStyle.Render(f.Name + " (disabled)")
		}

		b.WriteString(fmt.Sprintf("%s%s [%d] %s\n", indent, prefix, i+1, label))
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
