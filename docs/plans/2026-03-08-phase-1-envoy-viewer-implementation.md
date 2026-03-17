# Phase 1 — Envoy Config Viewer: Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a CLI that parses a raw Envoy config dump (from file or live port-forward) and renders the complete Envoy configuration structure as a rich terminal visualization.

**Architecture:** CLI (cobra) → Parser (JSON config dump → EnvoySnapshot) → Renderer (lipgloss panels). The parser handles three config dump sections (Listeners, Routes, Clusters) and joins them by RDS route_config_name.

**Tech Stack:** Go, cobra, client-go (port-forward), lipgloss

**Design doc:** `docs/plans/2026-03-08-phase-1-envoy-viewer-design.md`

---

### Task 1: Project Scaffolding

**Files:**
- Create: `go.mod`
- Create: `cmd/kfp/main.go`

**Step 1: Initialize Go module and add dependencies**

```bash
go mod init github.com/kgateway-dev/kfp
go get github.com/spf13/cobra@latest
go get github.com/charmbracelet/lipgloss@latest
go get k8s.io/client-go@latest
go get k8s.io/api@latest
go get k8s.io/apimachinery@latest
```

**Step 2: Create the CLI entrypoint**

Create `cmd/kfp/main.go`:

```go
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "krp",
		Short: "Kgateway filter chain printer — visualize Envoy config",
	}

	dump := &cobra.Command{
		Use:   "dump",
		Short: "Dump and visualize the Envoy filter chain configuration",
		RunE:  runDump,
	}

	// Input source flags (mutually exclusive)
	dump.Flags().String("file", "", "Path to an Envoy config_dump JSON file")
	dump.Flags().String("gateway", "", "Gateway name (fetches config via port-forward to gateway-proxy pod)")
	dump.Flags().StringP("namespace", "n", "default", "Namespace of the Gateway (used with --gateway)")
	dump.Flags().String("context", "", "Kubeconfig context (used with --gateway, default: current context)")

	root.AddCommand(dump)

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func runDump(cmd *cobra.Command, args []string) error {
	file, _ := cmd.Flags().GetString("file")
	gateway, _ := cmd.Flags().GetString("gateway")

	if file == "" && gateway == "" {
		return fmt.Errorf("specify either --file <path> or --gateway <name>")
	}
	if file != "" && gateway != "" {
		return fmt.Errorf("--file and --gateway are mutually exclusive")
	}

	fmt.Println("krp dump — not yet implemented")
	return nil
}
```

**Step 3: Verify it builds and runs**

```bash
go mod tidy
go run ./cmd/kfp dump --file foo.json
```

Expected: `krp dump — not yet implemented`

**Step 4: Commit**

```bash
git add go.mod go.sum cmd/kfp/main.go
git commit -m "feat: scaffold CLI with cobra dump command"
```

---

### Task 2: Data Model

**Files:**
- Create: `internal/model/envoy.go`
- Create: `internal/model/envoy_test.go`

**Step 1: Write the failing test**

Create `internal/model/envoy_test.go`:

```go
package model_test

import (
	"encoding/json"
	"testing"

	"github.com/kgateway-dev/kfp/internal/model"
)

func TestEnvoySnapshotJSONRoundtrip(t *testing.T) {
	snapshot := model.EnvoySnapshot{
		Listeners: []model.Listener{
			{
				Name:    "listener~80",
				Address: "0.0.0.0:80",
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

	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var got model.EnvoySnapshot
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if len(got.Listeners) != 1 {
		t.Fatalf("expected 1 listener, got %d", len(got.Listeners))
	}
	l := got.Listeners[0]
	if l.Name != "listener~80" {
		t.Errorf("expected listener name 'listener~80', got %q", l.Name)
	}
	if l.FilterChains[0].HCM.RouteConfig.VirtualHosts[0].Routes[0].Cluster != "kube_httpbin_httpbin_8000" {
		t.Error("cluster name mismatch after roundtrip")
	}
}

func TestEnvoySnapshotTLS(t *testing.T) {
	snapshot := model.EnvoySnapshot{
		Listeners: []model.Listener{
			{
				Name:    "listener~443",
				Address: "0.0.0.0:443",
				FilterChains: []model.NetworkFilterChain{
					{
						Name: "https-api",
						TLS: &model.TLSContext{
							SNIHosts: []string{"api.example.com"},
						},
						HCM: &model.HCMConfig{
							RouteConfigName: "https-api",
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

	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var got model.EnvoySnapshot
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	fc := got.Listeners[0].FilterChains[0]
	if fc.TLS == nil {
		t.Fatal("expected TLS context")
	}
	if fc.TLS.SNIHosts[0] != "api.example.com" {
		t.Errorf("expected SNI host 'api.example.com', got %q", fc.TLS.SNIHosts[0])
	}
	if !fc.HCM.HTTPFilters[0].Disabled {
		t.Error("expected first filter to be disabled")
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/model/... -v
```

Expected: FAIL — package does not exist.

**Step 3: Implement the model**

Create `internal/model/envoy.go`:

