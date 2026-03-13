package renderer_test

import (
	"strings"
	"testing"

	"github.com/DuncanDoyle/kfp/internal/model"
	"github.com/DuncanDoyle/kfp/internal/renderer"
)

func TestRender_SimpleHTTP(t *testing.T) {
	snapshot := &model.EnvoySnapshot{
		Listeners: []model.Listener{
			{
				Name:    "listener~80",
				Address: "[::]:80",
				FilterChains: []model.NetworkFilterChain{
					{
						Name: "listener~80",
						HCM: &model.HCMConfig{
							RouteConfigName: "listener~80",
							HTTPFilters: []model.HTTPFilter{
								{Name: "envoy.filters.http.router"},
							},
							RouteConfig: &model.RouteConfig{
								Name: "listener~80",
								VirtualHosts: []model.VirtualHost{
									{
										Name:    "listener~80~api_example_com",
										Domains: []string{"api.example.com"},
										Routes: []model.Route{
											{
												Name:    "listener~80~api_example_com-route-0-httproute-api-example-com-default-0-0-matcher-0",
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

	output := renderer.Render(snapshot)

	checks := []string{
		"listener~80",
		"api.example.com",
		"envoy.filters.http.router",
		"kube_httpbin_httpbin_8000",
	}
	for _, s := range checks {
		if !strings.Contains(output, s) {
			t.Errorf("expected output to contain %q\nOutput:\n%s", s, output)
		}
	}
}

func TestRender_HTTPS_TwoFilterChains(t *testing.T) {
	snapshot := &model.EnvoySnapshot{
		Listeners: []model.Listener{
			{
				Name:    "listener~443",
				Address: "[::]:443",
				FilterChains: []model.NetworkFilterChain{
					{
						Name: "https-api",
						TLS:  &model.TLSContext{SNIHosts: []string{"api.example.com"}},
						HCM: &model.HCMConfig{
							RouteConfigName: "https-api",
							HTTPFilters: []model.HTTPFilter{
								{Name: "io.solo.transformation", Disabled: true},
								{Name: "envoy.filters.http.router"},
							},
							RouteConfig: &model.RouteConfig{
								Name: "https-api",
								VirtualHosts: []model.VirtualHost{
									{
										Name:    "https-api~api_example_com",
										Domains: []string{"api.example.com"},
										Routes: []model.Route{
											{
												Name:    "https-api~api_example_com-route-0",
												Match:   model.RouteMatch{Prefix: "/"},
												Cluster: "kube_httpbin_httpbin_8000",
											},
										},
									},
								},
							},
						},
					},
					{
						Name: "https-developer",
						TLS:  &model.TLSContext{SNIHosts: []string{"developer.example.com"}},
						HCM: &model.HCMConfig{
							RouteConfigName: "https-developer",
							HTTPFilters: []model.HTTPFilter{
								{Name: "io.solo.transformation", Disabled: true},
								{Name: "envoy.filters.http.router"},
							},
						},
					},
				},
			},
		},
	}

	output := renderer.Render(snapshot)

	checks := []string{
		"listener~443",
		"https-api",
		"https-developer",
		"api.example.com",
		"developer.example.com",
		"io.solo.transformation",
	}
	for _, s := range checks {
		if !strings.Contains(output, s) {
			t.Errorf("expected output to contain %q\nOutput:\n%s", s, output)
		}
	}
}

func TestRender_RoutePolicies_HeaderModifier(t *testing.T) {
	snapshot := &model.EnvoySnapshot{
		Listeners: []model.Listener{
			{
				Name:    "listener~80",
				Address: "[::]:80",
				FilterChains: []model.NetworkFilterChain{
					{
						HCM: &model.HCMConfig{
							RouteConfigName: "listener~80",
							HTTPFilters:     []model.HTTPFilter{{Name: "envoy.filters.http.router"}},
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
												RequestHeadersToAdd:     []model.HeaderOperation{{Key: "x-holiday", Value: "christmas"}},
												ResponseHeadersToAdd:    []model.HeaderOperation{{Key: "x-powered-by", Value: "kgateway"}},
												ResponseHeadersToRemove: []string{"server"},
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

	output := renderer.Render(snapshot)

	checks := []string{
		"Route Policies",
		"add-req-header",
		"x-holiday",
		"christmas",
		"add-res-header",
		"x-powered-by",
		"kgateway",
		"remove-res-header",
		"server",
	}
	for _, s := range checks {
		if !strings.Contains(output, s) {
			t.Errorf("expected output to contain %q\nOutput:\n%s", s, output)
		}
	}
}

func TestRender_RoutePolicies_Mirror(t *testing.T) {
	snapshot := &model.EnvoySnapshot{
		Listeners: []model.Listener{
			{
				Name:    "listener~80",
				Address: "[::]:80",
				FilterChains: []model.NetworkFilterChain{
					{
						HCM: &model.HCMConfig{
							RouteConfigName: "listener~80",
							HTTPFilters:     []model.HTTPFilter{{Name: "envoy.filters.http.router"}},
							RouteConfig: &model.RouteConfig{
								Name: "listener~80",
								VirtualHosts: []model.VirtualHost{
									{
										Name:    "vh",
										Domains: []string{"api.example.com"},
										Routes: []model.Route{
											{
												Match:          model.RouteMatch{Prefix: "/"},
												Cluster:        "kube_httpbin_httpbin_8000",
												MirrorClusters: []string{"kube_httpbin_httpbin-mirror_8000"},
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

	output := renderer.Render(snapshot)

	checks := []string{"Route Policies", "mirror", "kube_httpbin_httpbin-mirror_8000"}
	for _, s := range checks {
		if !strings.Contains(output, s) {
			t.Errorf("expected output to contain %q\nOutput:\n%s", s, output)
		}
	}
}

// TestRender_DisabledFilter_ActiveOnRoute verifies that a filter marked disabled at HCM
// level is NOT shown as "(disabled)" when the route has a typed_per_filter_config for it.
func TestRender_DisabledFilter_ActiveOnRoute(t *testing.T) {
	snapshot := &model.EnvoySnapshot{
		Listeners: []model.Listener{
			{
				Name:    "listener~443",
				Address: "[::]:443",
				FilterChains: []model.NetworkFilterChain{
					{
						HCM: &model.HCMConfig{
							RouteConfigName: "https-api",
							HTTPFilters: []model.HTTPFilter{
								{Name: "io.solo.transformation", Disabled: true},
								{Name: "envoy.filters.http.router"},
							},
							RouteConfig: &model.RouteConfig{
								Name: "https-api",
								VirtualHosts: []model.VirtualHost{
									{
										Name:    "vh",
										Domains: []string{"api.example.com"},
										Routes: []model.Route{
											{
												Match:   model.RouteMatch{Prefix: "/"},
												Cluster: "kube_httpbin_httpbin_8000",
												// Route has per-filter config: transformation is active here
												TypedPerFilterConfig: map[string]any{
													"io.solo.transformation": map[string]any{"@type": "type.googleapis.com/envoy.api.v2.filter.http.RouteTransformations"},
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

	output := renderer.Render(snapshot)

	if strings.Contains(output, "disabled") {
		t.Errorf("expected filter to NOT show as disabled when active via typed_per_filter_config\nOutput:\n%s", output)
	}
	if !strings.Contains(output, "io.solo.transformation") {
		t.Errorf("expected filter name to appear in output\nOutput:\n%s", output)
	}
}

func TestRender_EmptySnapshot(t *testing.T) {
	snapshot := &model.EnvoySnapshot{}
	output := renderer.Render(snapshot)
	if !strings.Contains(output, "No listeners") {
		t.Errorf("expected 'No listeners' message for empty snapshot, got:\n%s", output)
	}
}
