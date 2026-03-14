package parser_test

import (
	"os"
	"testing"

	"github.com/DuncanDoyle/kfp/internal/parser"
)

// testdataPath returns the path to a testdata file relative to the project root.
// Tests are run from the package directory, so we need to go up to the project root.
func testdataPath(scenario, file string) string {
	return "../../testdata/scenarios/" + scenario + "/" + file
}

func TestParse_SimpleHTTP(t *testing.T) {
	data, err := os.ReadFile(testdataPath("00-simple", "envoy/config_dump.json"))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	snapshot, err := parser.Parse(data)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	// Should have at least one dynamic listener
	if len(snapshot.Listeners) == 0 {
		t.Fatal("expected at least one listener")
	}

	// Find listener~80
	var found bool
	for _, l := range snapshot.Listeners {
		if l.Name == "listener~80" {
			found = true

			if l.Address != "[::]:80" && l.Address != "0.0.0.0:80" {
				t.Logf("listener address: %s", l.Address)
			}

			if len(l.FilterChains) == 0 {
				t.Fatal("expected at least one filter chain")
			}

			fc := l.FilterChains[0]
			if fc.TLS != nil {
				t.Error("expected no TLS on HTTP listener")
			}

			if fc.HCM == nil {
				t.Fatal("expected HCM in filter chain")
			}

			if fc.HCM.RouteConfigName != "listener~80" {
				t.Errorf("expected route config name 'listener~80', got %q", fc.HCM.RouteConfigName)
			}

			// Should have at least the router filter
			if len(fc.HCM.HTTPFilters) == 0 {
				t.Error("expected at least one HTTP filter")
			}

			// RouteConfig should be joined from RDS
			if fc.HCM.RouteConfig == nil {
				t.Fatal("expected RouteConfig to be joined from RDS")
			}

			if len(fc.HCM.RouteConfig.VirtualHosts) == 0 {
				t.Fatal("expected at least one virtual host")
			}

			vh := fc.HCM.RouteConfig.VirtualHosts[0]
			if vh.Name != "listener~80~api_example_com" {
				t.Errorf("expected VH name 'listener~80~api_example_com', got %q", vh.Name)
			}

			if len(vh.Routes) == 0 {
				t.Fatal("expected at least one route")
			}

			route := vh.Routes[0]
			if route.Match.Prefix != "/" {
				t.Errorf("expected prefix '/', got %q", route.Match.Prefix)
			}
			if route.Cluster != "kube_httpbin_httpbin_8000" {
				t.Errorf("expected cluster 'kube_httpbin_httpbin_8000', got %q", route.Cluster)
			}
		}
	}
	if !found {
		t.Error("listener~80 not found in parsed snapshot")
	}
}

func TestParse_HTTPSWithSNI(t *testing.T) {
	data, err := os.ReadFile(testdataPath("02_1-single-policy", "envoy/config_dump.json"))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	snapshot, err := parser.Parse(data)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	for _, l := range snapshot.Listeners {
		if l.Name != "listener~443" {
			continue
		}

		// Should have two filter chains (api.example.com and developer.example.com)
		if len(l.FilterChains) < 2 {
			t.Fatalf("expected at least 2 filter chains, got %d", len(l.FilterChains))
		}

		// Check first filter chain (api.example.com)
		fc0 := l.FilterChains[0]
		if fc0.TLS == nil {
			t.Fatal("expected TLS context on HTTPS filter chain")
		}
		if len(fc0.TLS.SNIHosts) == 0 || fc0.TLS.SNIHosts[0] != "api.example.com" {
			t.Errorf("expected SNI host 'api.example.com', got %v", fc0.TLS.SNIHosts)
		}

		if fc0.HCM == nil {
			t.Fatal("expected HCM")
		}
		if fc0.HCM.RouteConfigName != "https-api" {
			t.Errorf("expected route config name 'https-api', got %q", fc0.HCM.RouteConfigName)
		}

		// Should have transformation filter + router
		if len(fc0.HCM.HTTPFilters) < 2 {
			t.Fatalf("expected at least 2 HTTP filters, got %d", len(fc0.HCM.HTTPFilters))
		}
		if fc0.HCM.HTTPFilters[0].Name != "io.solo.transformation" {
			t.Errorf("expected first filter 'io.solo.transformation', got %q", fc0.HCM.HTTPFilters[0].Name)
		}

		// RouteConfig should be joined
		if fc0.HCM.RouteConfig == nil {
			t.Fatal("expected RouteConfig joined from RDS for https-api")
		}

		return
	}

	t.Error("listener~443 not found")
}