```go
package model

// EnvoySnapshot is the complete parsed Envoy configuration.
// Built by joining data from ListenersConfigDump, RoutesConfigDump, and ClustersConfigDump.
type EnvoySnapshot struct {
	Listeners []Listener `json:"listeners"`
}

// Listener represents an Envoy listener (e.g. listener~80, listener~443).
type Listener struct {
	Name         string               `json:"name"`
	Address      string               `json:"address"` // e.g. "0.0.0.0:80"
	FilterChains []NetworkFilterChain `json:"filterChains"`
}

// NetworkFilterChain is a network-level filter chain within a listener.
// For HTTPS listeners, there is typically one filter chain per SNI host.
type NetworkFilterChain struct {
	Name string      `json:"name"`
	TLS  *TLSContext `json:"tls,omitempty"` // nil for plaintext
	HCM  *HCMConfig  `json:"hcm,omitempty"` // extracted from the network filter list
}

// TLSContext holds TLS/SNI information for a filter chain.
type TLSContext struct {
	SNIHosts []string `json:"sniHosts"` // from filter_chain_match.server_names
}

// HCMConfig represents the HTTP Connection Manager configuration.
type HCMConfig struct {
	RouteConfigName string       `json:"routeConfigName"` // rds.route_config_name
	HTTPFilters     []HTTPFilter `json:"httpFilters"`      // the HTTP filter pipeline
	RouteConfig     *RouteConfig `json:"routeConfig,omitempty"` // joined from RDS section
}

// HTTPFilter is a single filter in the HTTP filter pipeline.
type HTTPFilter struct {
	Name        string         `json:"name"`                  // e.g. "io.solo.transformation"
	TypedConfig map[string]any `json:"typedConfig,omitempty"` // raw typed config (for Phase 2)
	Disabled    bool           `json:"disabled,omitempty"`    // filter disabled at HCM level, enabled per-route
}

// RouteConfig is an Envoy route configuration (from RDS).
type RouteConfig struct {
	Name         string        `json:"name"`
	VirtualHosts []VirtualHost `json:"virtualHosts"`
}

// VirtualHost is an Envoy virtual host within a route config.
type VirtualHost struct {
	Name    string   `json:"name"`    // e.g. "listener~80~api_example_com"
	Domains []string `json:"domains"` // e.g. ["api.example.com"]
	Routes  []Route  `json:"routes"`
}

// Route is an Envoy route within a virtual host.
type Route struct {
	Name                 string            `json:"name"`
	Match                RouteMatch        `json:"match"`
	Cluster              string            `json:"cluster"`              // backend cluster name
	TypedPerFilterConfig map[string]any    `json:"typedPerFilterConfig,omitempty"` // per-route filter config (Phase 2)
	Metadata             map[string]any    `json:"metadata,omitempty"`            // filter_metadata (Phase 4)
}

// RouteMatch describes what traffic a route matches.
type RouteMatch struct {
	Prefix  string        `json:"prefix,omitempty"`
	Path    string        `json:"path,omitempty"`
	Regex   string        `json:"regex,omitempty"`
	Headers []HeaderMatch `json:"headers,omitempty"`
}

// HeaderMatch is a header-based match condition on a route.
type HeaderMatch struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}
```

**Step 4: Run tests to verify they pass**

```bash
go test ./internal/model/... -v
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/model/envoy.go internal/model/envoy_test.go
git commit -m "feat: add EnvoySnapshot data model"
```

---

### Task 3: Config Dump Parser

**Files:**
- Create: `internal/parser/parser.go`
- Create: `internal/parser/parser_test.go`

This is the core of Phase 1. The parser reads the raw config dump JSON, extracts listeners/routes/clusters, and joins them into an `EnvoySnapshot`.

**Step 1: Write failing tests using real testdata fixtures**

Create `internal/parser/parser_test.go`:

```go
package parser_test

import (
	"os"
	"testing"

	"github.com/kgateway-dev/kfp/internal/parser"
)

// testdataPath returns the path to a testdata file relative to the project root.
// Tests are run from the package directory, so we need to go up to the project root.
func testdataPath(scenario, file string) string {
	return "../../testdata/scenarios/" + scenario + "/" + file
}

func TestParse_SimpleHTTP(t *testing.T) {
	data, err := os.ReadFile(testdataPath("01-simple", "envoy/config_dump.json"))
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

	// Find listener~443
	var listener443 *struct {
		l interface{}
	}
	_ = listener443

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
```

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/parser/... -v
```

Expected: FAIL — package does not exist.

**Step 3: Implement the parser**

Create `internal/parser/parser.go`:

```go
package parser

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kgateway-dev/kfp/internal/model"
)

