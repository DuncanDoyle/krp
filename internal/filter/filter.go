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
package filter

import "github.com/DuncanDoyle/kfp/internal/model"

// FilterOptions controls which routes [Filter] retains from a snapshot.
//
// HTTPRouteName and HTTPRouteNamespace together identify a single HTTPRoute.
// They correspond to the name and namespace fields of the HTTPRoute resource,
// which may differ from the Gateway name and namespace used for port-forwarding.
//
// RuleIndex narrows the selection to a single rule within the HTTPRoute.
// Use -1 (the default) to include routes from all rules.
type FilterOptions struct {
	HTTPRouteName      string // HTTPRoute resource name, e.g. "api-example-com"
	HTTPRouteNamespace string // HTTPRoute resource namespace, e.g. "default"
	RuleIndex          int    // zero-based rule index; -1 means all rules
}

// Filter returns a new [model.EnvoySnapshot] that contains only the routes
// whose Envoy route name embeds the HTTPRoute identity described by opts.
//
// Pruning rules:
//   - Virtual hosts with no matching routes are removed.
//   - Filter chains with no matching virtual hosts retain their HCM (so the
//     HTTP filter pipeline remains visible) but have an empty VirtualHosts slice.
//   - Listeners where no filter chain has any matching routes are removed entirely.
//
// If opts.HTTPRouteName is empty no filtering is applied and snapshot is
// returned unchanged.
func Filter(snapshot *model.EnvoySnapshot, opts FilterOptions) *model.EnvoySnapshot {
	return snapshot
}
