package parser

import (
	"testing"

	"github.com/DuncanDoyle/krp/internal/model"
)

// TestCloneRouteConfig_MapsAreIndependent verifies that TypedPerFilterConfig and Metadata
// are deep-copied (new maps) so that mutations on the clone cannot affect the original,
// and two clones of the same source are independent of each other.
func TestCloneRouteConfig_MapsAreIndependent(t *testing.T) {
	original := &model.RouteConfig{
		Name: "test-rc",
		VirtualHosts: []model.VirtualHost{
			{
				Name:    "vh",
				Domains: []string{"example.com"},
				Routes: []model.Route{
					{
						Name:    "route-0",
						Match:   model.RouteMatch{Prefix: "/"},
						Cluster: "kube_default_svc_8080",
						TypedPerFilterConfig: map[string]any{
							"io.solo.transformation": map[string]any{"key": "value"},
						},
						Metadata: map[string]any{
							"merge.EKTP": []any{"solo.io/EKTP/default/my-policy"},
						},
					},
				},
			},
		},
	}

	clone1 := cloneRouteConfig(original)
	clone2 := cloneRouteConfig(original)

	// Mutate clone1 — should not affect original or clone2
	clone1.VirtualHosts[0].Routes[0].TypedPerFilterConfig["new-key"] = "new-val"
	clone1.VirtualHosts[0].Routes[0].Metadata["extra"] = "extra-val"

	if _, ok := original.VirtualHosts[0].Routes[0].TypedPerFilterConfig["new-key"]; ok {
		t.Error("mutating clone1.TypedPerFilterConfig leaked into original")
	}
	if _, ok := clone2.VirtualHosts[0].Routes[0].TypedPerFilterConfig["new-key"]; ok {
		t.Error("mutating clone1.TypedPerFilterConfig leaked into clone2")
	}
	if _, ok := original.VirtualHosts[0].Routes[0].Metadata["extra"]; ok {
		t.Error("mutating clone1.Metadata leaked into original")
	}
	if _, ok := clone2.VirtualHosts[0].Routes[0].Metadata["extra"]; ok {
		t.Error("mutating clone1.Metadata leaked into clone2")
	}
}

// TestCloneRouteConfig_NilMaps verifies that nil TypedPerFilterConfig and Metadata
// stay nil in the clone (no allocation for empty maps).
func TestCloneRouteConfig_NilMaps(t *testing.T) {
	original := &model.RouteConfig{
		Name: "test-rc",
		VirtualHosts: []model.VirtualHost{
			{
				Name: "vh",
				Routes: []model.Route{
					{
						Name:    "route-0",
						Match:   model.RouteMatch{Prefix: "/"},
						Cluster: "kube_default_svc_8080",
						// TypedPerFilterConfig and Metadata intentionally nil
					},
				},
			},
		},
	}

	clone := cloneRouteConfig(original)

	if clone.VirtualHosts[0].Routes[0].TypedPerFilterConfig != nil {
		t.Error("expected nil TypedPerFilterConfig in clone when original is nil")
	}
	if clone.VirtualHosts[0].Routes[0].Metadata != nil {
		t.Error("expected nil Metadata in clone when original is nil")
	}
}