// Parse takes raw Envoy /config_dump JSON bytes and returns an EnvoySnapshot.
// It joins listeners with their RDS route configs by matching route_config_name.
func Parse(data []byte) (*model.EnvoySnapshot, error) {
	var dump configDump
	if err := json.Unmarshal(data, &dump); err != nil {
		return nil, fmt.Errorf("parsing config dump JSON: %w", err)
	}

	// Parse each config section by @type
	var listeners []rawListener
	routeConfigs := map[string]*model.RouteConfig{} // keyed by name

	for _, raw := range dump.Configs {
		var typed typedConfig
		if err := json.Unmarshal(raw, &typed); err != nil {
			continue
		}

		switch typed.Type {
		case "type.googleapis.com/envoy.admin.v3.ListenersConfigDump":
			var ld listenersConfigDump
			if err := json.Unmarshal(raw, &ld); err != nil {
				continue
			}
			listeners = ld.DynamicListeners

		case "type.googleapis.com/envoy.admin.v3.RoutesConfigDump":
			var rd routesConfigDump
			if err := json.Unmarshal(raw, &rd); err != nil {
				continue
			}
			for _, drc := range rd.DynamicRouteConfigs {
				rc := parseRouteConfig(drc.RouteConfig)
				routeConfigs[rc.Name] = rc
			}
		}
	}

	// Build the EnvoySnapshot by joining listeners with route configs
	snapshot := &model.EnvoySnapshot{}
	for _, rl := range listeners {
		l := parseListener(rl, routeConfigs)
		snapshot.Listeners = append(snapshot.Listeners, l)
	}

	return snapshot, nil
}

// parseListener converts a raw dynamic listener into the model.Listener,
// joining each HCM to its route config via route_config_name.
func parseListener(rl rawListener, routeConfigs map[string]*model.RouteConfig) model.Listener {
	l := model.Listener{
		Name: rl.Name,
	}

	if rl.ActiveState.Listener.Address.SocketAddress.Address != "" {
		l.Address = fmt.Sprintf("%s:%d",
			rl.ActiveState.Listener.Address.SocketAddress.Address,
			rl.ActiveState.Listener.Address.SocketAddress.PortValue,
		)
	}

	for _, rfc := range rl.ActiveState.Listener.FilterChains {
		nfc := model.NetworkFilterChain{
			Name: rfc.Name,
		}

		// TLS context from filter_chain_match.server_names
		if len(rfc.FilterChainMatch.ServerNames) > 0 {
			nfc.TLS = &model.TLSContext{
				SNIHosts: rfc.FilterChainMatch.ServerNames,
			}
		}

		// Find the HCM in the network filters
		for _, nf := range rfc.Filters {
			if nf.Name != "envoy.filters.network.http_connection_manager" {
				continue
			}
			hcm := parseHCM(nf.TypedConfig)
			// Join with RDS route config
			if rc, ok := routeConfigs[hcm.RouteConfigName]; ok {
				hcm.RouteConfig = rc
			}
			nfc.HCM = hcm
		}

		l.FilterChains = append(l.FilterChains, nfc)
	}

	return l
}

// parseHCM extracts the HCM config from the raw typed_config JSON.
func parseHCM(raw json.RawMessage) *model.HCMConfig {
	var hcm rawHCM
	if err := json.Unmarshal(raw, &hcm); err != nil {
		return &model.HCMConfig{}
	}

	result := &model.HCMConfig{
		RouteConfigName: hcm.RDS.RouteConfigName,
	}

	for _, hf := range hcm.HTTPFilters {
		filter := model.HTTPFilter{
			Name:     hf.Name,
			Disabled: hf.Disabled,
		}
		// Store the raw typed config for later phases
		if len(hf.TypedConfig) > 0 {
			var tc map[string]any
			if err := json.Unmarshal(hf.TypedConfig, &tc); err == nil {
				filter.TypedConfig = tc
			}
		}
		result.HTTPFilters = append(result.HTTPFilters, filter)
	}

	return result
}

// parseRouteConfig converts a raw route config into the model.
func parseRouteConfig(raw rawRouteConfig) *model.RouteConfig {
	rc := &model.RouteConfig{
		Name: raw.Name,
	}

	for _, rvh := range raw.VirtualHosts {
		vh := model.VirtualHost{
			Name:    rvh.Name,
			Domains: rvh.Domains,
		}

		for _, rr := range rvh.Routes {
			route := model.Route{
				Name: rr.Name,
				Match: model.RouteMatch{
					Prefix: rr.Match.Prefix,
					Path:   rr.Match.Path,
				},
			}

			// Extract cluster from the route action
			if rr.Route.Cluster != "" {
				route.Cluster = rr.Route.Cluster
			}

			// Extract header matches
			for _, hm := range rr.Match.Headers {
				value := hm.StringMatch.Exact
				route.Match.Headers = append(route.Match.Headers, model.HeaderMatch{
					Name:  hm.Name,
					Value: value,
				})
			}

			// Store per-filter config and metadata for later phases
			if len(rr.TypedPerFilterConfig) > 0 {
				tpfc := map[string]any{}
				for k, v := range rr.TypedPerFilterConfig {
					var parsed any
					if err := json.Unmarshal(v, &parsed); err == nil {
						tpfc[k] = parsed
					}
				}
				route.TypedPerFilterConfig = tpfc
			}

			if rr.Metadata != nil && len(rr.Metadata.FilterMetadata) > 0 {
				meta := map[string]any{}
				for k, v := range rr.Metadata.FilterMetadata {
					var parsed any
					if err := json.Unmarshal(v, &parsed); err == nil {
						meta[k] = parsed
					}
				}
				route.Metadata = meta
			}

			vh.Routes = append(vh.Routes, route)
		}

		rc.VirtualHosts = append(rc.VirtualHosts, vh)
	}

	return rc
}