func TestParse_ExtAuth(t *testing.T) {
	data, err := os.ReadFile(testdataPath("02_7-single-policy", "envoy/config_dump.json"))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	snapshot, err := parser.Parse(data)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	for _, l := range snapshot.Listeners {
		if l.Name != "listener~80" {
			continue
		}
		if len(l.FilterChains) == 0 || l.FilterChains[0].HCM == nil {
			t.Fatal("expected HCM in filter chain")
		}
		filters := l.FilterChains[0].HCM.HTTPFilters

		// ext_authz scenario should have multiple filters
		if len(filters) < 2 {
			t.Fatalf("expected at least 2 HTTP filters for ext_authz scenario, got %d", len(filters))
		}

		// Verify the router is the last filter
		lastFilter := filters[len(filters)-1]
		if lastFilter.Name != "envoy.filters.http.router" {
			t.Errorf("expected last filter to be router, got %q", lastFilter.Name)
		}

		return
	}

	t.Error("listener~80 not found")
}

func TestParse_RequestHeaderModifier(t *testing.T) {
	data, err := os.ReadFile(testdataPath("02_2-single-policy", "envoy/config_dump.json"))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	snapshot, err := parser.Parse(data)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	for _, l := range snapshot.Listeners {
		if l.Name != "listener~80" {
			continue
		}
		fc := l.FilterChains[0]
		if fc.HCM == nil || fc.HCM.RouteConfig == nil {
			t.Fatal("expected HCM with RouteConfig")
		}
		route := fc.HCM.RouteConfig.VirtualHosts[0].Routes[0]
		if len(route.RequestHeadersToAdd) == 0 {
			t.Fatal("expected RequestHeadersToAdd to be populated")
		}
		h := route.RequestHeadersToAdd[0]
		if h.Key != "x-holiday" || h.Value != "christmas" {
			t.Errorf("unexpected header: %s=%s", h.Key, h.Value)
		}
		return
	}
	t.Error("listener~80 not found")
}

func TestParse_ResponseHeaderModifier(t *testing.T) {
	data, err := os.ReadFile(testdataPath("02_3-single-policy", "envoy/config_dump.json"))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	snapshot, err := parser.Parse(data)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	for _, l := range snapshot.Listeners {
		if l.Name != "listener~80" {
			continue
		}
		route := l.FilterChains[0].HCM.RouteConfig.VirtualHosts[0].Routes[0]
		if len(route.ResponseHeadersToAdd) == 0 {
			t.Fatal("expected ResponseHeadersToAdd to be populated")
		}
		h := route.ResponseHeadersToAdd[0]
		if h.Key != "x-powered-by" || h.Value != "kgateway" {
			t.Errorf("unexpected header: %s=%s", h.Key, h.Value)
		}
		if len(route.ResponseHeadersToRemove) == 0 {
			t.Fatal("expected ResponseHeadersToRemove to be populated")
		}
		if route.ResponseHeadersToRemove[0] != "server" {
			t.Errorf("expected 'server' removed, got %q", route.ResponseHeadersToRemove[0])
		}
		return
	}
	t.Error("listener~80 not found")
}

func TestParse_RequestMirror(t *testing.T) {
	data, err := os.ReadFile(testdataPath("02_4-single-policy", "envoy/config_dump.json"))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	snapshot, err := parser.Parse(data)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	for _, l := range snapshot.Listeners {
		if l.Name != "listener~80" {
			continue
		}
		route := l.FilterChains[0].HCM.RouteConfig.VirtualHosts[0].Routes[0]
		if len(route.MirrorClusters) == 0 {
			t.Fatal("expected MirrorClusters to be populated")
		}
		if route.MirrorClusters[0] != "kube_httpbin_httpbin-mirror_8000" {
			t.Errorf("unexpected mirror cluster: %q", route.MirrorClusters[0])
		}
		return
	}
	t.Error("listener~80 not found")
}

