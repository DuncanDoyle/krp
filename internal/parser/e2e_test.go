package parser_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DuncanDoyle/kfp/internal/model"
	"github.com/DuncanDoyle/kfp/internal/parser"
	"github.com/DuncanDoyle/kfp/internal/renderer"
)

// TestParseAllScenarios walks testdata/scenarios/ and parses every config_dump.json found.
// This ensures the parser handles all real config dump variants without errors.
func TestParseAllScenarios(t *testing.T) {
	scenariosDir := "../../testdata/scenarios"

	entries, err := os.ReadDir(scenariosDir)
	if err != nil {
		t.Fatalf("reading scenarios dir: %v", err)
	}

	parsed := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dumpPath := filepath.Join(scenariosDir, entry.Name(), "envoy", "config_dump.json")
		data, err := os.ReadFile(dumpPath)
		if err != nil {
			// Skip scenarios without config dump
			continue
		}

		t.Run(entry.Name(), func(t *testing.T) {
			result, err := parser.Parse(data)
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}
			snapshot := result.Snapshot

			if len(snapshot.Listeners) == 0 {
				t.Error("expected at least one listener")
			}

			// Verify rendering doesn't panic
			output := renderer.Render(snapshot)
			if output == "" {
				t.Error("expected non-empty render output")
			}

			t.Logf("Scenario %s: %d listeners, render length %d", entry.Name(), len(snapshot.Listeners), len(output))
		})

		parsed++
	}

	if parsed == 0 {
		t.Fatal("no config_dump.json files found in testdata/scenarios/")
	}

	t.Logf("Successfully parsed %d scenarios", parsed)
}

// firstRoute returns the first route from the first virtual host of the first
// filter chain in listener~80. Fails the test if any step is missing.
func firstRoute(t *testing.T, snapshot *model.EnvoySnapshot) model.Route {
	t.Helper()
	for _, l := range snapshot.Listeners {
		if l.Name != "listener~80" {
			continue
		}
		if len(l.FilterChains) == 0 || l.FilterChains[0].HCM == nil {
			t.Fatal("expected HCM in filter chain")
		}
		rc := l.FilterChains[0].HCM.RouteConfig
		if rc == nil || len(rc.VirtualHosts) == 0 || len(rc.VirtualHosts[0].Routes) == 0 {
			t.Fatal("expected at least one route in virtual host")
		}
		return rc.VirtualHosts[0].Routes[0]
	}
	t.Fatal("listener~80 not found")
	return model.Route{}
}

// parseMatcherScenario loads the config dump for a 01_x-matchers scenario.
// It skips the test if the dump has not been collected yet.
func parseMatcherScenario(t *testing.T, scenario string) *model.EnvoySnapshot {
	t.Helper()
	dumpPath := filepath.Join("../../testdata/scenarios", scenario, "envoy/config_dump.json")
	data, err := os.ReadFile(dumpPath)
	if err != nil {
		t.Skipf("config dump not yet collected for %s (run setup.sh then collect): %v", scenario, err)
	}
	result, err := parser.Parse(data)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	return result.Snapshot
}

// TestMatcherScenario_01_1_PathPrefix verifies that a PathPrefix matcher
// is parsed as path_separated_prefix and that the URLRewrite filter is captured.
func TestMatcherScenario_01_1_PathPrefix(t *testing.T) {
	snapshot := parseMatcherScenario(t, "01_1-matchers")
	route := firstRoute(t, snapshot)

	// kgateway translates PathPrefix to path_separated_prefix in Envoy
	if route.Match.PathSeparatedPrefix != "/api/v1" {
		t.Errorf("expected PathSeparatedPrefix '/api/v1', got %q", route.Match.PathSeparatedPrefix)
	}
	if route.Match.Prefix != "" {
		t.Errorf("expected Prefix to be empty for PathPrefix scenario, got %q", route.Match.Prefix)
	}

	// URLRewrite filter should be captured as a regex_rewrite on the route action
	if route.Rewrite == nil {
		t.Fatal("expected Rewrite to be set for PathPrefix + URLRewrite scenario")
	}
	if route.Rewrite.Substitution != "/" {
		t.Errorf("expected rewrite substitution '/', got %q", route.Rewrite.Substitution)
	}
}

// TestMatcherScenario_01_2_ExactPath verifies that an Exact path matcher
// is parsed into route.Match.Path and that a ReplaceFullPath URLRewrite is captured.
func TestMatcherScenario_01_2_ExactPath(t *testing.T) {
	snapshot := parseMatcherScenario(t, "01_2-matchers")
	route := firstRoute(t, snapshot)

	if route.Match.Path != "/api/v1/users" {
		t.Errorf("expected exact path '/api/v1/users', got %q", route.Match.Path)
	}
	if route.Rewrite == nil {
		t.Fatal("expected Rewrite to be set for Exact + URLRewrite scenario")
	}
}