// --- Raw JSON structs matching the actual Envoy config dump format ---

type configDump struct {
	Configs []json.RawMessage `json:"configs"`
}

type typedConfig struct {
	Type string `json:"@type"`
}

// Listeners

type listenersConfigDump struct {
	DynamicListeners []rawListener `json:"dynamic_listeners"`
}

type rawListener struct {
	Name        string `json:"name"`
	ActiveState struct {
		Listener struct {
			Name    string `json:"name"`
			Address struct {
				SocketAddress struct {
					Address   string `json:"address"`
					PortValue int    `json:"port_value"`
				} `json:"socket_address"`
			} `json:"address"`
			FilterChains []rawFilterChain `json:"filter_chains"`
		} `json:"listener"`
	} `json:"active_state"`
}

type rawFilterChain struct {
	Name             string `json:"name"`
	FilterChainMatch struct {
		ServerNames []string `json:"server_names"`
	} `json:"filter_chain_match"`
	Filters []rawNetworkFilter `json:"filters"`
}

type rawNetworkFilter struct {
	Name        string          `json:"name"`
	TypedConfig json.RawMessage `json:"typed_config"`
}

// HCM

type rawHCM struct {
	RDS struct {
		RouteConfigName string `json:"route_config_name"`
	} `json:"rds"`
	HTTPFilters []rawHTTPFilter `json:"http_filters"`
}

type rawHTTPFilter struct {
	Name        string          `json:"name"`
	TypedConfig json.RawMessage `json:"typed_config"`
	Disabled    bool            `json:"disabled"`
}

// Routes

type routesConfigDump struct {
	DynamicRouteConfigs []struct {
		RouteConfig rawRouteConfig `json:"route_config"`
	} `json:"dynamic_route_configs"`
}

type rawRouteConfig struct {
	Name         string           `json:"name"`
	VirtualHosts []rawVirtualHost `json:"virtual_hosts"`
}

type rawVirtualHost struct {
	Name    string     `json:"name"`
	Domains []string   `json:"domains"`
	Routes  []rawRoute `json:"routes"`
}

type rawRoute struct {
	Name  string `json:"name"`
	Match struct {
		Prefix  string `json:"prefix"`
		Path    string `json:"path"`
		Headers []struct {
			Name        string `json:"name"`
			StringMatch struct {
				Exact string `json:"exact"`
			} `json:"string_match"`
		} `json:"headers"`
	} `json:"match"`
	Route struct {
		Cluster string `json:"cluster"`
	} `json:"route"`
	TypedPerFilterConfig map[string]json.RawMessage `json:"typed_per_filter_config"`
	Metadata             *rawRouteMetadata          `json:"metadata"`
}

type rawRouteMetadata struct {
	FilterMetadata map[string]json.RawMessage `json:"filter_metadata"`
}

// formatAddress produces a human-readable address string.
func formatAddress(addr string, port int) string {
	return fmt.Sprintf("%s:%d", addr, port)
}

// Helper to avoid unused import
var _ = strings.Contains
```

**Step 4: Run tests to verify they pass**

```bash
go test ./internal/parser/... -v
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/parser/parser.go internal/parser/parser_test.go
git commit -m "feat: add Envoy config dump parser with RDS joining"
```

---

### Task 4: Port-Forwarder

**Files:**
- Create: `internal/envoy/portforward.go`
- Create: `internal/envoy/client.go`
- Create: `internal/envoy/client_test.go`

**Step 1: Write failing test for the admin client**

Create `internal/envoy/client_test.go`:

```go
package envoy_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kgateway-dev/kfp/internal/envoy"
)

func TestFetchConfigDump_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/config_dump" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"configs": []}`))
	}))
	defer srv.Close()

	data, err := envoy.FetchConfigDump(srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty config dump")
	}
}

func TestFetchConfigDump_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := envoy.FetchConfigDump(srv.URL)
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/envoy/... -v
```

Expected: FAIL — package does not exist.

**Step 3: Implement the admin client**

Create `internal/envoy/client.go`:

```go
package envoy

import (
	"fmt"
	"io"
	"net/http"
)

// FetchConfigDump fetches /config_dump from the given Envoy admin base URL
// and returns the raw JSON bytes.
func FetchConfigDump(baseURL string) ([]byte, error) {
	resp, err := http.Get(baseURL + "/config_dump")
	if err != nil {
		return nil, fmt.Errorf("GET /config_dump failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Envoy admin returned HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading config_dump response: %w", err)
	}
	return data, nil
}
```

**Step 4: Implement the port-forwarder**

Create `internal/envoy/portforward.go`:

```go
package envoy

