package filter_test

import (
	"fmt"
	"testing"

	"github.com/DuncanDoyle/krp/internal/filter"
	"github.com/DuncanDoyle/krp/internal/model"
)

// routeName builds a realistic Envoy route name for test cases, following the
// kgateway convention:
//
//	listener~80~example-route-0-httproute-<name>-<ns>-<rule>-0-matcher-0
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
	// Two routes from the same HTTPRoute but belonging to different rules.
	// Filtering by rule 1 must return only that route.
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
	// Two virtual hosts in one listener: VH1 has a matching route, VH2 does not.
	// VH2 must be pruned; VH1 must be retained.
	snap := snapshotWithTwoVHs(
		routeName("api-example-com", "default", 0),                    // VH1: matches
		"listener~80~other-route-0-httproute-other-default-0-0-matcher-0", // VH2: does not match
	)
	opts := filter.FilterOptions{HTTPRouteName: "api-example-com", HTTPRouteNamespace: "default", RuleIndex: -1}
	got := filter.Filter(snap, opts)
	vhs := got.Listeners[0].FilterChains[0].HCM.RouteConfig.VirtualHosts
	if len(vhs) != 1 {
		t.Fatalf("expected 1 VH after pruning, got %d", len(vhs))
	}
}

func TestFilter_PrunesEmptyListener(t *testing.T) {
	// Two listeners: only listener~80 has a matching route.
	// listener~443 must be pruned entirely.
	snap := snapshotWithTwoListeners(
		routeName("api-example-com", "default", 0),                // listener~80: matches
		"listener~443~other-route-0-httproute-x-y-0-0-matcher-0", // listener~443: does not match
	)
	opts := filter.FilterOptions{HTTPRouteName: "api-example-com", HTTPRouteNamespace: "default", RuleIndex: -1}
	got := filter.Filter(snap, opts)
	if len(got.Listeners) != 1 {
		t.Fatalf("expected 1 listener after pruning, got %d", len(got.Listeners))
	}
}

func TestFilter_MultipleFilterChains_PartialMatch(t *testing.T) {
	// One listener with two filter chains: FC1 has a matching route, FC2 does not.
	// The listener must be retained. FC2 must be kept with an empty VirtualHosts
	// slice so the renderer can still display its HTTP filter pipeline.
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

	if len(got.Listeners) != 1 {
		t.Fatalf("expected listener to be retained, got %d listeners", len(got.Listeners))
	}
	if len(got.Listeners[0].FilterChains) != 2 {
		t.Fatalf("expected both filter chains present, got %d", len(got.Listeners[0].FilterChains))
	}
	fc1VHs := got.Listeners[0].FilterChains[0].HCM.RouteConfig.VirtualHosts
	fc2VHs := got.Listeners[0].FilterChains[1].HCM.RouteConfig.VirtualHosts
	if len(fc1VHs) != 1 {
		t.Errorf("FC1: expected 1 VH, got %d", len(fc1VHs))
	}
	if len(fc2VHs) != 0 {
		t.Errorf("FC2: expected 0 VHs (pruned), got %d", len(fc2VHs))
	}
}

// --- snapshot builder helpers ---

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
