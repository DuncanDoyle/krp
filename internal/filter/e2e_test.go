package filter_test

import (
	"os"
	"strings"
	"testing"

	"github.com/DuncanDoyle/krp/internal/filter"
	"github.com/DuncanDoyle/krp/internal/parser"
)

// testdataPath returns the path to a testdata file relative to the project root.
// Tests are run from the package directory, so two levels up reaches the root.
func testdataPath(scenario, file string) string {
	return "../../testdata/scenarios/" + scenario + "/" + file
}

// TestFilter_E2E_SimpleHTTP_Match verifies that filtering the 00-simple config
// dump by its sole HTTPRoute returns only routes that carry that HTTPRoute's
// identity in their name.
//
// The 00-simple scenario contains one HTTPRoute named "api-example-com" in
// namespace "default", which produces route names of the form:
//
//	listener~80~api_example_com-route-0-httproute-api-example-com-default-0-0-matcher-0
func TestFilter_E2E_SimpleHTTP_Match(t *testing.T) {
	data, err := os.ReadFile(testdataPath("00-simple", "envoy/config_dump.json"))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	result, err := parser.Parse(data)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	opts := filter.FilterOptions{
		HTTPRouteName:      "api-example-com",
		HTTPRouteNamespace: "default",
		RuleIndex:          -1,
	}
	filtered := filter.Filter(result.Snapshot, opts)

	if len(filtered.Listeners) == 0 {
		t.Fatal("expected at least one listener after filter")
	}
	// Every remaining route must embed the httproute identity marker.
	for _, l := range filtered.Listeners {
		for _, fc := range l.FilterChains {
			if fc.HCM == nil || fc.HCM.RouteConfig == nil {
				continue
			}
			for _, vh := range fc.HCM.RouteConfig.VirtualHosts {
				for _, r := range vh.Routes {
					if !strings.Contains(r.Name, "httproute-api-example-com-default-") {
						t.Errorf("unexpected route in filtered snapshot: %s", r.Name)
					}
				}
			}
		}
	}
}

// TestFilter_E2E_SimpleHTTP_NoMatch verifies that filtering by a name that does
// not exist in the config dump returns an empty snapshot.
func TestFilter_E2E_SimpleHTTP_NoMatch(t *testing.T) {
	data, err := os.ReadFile(testdataPath("00-simple", "envoy/config_dump.json"))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	result, err := parser.Parse(data)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	opts := filter.FilterOptions{
		HTTPRouteName:      "does-not-exist",
		HTTPRouteNamespace: "default",
		RuleIndex:          -1,
	}
	filtered := filter.Filter(result.Snapshot, opts)

	if len(filtered.Listeners) != 0 {
		t.Fatalf("expected empty snapshot, got %d listeners", len(filtered.Listeners))
	}
}