import (
	"context"
	"fmt"
	"net"
	"net/http"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

const envoyAdminPort = 19000

// PortForwardResult holds the local address and a stop function.
type PortForwardResult struct {
	LocalAddr string // e.g. "http://localhost:12345"
	Stop      func()
}

// PortForwardToGateway finds a ready gateway-proxy pod and opens a port-forward
// to the Envoy admin port. Caller must call Stop() when done.
func PortForwardToGateway(ctx context.Context, gatewayName, namespace, kubeContext string) (*PortForwardResult, error) {
	cfg, err := buildRestConfig(kubeContext)
	if err != nil {
		return nil, err
	}

	kc, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating kubernetes client: %w", err)
	}

	podName, err := findGatewayProxyPod(ctx, kc, gatewayName, namespace)
	if err != nil {
		return nil, err
	}

	localPort, err := freePort()
	if err != nil {
		return nil, fmt.Errorf("finding free local port: %w", err)
	}

	stopCh := make(chan struct{})
	readyCh := make(chan struct{})
	errCh := make(chan error, 1)

	transport, upgrader, err := spdy.RoundTripperFor(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating SPDY transport: %w", err)
	}

	url := kc.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(namespace).
		Name(podName).
		SubResource("portforward").URL()

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, "POST", url)
	ports := []string{fmt.Sprintf("%d:%d", localPort, envoyAdminPort)}

	fw, err := portforward.New(dialer, ports, stopCh, readyCh, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("creating port-forwarder: %w", err)
	}

	go func() {
		errCh <- fw.ForwardPorts()
	}()

	select {
	case <-readyCh:
	case err := <-errCh:
		return nil, fmt.Errorf("port-forward failed: %w", err)
	case <-ctx.Done():
		close(stopCh)
		return nil, ctx.Err()
	}

	return &PortForwardResult{
		LocalAddr: fmt.Sprintf("http://localhost:%d", localPort),
		Stop:      func() { close(stopCh) },
	}, nil
}

func buildRestConfig(kubeContext string) (*rest.Config, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	overrides := &clientcmd.ConfigOverrides{}
	if kubeContext != "" {
		overrides.CurrentContext = kubeContext
	}

	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules, overrides,
	).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("loading kubeconfig: %w", err)
	}
	return cfg, nil
}

// findGatewayProxyPod finds the first ready pod for a kgateway Gateway.
func findGatewayProxyPod(ctx context.Context, kc kubernetes.Interface, gatewayName, namespace string) (string, error) {
	selector := fmt.Sprintf("gateway.networking.k8s.io/gateway-name=%s", gatewayName)
	pods, err := kc.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return "", fmt.Errorf("listing gateway-proxy pods: %w", err)
	}

	for _, pod := range pods.Items {
		if isPodReady(&pod) {
			return pod.Name, nil
		}
	}

	return "", fmt.Errorf(
		"no ready gateway-proxy pod found for Gateway %s/%s (selector: %s)\n"+
			"Hint: ensure the gateway pod is running and you have RBAC permissions for 'get pods' and 'create pods/portforward' in namespace %s",
		namespace, gatewayName, selector, namespace,
	)
}

func isPodReady(pod *corev1.Pod) bool {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
```

**Step 5: Run tests to verify they pass**

```bash
go test ./internal/envoy/... -v
```

Expected: PASS (admin client tests pass; port-forwarder has no unit tests — requires live cluster)

**Step 6: Commit**

```bash
git add internal/envoy/client.go internal/envoy/client_test.go internal/envoy/portforward.go
git commit -m "feat: add Envoy admin client and port-forwarder"
```

---

### Task 5: Renderer

**Files:**
- Create: `internal/renderer/renderer.go`
- Create: `internal/renderer/renderer_test.go`

The renderer takes an `EnvoySnapshot` and produces a styled terminal output using lipgloss.

**Step 1: Write failing tests**

Create `internal/renderer/renderer_test.go`:

```go
package renderer_test

import (
	"strings"
	"testing"

	"github.com/kgateway-dev/kfp/internal/model"
	"github.com/kgateway-dev/kfp/internal/renderer"
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

func TestRender_EmptySnapshot(t *testing.T) {
	snapshot := &model.EnvoySnapshot{}
	output := renderer.Render(snapshot)
	if !strings.Contains(output, "No listeners") {
		t.Errorf("expected 'No listeners' message for empty snapshot, got:\n%s", output)
	}
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/renderer/... -v
```

Expected: FAIL — package does not exist.

**Step 3: Implement the renderer**

Create `internal/renderer/renderer.go`:

```go
package renderer

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/kgateway-dev/kfp/internal/model"
)

var (
	// Listener panel border
	listenerStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(0, 1).
		MarginBottom(1)

	listenerTitleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("12")) // bright blue

	// Filter chain
	filterChainLabelStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("14")) // cyan

	tlsStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("11")) // yellow

	// Route config / VirtualHost
	vhStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("15")) // white

	domainStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")) // gray

	// HTTP filters
	filterStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("13")) // magenta

	disabledStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")). // gray
		Italic(true)

	// Backend cluster
	clusterStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("10")). // green
		Bold(true)

	// Route match
	matchStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("15")) // white

	// Warning/empty
	warningStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("9")) // red

	// Tree characters
	treeT    = "├─"
	treeL    = "└─"
	treeI    = "│ "
	treeSpc  = "  "
)

