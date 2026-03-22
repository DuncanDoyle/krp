package renderer_test

import (
	"strings"
	"testing"

	_ "github.com/charmbracelet/bubbles"
	_ "github.com/charmbracelet/bubbletea"
	"github.com/DuncanDoyle/krp/internal/model"
	"github.com/DuncanDoyle/krp/internal/renderer"
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

// TestRender_RoutePolicies_URLRewrite verifies that a route with a regex_rewrite
// (URLRewrite HTTPRouteFilter) is rendered with a "rewrite:" policy line.
// Covers issue #9.
func TestRender_RoutePolicies_URLRewrite(t *testing.T) {
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
												Match:   model.RouteMatch{PathSeparatedPrefix: "/api/v1"},
												Cluster: "kube_default_httpbin_8000",
												Rewrite: &model.RouteRewrite{
													RegexPattern: "^/api/v1(/.*)?$",
													Substitution: "/v1\\1",
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

	checks := []string{
		"Route Policies",
		"rewrite:",
		"^/api/v1(/.*)?$",
		"/v1\\1",
	}
	for _, s := range checks {
		if !strings.Contains(output, s) {
			t.Errorf("expected output to contain %q\nOutput:\n%s", s, output)
		}
	}
}

// TestRender_MatchTypes_PathSeparatedPrefix verifies that a route with a
// path_separated_prefix match is rendered with "(path-prefix)" label.
// Covers issue #10.
func TestRender_MatchTypes_PathSeparatedPrefix(t *testing.T) {
	snapshot := routeSnapshotWithMatch(model.RouteMatch{PathSeparatedPrefix: "/api/v1"})
	output := renderer.Render(snapshot)

	if !strings.Contains(output, "/api/v1") {
		t.Errorf("expected path value in output\nOutput:\n%s", output)
	}
	if !strings.Contains(output, "path-prefix") {
		t.Errorf("expected 'path-prefix' label in output\nOutput:\n%s", output)
	}
}

// TestRender_MatchTypes_Regex verifies that a route with a safe_regex path match
// is rendered with "(regex)" label.
// Covers issue #10.
func TestRender_MatchTypes_Regex(t *testing.T) {
	snapshot := routeSnapshotWithMatch(model.RouteMatch{Regex: "^/api/v[0-9]+/.*$"})
	output := renderer.Render(snapshot)

	if !strings.Contains(output, "^/api/v[0-9]+/.*$") {
		t.Errorf("expected regex value in output\nOutput:\n%s", output)
	}
	if !strings.Contains(output, "regex") {
		t.Errorf("expected 'regex' label in output\nOutput:\n%s", output)
	}
}

// TestRender_MatchTypes_HeaderExact verifies that an exact header match is rendered
// with "header(name=value)" notation.
// Covers issue #10.
func TestRender_MatchTypes_HeaderExact(t *testing.T) {
	snapshot := routeSnapshotWithMatch(model.RouteMatch{
		Prefix:  "/api",
		Headers: []model.HeaderMatch{{Name: "x-env", Value: "prod"}},
	})
	output := renderer.Render(snapshot)

	if !strings.Contains(output, "header(x-env=prod)") {
		t.Errorf("expected 'header(x-env=prod)' in output\nOutput:\n%s", output)
	}
}

// TestRender_MatchTypes_HeaderRegex verifies that a regex header match is rendered
// with "header(name~value)" notation.
// Covers issue #10.
func TestRender_MatchTypes_HeaderRegex(t *testing.T) {
	snapshot := routeSnapshotWithMatch(model.RouteMatch{
		Prefix:  "/api",
		Headers: []model.HeaderMatch{{Name: "x-env", Value: "prod.*", Regex: true}},
	})
	output := renderer.Render(snapshot)

	if !strings.Contains(output, "header(x-env~prod.*)") {
		t.Errorf("expected 'header(x-env~prod.*)' in output\nOutput:\n%s", output)
	}
}

// TestRender_MatchTypes_QueryParamExact verifies that an exact query parameter match
// is rendered with "query(name=value)" notation.
// Covers issue #10.
func TestRender_MatchTypes_QueryParamExact(t *testing.T) {
	snapshot := routeSnapshotWithMatch(model.RouteMatch{
		Prefix:      "/search",
		QueryParams: []model.QueryParamMatch{{Name: "q", Value: "hello"}},
	})
	output := renderer.Render(snapshot)

	if !strings.Contains(output, "query(q=hello)") {
		t.Errorf("expected 'query(q=hello)' in output\nOutput:\n%s", output)
	}
}

// TestRender_MatchTypes_QueryParamRegex verifies that a regex query parameter match
// is rendered with "query(name~value)" notation.
// Covers issue #10.
func TestRender_MatchTypes_QueryParamRegex(t *testing.T) {
	snapshot := routeSnapshotWithMatch(model.RouteMatch{
		Prefix:      "/search",
		QueryParams: []model.QueryParamMatch{{Name: "q", Value: "hel.*", Regex: true}},
	})
	output := renderer.Render(snapshot)

	if !strings.Contains(output, "query(q~hel.*)") {
		t.Errorf("expected 'query(q~hel.*)' in output\nOutput:\n%s", output)
	}
}

// routeSnapshotWithMatch constructs a minimal EnvoySnapshot with one route using the given match.
// Shared helper for match-type renderer tests.
func routeSnapshotWithMatch(match model.RouteMatch) *model.EnvoySnapshot {
	return &model.EnvoySnapshot{
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
												Match:   match,
												Cluster: "kube_default_httpbin_8000",
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
}
