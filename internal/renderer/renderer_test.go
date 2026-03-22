package renderer_test

import (
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/DuncanDoyle/krp/internal/model"
	"github.com/DuncanDoyle/krp/internal/renderer"
)

// TestMain forces ANSI color output for all renderer tests. Without this,
// lipgloss detects no TTY and strips all ANSI escape codes, making it
// impossible to verify that cursorStyle (Reverse) produces output distinct
// from filterStyle (Foreground). termenv.ANSI is sufficient — it emits the
// 4-bit ANSI sequences needed to distinguish reverse-video from foreground color.
func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.ANSI)
	os.Exit(m.Run())
}

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

// --- RenderInteractive tests ---

// interactiveSnapshot returns a minimal snapshot with one listener, one filter
// chain, one virtual host, one route, and two HTTP filters. The first filter
// has TypedConfig set (HCM-level config); the second has none.
func interactiveSnapshot() *model.EnvoySnapshot {
	return &model.EnvoySnapshot{
		Listeners: []model.Listener{
			{
				Name:    "listener~80",
				Address: "[::]:80",
				FilterChains: []model.NetworkFilterChain{
					{
						HCM: &model.HCMConfig{
							RouteConfigName: "listener~80",
							HTTPFilters: []model.HTTPFilter{
								{
									Name:        "io.solo.transformation",
									TypedConfig: map[string]any{"@type": "type.googleapis.com/envoy.api.v2.filter.http.RouteTransformations"},
								},
								{Name: "envoy.filters.http.router"},
							},
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

// TestRenderInteractive_NoOpts verifies that RenderInteractive with zero-value
// opts produces string-equal output to Render (same string, no ANSI differences).
func TestRenderInteractive_NoOpts(t *testing.T) {
	snapshot := interactiveSnapshot()
	static := renderer.Render(snapshot)
	interactive := renderer.RenderInteractive(snapshot, renderer.RenderOpts{})
	if static != interactive {
		t.Errorf("expected RenderInteractive with empty opts to equal Render output\nRender:\n%s\nRenderInteractive:\n%s", static, interactive)
	}
}

// TestRenderInteractive_CursorOnFirstFilter verifies that a cursor on the first
// filter changes the output (the filter name is still present but styled differently).
func TestRenderInteractive_CursorOnFirstFilter(t *testing.T) {
	snapshot := interactiveSnapshot()
	cursor := renderer.FilterRef{ListenerIdx: 0, FilterChainIdx: 0, VirtualHostIdx: 0, RouteIdx: 0, FilterIdx: 0}
	output := renderer.RenderInteractive(snapshot, renderer.RenderOpts{Cursor: &cursor})

	if !strings.Contains(output, "io.solo.transformation") {
		t.Errorf("expected filter name to still be present in output\nOutput:\n%s", output)
	}
	static := renderer.Render(snapshot)
	if output == static {
		t.Errorf("expected cursor output to differ from static output")
	}
}

// TestRenderInteractive_ExpandedPerRouteConfig verifies that a filter with per-route
// TypedPerFilterConfig shows that config inline when expanded.
func TestRenderInteractive_ExpandedPerRouteConfig(t *testing.T) {
	snapshot := &model.EnvoySnapshot{
		Listeners: []model.Listener{
			{
				Name:    "listener~80",
				Address: "[::]:80",
				FilterChains: []model.NetworkFilterChain{
					{
						HCM: &model.HCMConfig{
							RouteConfigName: "listener~80",
							HTTPFilters: []model.HTTPFilter{
								{Name: "io.solo.transformation", Disabled: true},
								{Name: "envoy.filters.http.router"},
							},
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
												TypedPerFilterConfig: map[string]any{
													"io.solo.transformation": map[string]any{
														"@type":        "type.googleapis.com/solo.transformation",
														"request_body": "passthrough",
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
		},
	}

	ref := renderer.FilterRef{ListenerIdx: 0, FilterChainIdx: 0, VirtualHostIdx: 0, RouteIdx: 0, FilterIdx: 0}
	output := renderer.RenderInteractive(snapshot, renderer.RenderOpts{
		Expanded: map[renderer.FilterRef]bool{ref: true},
	})

	// Per-route config takes precedence; its keys should appear in the output.
	if !strings.Contains(output, "request_body") {
		t.Errorf("expected per-route config key 'request_body' in output\nOutput:\n%s", output)
	}
	if !strings.Contains(output, "passthrough") {
		t.Errorf("expected per-route config value 'passthrough' in output\nOutput:\n%s", output)
	}
}

// TestRenderInteractive_ExpandedHCMFallback verifies that a filter with no per-route
// TypedPerFilterConfig falls back to showing HTTPFilter.TypedConfig when expanded.
func TestRenderInteractive_ExpandedHCMFallback(t *testing.T) {
	snapshot := interactiveSnapshot() // first filter has TypedConfig at HCM level
	ref := renderer.FilterRef{ListenerIdx: 0, FilterChainIdx: 0, VirtualHostIdx: 0, RouteIdx: 0, FilterIdx: 0}
	output := renderer.RenderInteractive(snapshot, renderer.RenderOpts{
		Expanded: map[renderer.FilterRef]bool{ref: true},
	})

	if !strings.Contains(output, "RouteTransformations") {
		t.Errorf("expected HCM-level TypedConfig key 'RouteTransformations' in output\nOutput:\n%s", output)
	}
}

// TestRenderInteractive_ExpandedNoConfig verifies that expanding a filter with no
// typed config at either level shows "(no typed config)".
func TestRenderInteractive_ExpandedNoConfig(t *testing.T) {
	snapshot := interactiveSnapshot() // second filter (envoy.filters.http.router) has no TypedConfig
	ref := renderer.FilterRef{ListenerIdx: 0, FilterChainIdx: 0, VirtualHostIdx: 0, RouteIdx: 0, FilterIdx: 1}
	output := renderer.RenderInteractive(snapshot, renderer.RenderOpts{
		Expanded: map[renderer.FilterRef]bool{ref: true},
	})

	if !strings.Contains(output, "(no typed config)") {
		t.Errorf("expected '(no typed config)' in output\nOutput:\n%s", output)
	}
}

// TestRenderInteractive_ExpandedEmptyMap verifies that an empty map TypedPerFilterConfig
// entry is treated as "has config" and renders as "{}".
func TestRenderInteractive_ExpandedEmptyMap(t *testing.T) {
	snapshot := &model.EnvoySnapshot{
		Listeners: []model.Listener{
			{
				Name:    "listener~80",
				Address: "[::]:80",
				FilterChains: []model.NetworkFilterChain{
					{
						HCM: &model.HCMConfig{
							RouteConfigName: "listener~80",
							HTTPFilters:     []model.HTTPFilter{{Name: "envoy.filters.http.cors", Disabled: true}},
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
												TypedPerFilterConfig: map[string]any{
													"envoy.filters.http.cors": map[string]any{}, // empty map
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

	ref := renderer.FilterRef{ListenerIdx: 0, FilterChainIdx: 0, VirtualHostIdx: 0, RouteIdx: 0, FilterIdx: 0}
	output := renderer.RenderInteractive(snapshot, renderer.RenderOpts{
		Expanded: map[renderer.FilterRef]bool{ref: true},
	})

	if !strings.Contains(output, "{}") {
		t.Errorf("expected '{}' for empty map config\nOutput:\n%s", output)
	}
}

// TestRenderInteractive_AllExpanded verifies that expanding all items shows
// config for all of them.
func TestRenderInteractive_AllExpanded(t *testing.T) {
	snapshot := interactiveSnapshot()
	expanded := map[renderer.FilterRef]bool{
		{ListenerIdx: 0, FilterChainIdx: 0, VirtualHostIdx: 0, RouteIdx: 0, FilterIdx: 0}: true,
		{ListenerIdx: 0, FilterChainIdx: 0, VirtualHostIdx: 0, RouteIdx: 0, FilterIdx: 1}: true,
	}
	output := renderer.RenderInteractive(snapshot, renderer.RenderOpts{Expanded: expanded})

	// First filter has HCM-level config; second has none.
	if !strings.Contains(output, "RouteTransformations") {
		t.Errorf("expected first filter's config in output\nOutput:\n%s", output)
	}
	if !strings.Contains(output, "(no typed config)") {
		t.Errorf("expected '(no typed config)' for second filter\nOutput:\n%s", output)
	}
}

// TestRenderInteractive_NilHCM verifies that a filter chain with a nil HCM does
// not crash and produces no cursor or expansion output.
func TestRenderInteractive_NilHCM(t *testing.T) {
	snapshot := &model.EnvoySnapshot{
		Listeners: []model.Listener{
			{
				Name:    "listener~80",
				Address: "[::]:80",
				FilterChains: []model.NetworkFilterChain{
					{Name: "no-hcm", HCM: nil}, // nil HCM — no navigable filters
				},
			},
		},
	}

	// Should not panic even with a cursor set.
	ref := renderer.FilterRef{}
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("RenderInteractive panicked on nil-HCM snapshot: %v", r)
		}
	}()
	output := renderer.RenderInteractive(snapshot, renderer.RenderOpts{Cursor: &ref})

	if !strings.Contains(output, "[no HCM]") {
		t.Errorf("expected '[no HCM]' in output for nil-HCM filter chain\nOutput:\n%s", output)
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