// Render produces a styled string representation of the EnvoySnapshot.
func Render(snapshot *model.EnvoySnapshot) string {
	if len(snapshot.Listeners) == 0 {
		return warningStyle.Render("No listeners found in config dump.")
	}

	var panels []string
	for _, listener := range snapshot.Listeners {
		panels = append(panels, renderListener(listener))
	}

	return strings.Join(panels, "\n")
}

func renderListener(l model.Listener) string {
	var b strings.Builder

	// Title line
	title := listenerTitleStyle.Render(fmt.Sprintf("Listener: %s", l.Name))
	addr := domainStyle.Render(l.Address)
	b.WriteString(fmt.Sprintf("%s %s\n", title, addr))

	for i, fc := range l.FilterChains {
		isLast := i == len(l.FilterChains)-1
		renderFilterChain(&b, fc, i, isLast)
	}

	return listenerStyle.Render(b.String())
}

func renderFilterChain(b *strings.Builder, fc model.NetworkFilterChain, idx int, isLast bool) {
	prefix := treeT
	childPrefix := treeI
	if isLast {
		prefix = treeL
		childPrefix = treeSpc
	}

	// Filter chain label with optional TLS info
	label := filterChainLabelStyle.Render(fmt.Sprintf("FilterChain[%d]", idx))
	if fc.Name != "" {
		label += " " + domainStyle.Render(fc.Name)
	}
	if fc.TLS != nil && len(fc.TLS.SNIHosts) > 0 {
		label += " " + tlsStyle.Render(fmt.Sprintf("TLS: %s", strings.Join(fc.TLS.SNIHosts, ", ")))
	}
	b.WriteString(fmt.Sprintf("%s %s\n", prefix, label))

	if fc.HCM == nil {
		b.WriteString(fmt.Sprintf("%s  %s\n", childPrefix, warningStyle.Render("[no HCM]")))
		return
	}

	// HCM → RDS reference
	b.WriteString(fmt.Sprintf("%s  %s HCM %s RDS: %s\n",
		childPrefix, treeL, domainStyle.Render("→"), fc.HCM.RouteConfigName))

	renderHCMContent(b, fc.HCM, childPrefix+treeSpc+"  ")
}

func renderHCMContent(b *strings.Builder, hcm *model.HCMConfig, indent string) {
	if hcm.RouteConfig == nil {
		b.WriteString(fmt.Sprintf("%s%s\n", indent, warningStyle.Render("[RDS not found: "+hcm.RouteConfigName+"]")))
		renderHTTPFilters(b, hcm.HTTPFilters, indent)
		return
	}

	for i, vh := range hcm.RouteConfig.VirtualHosts {
		isLastVH := i == len(hcm.RouteConfig.VirtualHosts)-1
		vhPrefix := treeT
		vhChildPrefix := treeI
		if isLastVH {
			vhPrefix = treeL
			vhChildPrefix = treeSpc
		}

		domains := domainStyle.Render(fmt.Sprintf("[%s]", strings.Join(vh.Domains, ", ")))
		b.WriteString(fmt.Sprintf("%s%s VirtualHost: %s %s\n",
			indent, vhPrefix, vhStyle.Render(vh.Name), domains))

		routeIndent := indent + vhChildPrefix + "  "
		for j, route := range vh.Routes {
			isLastRoute := j == len(vh.Routes)-1
			routePrefix := treeT
			routeChildPrefix := treeI
			if isLastRoute {
				routePrefix = treeL
				routeChildPrefix = treeSpc
			}

			matchStr := formatMatch(route.Match)
			b.WriteString(fmt.Sprintf("%s%s Route: %s\n",
				routeIndent, routePrefix, matchStyle.Render(matchStr)))

			filterIndent := routeIndent + routeChildPrefix + "  "

			// HTTP filters
			renderHTTPFilters(b, hcm.HTTPFilters, filterIndent)

			// Backend cluster
			if route.Cluster != "" {
				b.WriteString(fmt.Sprintf("%sBackend: %s\n",
					filterIndent, clusterStyle.Render(route.Cluster)))
			}
		}
	}
}

func renderHTTPFilters(b *strings.Builder, filters []model.HTTPFilter, indent string) {
	if len(filters) == 0 {
		return
	}

	b.WriteString(fmt.Sprintf("%sHTTP Filters:\n", indent))
	for i, f := range filters {
		isLast := i == len(filters)-1
		prefix := treeT
		if isLast {
			prefix = treeL
		}

		label := filterStyle.Render(f.Name)
		if f.Disabled {
			label = disabledStyle.Render(f.Name + " (disabled)")
		}

		b.WriteString(fmt.Sprintf("%s%s [%d] %s\n", indent, prefix, i+1, label))
	}
}