// TestMatcherScenario_01_3_RegexPath verifies that a RegularExpression path
// matcher is parsed into route.Match.Regex (from safe_regex in Envoy).
func TestMatcherScenario_01_3_RegexPath(t *testing.T) {
	snapshot := parseMatcherScenario(t, "01_3-matchers")
	route := firstRoute(t, snapshot)

	if route.Match.Regex == "" {
		t.Error("expected Regex to be set for RegularExpression path matcher")
	}
	if route.Rewrite == nil {
		t.Fatal("expected Rewrite to be set for Regex + URLRewrite scenario")
	}
}

// TestMatcherScenario_01_4_HeaderExact verifies that an Exact header matcher
// is captured alongside a PathPrefix match.
func TestMatcherScenario_01_4_HeaderExact(t *testing.T) {
	snapshot := parseMatcherScenario(t, "01_4-matchers")
	route := firstRoute(t, snapshot)

	if route.Match.PathSeparatedPrefix == "" {
		t.Error("expected PathSeparatedPrefix for PathPrefix + header scenario")
	}
	if len(route.Match.Headers) == 0 {
		t.Fatal("expected at least one header matcher")
	}
	h := route.Match.Headers[0]
	if h.Name != "x-api-version" {
		t.Errorf("expected header name 'x-api-version', got %q", h.Name)
	}
	if h.Value != "v1" {
		t.Errorf("expected header value 'v1', got %q", h.Value)
	}
	if h.Regex {
		t.Error("expected exact (non-regex) header match")
	}
}

// TestMatcherScenario_01_5_HeaderRegex verifies that a RegularExpression header
// matcher is captured with Regex=true.
func TestMatcherScenario_01_5_HeaderRegex(t *testing.T) {
	snapshot := parseMatcherScenario(t, "01_5-matchers")
	route := firstRoute(t, snapshot)

	if len(route.Match.Headers) == 0 {
		t.Fatal("expected at least one header matcher")
	}
	h := route.Match.Headers[0]
	if h.Name != "x-client-id" {
		t.Errorf("expected header name 'x-client-id', got %q", h.Name)
	}
	if !h.Regex {
		t.Error("expected regex header match (Regex=true)")
	}
	if h.Value == "" {
		t.Error("expected non-empty regex pattern in header value")
	}
}

// TestMatcherScenario_01_6_Method verifies that a Method matcher (GET) is
// captured as a :method header match in Envoy.
func TestMatcherScenario_01_6_Method(t *testing.T) {
	snapshot := parseMatcherScenario(t, "01_6-matchers")
	route := firstRoute(t, snapshot)

	// Gateway API method matchers translate to :method header matchers in Envoy
	var found bool
	for _, h := range route.Match.Headers {
		if h.Name == ":method" && h.Value == "GET" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected :method=GET header match, got headers: %+v", route.Match.Headers)
	}
}

// TestMatcherScenario_01_7_QueryParam verifies that an Exact query param matcher
// is captured in route.Match.QueryParams.
func TestMatcherScenario_01_7_QueryParam(t *testing.T) {
	snapshot := parseMatcherScenario(t, "01_7-matchers")
	route := firstRoute(t, snapshot)

	if len(route.Match.QueryParams) == 0 {
		t.Fatal("expected at least one query parameter matcher")
	}
	qp := route.Match.QueryParams[0]
	if qp.Name != "format" {
		t.Errorf("expected query param name 'format', got %q", qp.Name)
	}
	if qp.Value != "json" {
		t.Errorf("expected query param value 'json', got %q", qp.Value)
	}
	if qp.Regex {
		t.Error("expected exact (non-regex) query param match")
	}
}

// TestMatcherScenario_01_8_PathPrefixMultiHeader verifies that a combination of
// PathPrefix and two Exact header matchers is fully captured.
func TestMatcherScenario_01_8_PathPrefixMultiHeader(t *testing.T) {
	snapshot := parseMatcherScenario(t, "01_8-matchers")
	route := firstRoute(t, snapshot)

	if route.Match.PathSeparatedPrefix == "" {
		t.Error("expected PathSeparatedPrefix for PathPrefix + multi-header scenario")
	}
	if len(route.Match.Headers) < 2 {
		t.Fatalf("expected 2 header matchers, got %d", len(route.Match.Headers))
	}
}

// TestMatcherScenario_01_9_ExactMethodQueryParam verifies the three-way combination
// of Exact path + Method + QueryParam matchers.
func TestMatcherScenario_01_9_ExactMethodQueryParam(t *testing.T) {
	snapshot := parseMatcherScenario(t, "01_9-matchers")
	route := firstRoute(t, snapshot)

	if route.Match.Path == "" {
		t.Error("expected exact path match for 01_9 scenario")
	}

	var methodFound bool
	for _, h := range route.Match.Headers {
		if h.Name == ":method" && h.Value == "GET" {
			methodFound = true
		}
	}
	if !methodFound {
		t.Error("expected :method=GET header match")
	}

	if len(route.Match.QueryParams) == 0 {
		t.Error("expected at least one query param matcher")
	}
}
