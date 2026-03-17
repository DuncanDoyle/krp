// Package filter narrows an [model.EnvoySnapshot] to the routes that belong
// to a specific Kubernetes HTTPRoute.
//
// Kgateway encodes the HTTPRoute identity in each Envoy route name using the
// convention:
//
//	<listener>-route-<N>-httproute-<name>-<namespace>-<rule>-<backend>-matcher-<M>
//
// [Filter] uses substring matching on that segment to select routes without
// making any Kubernetes API calls. Listeners and virtual hosts that contain no
// matching routes are pruned from the returned snapshot.
//
// # Known limitation
//
// Both the HTTPRoute name and its namespace can contain dashes. If a name and
// namespace pair produces the same concatenated string as another pair (e.g.
// HTTPRoute "foo-bar" in namespace "baz" versus "foo" in "bar-baz"), the two
// routes are indistinguishable without a Kubernetes API call. In practice this
// ambiguity is extremely rare.
package filter

import (
	"fmt"
	"strings"

	"github.com/DuncanDoyle/krp/internal/model"
)

// FilterOptions controls which routes [Filter] retains from a snapshot.
//
// HTTPRouteName and HTTPRouteNamespace together identify a single HTTPRoute
// resource. They correspond to the metadata.name and metadata.namespace fields
// of the HTTPRoute and may differ from the Gateway name and namespace used for
// port-forwarding.
//
// RuleIndex narrows the selection to one rule within the HTTPRoute. Use -1
// (the zero value is not safe here; always set RuleIndex explicitly) to include
// routes from all rules.
type FilterOptions struct {
	HTTPRouteName      string // HTTPRoute resource name, e.g. "api-example-com"
	HTTPRouteNamespace string // HTTPRoute resource namespace, e.g. "default"
	RuleIndex          int    // zero-based rule index within the HTTPRoute; -1 means all rules
}

// Filter returns a new [model.EnvoySnapshot] that contains only the routes
// whose Envoy route name embeds the HTTPRoute identity described by opts.
//
// Pruning behaviour:
//   - Virtual hosts with no matching routes are removed.
//   - Filter chains with no matching virtual hosts keep their HCM (preserving
//     the HTTP filter pipeline for display) but have an empty VirtualHosts slice.
//   - Listeners where no filter chain retains any virtual hosts are removed.
//
// If opts.HTTPRouteName is empty, snapshot is returned unchanged.
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

// filterListener returns a copy of l containing only the virtual hosts that
// match opts, or nil if no filter chain in the listener has any matches.
func filterListener(l model.Listener, opts FilterOptions) *model.Listener {
	out := l
	out.FilterChains = nil
	for _, fc := range l.FilterChains {
		out.FilterChains = append(out.FilterChains, filterChain(fc, opts))
	}
	// Keep the listener only when at least one filter chain has matching VHs.
	for _, fc := range out.FilterChains {
		if fc.HCM != nil && fc.HCM.RouteConfig != nil && len(fc.HCM.RouteConfig.VirtualHosts) > 0 {
			return &out
		}
	}
	return nil
}

// filterChain returns a copy of fc whose RouteConfig contains only the virtual
// hosts that have matching routes. When no VHs match, the HCM is preserved
// (with an empty VirtualHosts slice) so callers can still display the HTTP
// filter pipeline.
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

// filterVirtualHost returns a copy of vh containing only the routes that match
// opts, or nil if no routes match.
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
// identity from opts, using substring matching on the kgateway naming
// convention:
//
//	...-httproute-<name>-<namespace>-<rule_idx>-<backend_idx>-matcher-<matcher_idx>
//
// When opts.RuleIndex >= 0 the match is further narrowed to that rule index.
func routeMatches(name string, opts FilterOptions) bool {
	// Base marker: identifies the HTTPRoute by name and namespace.
	// Example: "httproute-api-example-com-default-"
	base := fmt.Sprintf("httproute-%s-%s-", opts.HTTPRouteName, opts.HTTPRouteNamespace)
	if !strings.Contains(name, base) {
		return false
	}
	// Optional rule filter: narrows to a specific zero-based rule index.
	// Example: "httproute-api-example-com-default-1-"
	if opts.RuleIndex >= 0 {
		ruleMarker := fmt.Sprintf("%s%d-", base, opts.RuleIndex)
		return strings.Contains(name, ruleMarker)
	}
	return true
}