func formatMatch(m model.RouteMatch) string {
	var parts []string
	if m.Prefix != "" {
		parts = append(parts, m.Prefix+" (prefix)")
	}
	if m.Path != "" {
		parts = append(parts, m.Path+" (exact)")
	}
	if m.Regex != "" {
		parts = append(parts, m.Regex+" (regex)")
	}
	for _, h := range m.Headers {
		parts = append(parts, fmt.Sprintf("header(%s=%s)", h.Name, h.Value))
	}
	if len(parts) == 0 {
		return "/"
	}
	return strings.Join(parts, " + ")
}
```

**Step 4: Run tests to verify they pass**

```bash
go test ./internal/renderer/... -v
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/renderer/renderer.go internal/renderer/renderer_test.go
git commit -m "feat: add lipgloss Envoy config renderer"
```

---

### Task 6: Wire the Pipeline

**Files:**
- Modify: `cmd/kfp/main.go`

**Step 1: Wire the dump command to parser + renderer**

Replace the `runDump` function in `cmd/kfp/main.go`:

```go
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/kgateway-dev/kfp/internal/envoy"
	"github.com/kgateway-dev/kfp/internal/parser"
	"github.com/kgateway-dev/kfp/internal/renderer"
	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "krp",
		Short: "Kgateway filter chain printer — visualize Envoy config",
	}

	dump := &cobra.Command{
		Use:   "dump",
		Short: "Dump and visualize the Envoy filter chain configuration",
		RunE:  runDump,
	}

	dump.Flags().String("file", "", "Path to an Envoy config_dump JSON file")
	dump.Flags().String("gateway", "", "Gateway name (fetches config via port-forward to gateway-proxy pod)")
	dump.Flags().StringP("namespace", "n", "default", "Namespace of the Gateway (used with --gateway)")
	dump.Flags().String("context", "", "Kubeconfig context (used with --gateway, default: current context)")

	root.AddCommand(dump)

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func runDump(cmd *cobra.Command, args []string) error {
	file, _ := cmd.Flags().GetString("file")
	gateway, _ := cmd.Flags().GetString("gateway")
	namespace, _ := cmd.Flags().GetString("namespace")
	kubeContext, _ := cmd.Flags().GetString("context")

	if file == "" && gateway == "" {
		return fmt.Errorf("specify either --file <path> or --gateway <name>")
	}
	if file != "" && gateway != "" {
		return fmt.Errorf("--file and --gateway are mutually exclusive")
	}

	// Get the raw config dump bytes
	var data []byte
	var err error

	if file != "" {
		data, err = os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("reading file %s: %w", file, err)
		}
	} else {
		// Port-forward to the gateway-proxy pod and fetch config dump
		fmt.Fprintln(os.Stderr, "Connecting to Envoy admin API...")
		ctx := context.Background()
		pf, err := envoy.PortForwardToGateway(ctx, gateway, namespace, kubeContext)
		if err != nil {
			return fmt.Errorf("cannot reach Envoy admin API: %w", err)
		}
		defer pf.Stop()

		data, err = envoy.FetchConfigDump(pf.LocalAddr)
		if err != nil {
			return fmt.Errorf("fetching config dump: %w", err)
		}
	}

	// Parse the config dump into an EnvoySnapshot
	snapshot, err := parser.Parse(data)
	if err != nil {
		return err
	}

	// Render and print
	fmt.Println(renderer.Render(snapshot))
	return nil
}
```

**Step 2: Build to verify it compiles**

```bash
go mod tidy
go build ./...
```

Expected: no errors.

**Step 3: Run all tests**

```bash
go test ./... -v
```

Expected: all PASS.

**Step 4: Test with a real fixture file**

```bash
go run ./cmd/kfp dump --file testdata/scenarios/01-simple/envoy/config_dump.json
```

Expected: rendered output showing listener~80 with its filter chain, VirtualHost, route, and cluster.

```bash
go run ./cmd/kfp dump --file testdata/scenarios/02_1-single-policy/envoy/config_dump.json
```

Expected: rendered output showing listener~443 with two filter chains (api.example.com and developer.example.com), each with transformation + router filters.

**Step 5: Commit**

```bash
git add cmd/kfp/main.go
git commit -m "feat: wire dump command to parser and renderer pipeline"
```

---

### Task 7: End-to-End Parser Test

**Files:**
- Create: `internal/parser/e2e_test.go`

An integration test that parses all available testdata fixtures and verifies the full pipeline (parse → render) doesn't panic or error.

**Step 1: Write the test**

Create `internal/parser/e2e_test.go`:

```go
package parser_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kgateway-dev/kfp/internal/parser"
	"github.com/kgateway-dev/kfp/internal/renderer"
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
			snapshot, err := parser.Parse(data)
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}

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
```

**Step 2: Run the test**

```bash
go test ./internal/parser/... -v -run TestParseAllScenarios
```

Expected: PASS for all scenarios that have a config_dump.json.

**Step 3: Commit**

```bash
git add internal/parser/e2e_test.go
git commit -m "test: add end-to-end parser test across all testdata scenarios"
```

---

### Task 8: Polish and README

**Files:**
- Create: `README.md`
- Create: `.gitignore`

**Step 1: Create .gitignore**

Create `.gitignore`:

```
# Go
/kfp
*.exe
*.test
*.out