func TestParse_URLRewrite(t *testing.T) {
	data, err := os.ReadFile(testdataPath("02_5-single-policy", "envoy/config_dump.json"))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	snapshot, err := parser.Parse(data)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	for _, l := range snapshot.Listeners {
		if l.Name != "listener~80" {
			continue
		}
		fc := l.FilterChains[0]
		if fc.HCM == nil || fc.HCM.RouteConfig == nil {
			t.Fatal("expected HCM with RouteConfig")
		}
		route := fc.HCM.RouteConfig.VirtualHosts[0].Routes[0]

		// path_separated_prefix is how kgateway represents a PathPrefix match in Envoy
		if route.Match.PathSeparatedPrefix == "" {
			t.Error("expected PathSeparatedPrefix to be populated for URLRewrite scenario")
		}
		if route.Match.PathSeparatedPrefix != "/api/v1" {
			t.Errorf("expected PathSeparatedPrefix '/api/v1', got %q", route.Match.PathSeparatedPrefix)
		}

		// regex_rewrite is how kgateway represents a URLRewrite HTTPRouteFilter in Envoy
		if route.Rewrite == nil {
			t.Fatal("expected Rewrite to be populated for URLRewrite scenario")
		}
		if route.Rewrite.Substitution != "/" {
			t.Errorf("expected rewrite substitution '/', got %q", route.Rewrite.Substitution)
		}
		return
	}
	t.Error("listener~80 not found")
}

func TestParse_CORSPolicy(t *testing.T) {
	data, err := os.ReadFile(testdataPath("02_6-single-policy", "envoy/config_dump.json"))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	snapshot, err := parser.Parse(data)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	for _, l := range snapshot.Listeners {
		if l.Name != "listener~80" {
			continue
		}
		route := l.FilterChains[0].HCM.RouteConfig.VirtualHosts[0].Routes[0]

		// CORS policy appears as typed_per_filter_config on the route
		if len(route.TypedPerFilterConfig) == 0 {
			t.Fatal("expected TypedPerFilterConfig to be populated for CORS scenario")
		}
		if _, ok := route.TypedPerFilterConfig["envoy.filters.http.cors"]; !ok {
			t.Error("expected 'envoy.filters.http.cors' entry in TypedPerFilterConfig")
		}
		return
	}
	t.Error("listener~80 not found")
}

func TestParse_RateLimit(t *testing.T) {
	data, err := os.ReadFile(testdataPath("02_8-single-policy", "envoy/config_dump.json"))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	snapshot, err := parser.Parse(data)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	for _, l := range snapshot.Listeners {
		if l.Name != "listener~80" {
			continue
		}
		route := l.FilterChains[0].HCM.RouteConfig.VirtualHosts[0].Routes[0]

		// Rate limit policy appears as typed_per_filter_config on the route
		if len(route.TypedPerFilterConfig) == 0 {
			t.Fatal("expected TypedPerFilterConfig to be populated for rate limit scenario")
		}
		// kgateway uses "ratelimit_ee/default" as the filter name key
		if _, ok := route.TypedPerFilterConfig["ratelimit_ee/default"]; !ok {
			t.Error("expected 'ratelimit_ee/default' entry in TypedPerFilterConfig")
		}
		return
	}
	t.Error("listener~80 not found")
}

func TestParse_MalformedJSON(t *testing.T) {
	_, err := parser.Parse([]byte("{invalid json"))
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestParse_EmptyConfigs(t *testing.T) {
	snapshot, err := parser.Parse([]byte(`{"configs": []}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(snapshot.Listeners) != 0 {
		t.Errorf("expected 0 listeners, got %d", len(snapshot.Listeners))
	}
}