# IDE
.idea/
.vscode/
*.swp
```

**Step 2: Create README**

Create `README.md`:

```markdown
# krp — Kgateway Filter Chain Printer

Visualizes the Envoy filter chain configuration for [Kgateway](https://github.com/kgateway-dev/kgateway) gateway proxies.

## Installation

```bash
go install github.com/kgateway-dev/kfp/cmd/kfp@latest
```

## Usage

### From a config dump file

```bash
krp dump --file path/to/config_dump.json
```

### From a live cluster

```bash
# Auto port-forwards to the gateway-proxy pod
krp dump --gateway gw -n kgateway-system

# With explicit kubeconfig context
krp dump --gateway gw -n kgateway-system --context my-cluster
```

### Getting a config dump

```bash
kubectl port-forward -n kgateway-system deploy/gw 19000:19000 &
curl -s localhost:19000/config_dump | jq . > config_dump.json
kill %1
```

## Requirements (live mode)

- A running Kubernetes cluster with Kgateway installed
- RBAC: `get pods` and `create pods/portforward` in the Gateway namespace
- Your kubeconfig pointing at the target cluster
```

**Step 3: Commit**

```bash
git add .gitignore README.md
git commit -m "docs: add README and .gitignore"
```

---

## Phase 1.1 — Patch Cycle

**Issues resolved:** [#1](https://github.com/DuncanDoyle/kfp/issues/1), [#2](https://github.com/DuncanDoyle/kfp/issues/2)

### Background: how Kgateway registers optional filters

Kgateway installs filters like `io.solo.transformation`, `extauth`, and `ratelimit` in the HCM `http_filters` list with `"disabled": true`. This registers the filter in the pipeline without activating it globally. Routes that need the filter opt-in via `typed_per_filter_config` — Envoy activates the filter only for those routes. This pattern avoids per-request overhead on routes where a policy is not configured.

### Fix 1 — Route-level policies (issue #1)

K8S Gateway API HTTPRouteFilters for header manipulation (`RequestHeaderModifier`, `ResponseHeaderModifier`) and traffic mirroring (`RequestMirror`) do not produce HCM-level filters. Instead they translate directly into fields on the Envoy route object:

| HTTPRouteFilter | Envoy route field |
|---|---|
| `RequestHeaderModifier` | `request_headers_to_add` |
| `ResponseHeaderModifier` | `response_headers_to_add`, `response_headers_to_remove` |
| `RequestMirror` | `route.request_mirror_policies` |

**Model changes (`internal/model/envoy.go`):**

Added to `Route`:
```go
MirrorClusters          []string          // from route.request_mirror_policies
RequestHeadersToAdd     []HeaderOperation // from request_headers_to_add
ResponseHeadersToAdd    []HeaderOperation // from response_headers_to_add
ResponseHeadersToRemove []string          // from response_headers_to_remove
```

New type:
```go
type HeaderOperation struct {
    Key   string
    Value string
}
```

**Parser changes (`internal/parser/parser.go`):**

Extended `rawRoute` to capture `request_headers_to_add`, `response_headers_to_add`, `response_headers_to_remove`, and `route.request_mirror_policies`. `parseRouteConfig` populates the new model fields from these.

**Renderer changes (`internal/renderer/renderer.go`):**

Added `renderRoutePolicies()`. When a route has any of the above fields set, it emits a "Route Policies" block between the HTTP Filters and Backend lines:

```
Route Policies:
├─ add-req-header: x-holiday = christmas
├─ add-res-header: x-powered-by = kgateway
├─ remove-res-header: server
└─ mirror: kube_httpbin_httpbin-mirror_8000
```

### Fix 2 — Disabled filter display (issue #2)

A filter with `Disabled: true` at HCM level is active on any route that has a matching entry in `typed_per_filter_config`. The original renderer did not check this and showed all `Disabled: true` filters as "(disabled)" regardless of route context.

**Renderer change:** `renderHTTPFilters` now accepts `typedPerFilterConfig map[string]any`. When rendering filters for a specific route, a filter is only shown as "(disabled)" if `f.Disabled == true` AND `typedPerFilterConfig[f.Name]` is nil. If the route has a per-filter config entry for it, the filter renders as active.

**Test coverage added:**
- `parser_test.go`: `TestParse_RequestHeaderModifier`, `TestParse_ResponseHeaderModifier`, `TestParse_RequestMirror`
- `renderer_test.go`: `TestRender_RoutePolicies_HeaderModifier`, `TestRender_RoutePolicies_Mirror`, `TestRender_DisabledFilter_ActiveOnRoute`
