package tui

import (
	"testing"

	envoymodel "github.com/DuncanDoyle/krp/internal/model"
	"github.com/DuncanDoyle/krp/internal/renderer"
)

// TestBuildItems_SimpleSnapshot verifies that buildItems produces one FilterRef
// per filter per route, in the canonical traversal order.
func TestBuildItems_SimpleSnapshot(t *testing.T) {
	snapshot := &envoymodel.EnvoySnapshot{
		Listeners: []envoymodel.Listener{
			{
				Name: "listener~80",
				FilterChains: []envoymodel.NetworkFilterChain{
					{
						HCM: &envoymodel.HCMConfig{
							RouteConfigName: "listener~80",
							HTTPFilters: []envoymodel.HTTPFilter{
								{Name: "io.solo.transformation"},
								{Name: "envoy.filters.http.router"},
							},
							RouteConfig: &envoymodel.RouteConfig{
								VirtualHosts: []envoymodel.VirtualHost{
									{
										Routes: []envoymodel.Route{
											{Match: envoymodel.RouteMatch{Prefix: "/"}},
											{Match: envoymodel.RouteMatch{Prefix: "/api"}},
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

	items := buildItems(snapshot)

	// 2 routes × 2 filters = 4 items
	if len(items) != 4 {
		t.Fatalf("expected 4 items, got %d", len(items))
	}

	// Verify canonical order: route 0 filter 0, route 0 filter 1, route 1 filter 0, route 1 filter 1
	expected := []renderer.FilterRef{
		{ListenerIdx: 0, FilterChainIdx: 0, VirtualHostIdx: 0, RouteIdx: 0, FilterIdx: 0},
		{ListenerIdx: 0, FilterChainIdx: 0, VirtualHostIdx: 0, RouteIdx: 0, FilterIdx: 1},
		{ListenerIdx: 0, FilterChainIdx: 0, VirtualHostIdx: 0, RouteIdx: 1, FilterIdx: 0},
		{ListenerIdx: 0, FilterChainIdx: 0, VirtualHostIdx: 0, RouteIdx: 1, FilterIdx: 1},
	}
	for i, got := range items {
		if got != expected[i] {
			t.Errorf("items[%d]: got %+v, want %+v", i, got, expected[i])
		}
	}
}

// TestBuildItems_NilHCM verifies that filter chains with a nil HCM are skipped.
func TestBuildItems_NilHCM(t *testing.T) {
	snapshot := &envoymodel.EnvoySnapshot{
		Listeners: []envoymodel.Listener{
			{
				Name: "listener~80",
				FilterChains: []envoymodel.NetworkFilterChain{
					{Name: "no-hcm", HCM: nil},
				},
			},
		},
	}

	items := buildItems(snapshot)

	if len(items) != 0 {
		t.Errorf("expected 0 items for nil-HCM snapshot, got %d", len(items))
	}
}

// TestBuildItems_NilRouteConfig verifies that filter chains with a nil RouteConfig are skipped.
func TestBuildItems_NilRouteConfig(t *testing.T) {
	snapshot := &envoymodel.EnvoySnapshot{
		Listeners: []envoymodel.Listener{
			{
				Name: "listener~80",
				FilterChains: []envoymodel.NetworkFilterChain{
					{
						HCM: &envoymodel.HCMConfig{
							RouteConfigName: "listener~80",
							HTTPFilters:     []envoymodel.HTTPFilter{{Name: "envoy.filters.http.router"}},
							RouteConfig:     nil,
						},
					},
				},
			},
		},
	}

	items := buildItems(snapshot)

	if len(items) != 0 {
		t.Errorf("expected 0 items for nil-RouteConfig snapshot, got %d", len(items))
	}
}

// TestBuildItems_EmptySnapshot verifies that an empty snapshot produces no items.
func TestBuildItems_EmptySnapshot(t *testing.T) {
	items := buildItems(&envoymodel.EnvoySnapshot{})
	if len(items) != 0 {
		t.Errorf("expected 0 items for empty snapshot, got %d", len(items))
	}
}

// TestBuildItems_MultipleListeners verifies that ListenerIdx and FilterChainIdx
// are tracked correctly when the snapshot has more than one listener and more
// than one filter chain.
func TestBuildItems_MultipleListeners(t *testing.T) {
	hcm := func(name string) *envoymodel.HCMConfig {
		return &envoymodel.HCMConfig{
			RouteConfigName: name,
			HTTPFilters:     []envoymodel.HTTPFilter{{Name: "envoy.filters.http.router"}},
			RouteConfig: &envoymodel.RouteConfig{
				VirtualHosts: []envoymodel.VirtualHost{
					{Routes: []envoymodel.Route{{Match: envoymodel.RouteMatch{Prefix: "/"}}}},
				},
			},
		}
	}

	snapshot := &envoymodel.EnvoySnapshot{
		Listeners: []envoymodel.Listener{
			{
				Name: "listener~80",
				FilterChains: []envoymodel.NetworkFilterChain{
					{HCM: hcm("listener~80-fc0")},
					{HCM: hcm("listener~80-fc1")},
				},
			},
			{
				Name:         "listener~443",
				FilterChains: []envoymodel.NetworkFilterChain{{HCM: hcm("listener~443-fc0")}},
			},
		},
	}

	items := buildItems(snapshot)

	// 2 filter chains in listener 0 + 1 in listener 1 = 3 items (1 filter, 1 route each)
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}

	expected := []renderer.FilterRef{
		{ListenerIdx: 0, FilterChainIdx: 0, VirtualHostIdx: 0, RouteIdx: 0, FilterIdx: 0},
		{ListenerIdx: 0, FilterChainIdx: 1, VirtualHostIdx: 0, RouteIdx: 0, FilterIdx: 0},
		{ListenerIdx: 1, FilterChainIdx: 0, VirtualHostIdx: 0, RouteIdx: 0, FilterIdx: 0},
	}
	for i, got := range items {
		if got != expected[i] {
			t.Errorf("items[%d]: got %+v, want %+v", i, got, expected[i])
		}
	}
}

// TestFindCursorLine verifies that findCursorLine correctly locates the
// cursor line by counting newlines before the ANSI reverse-video code.
func TestFindCursorLine(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    int
	}{
		{
			name:    "cursor on first line",
			content: "\x1b[7mhighlighted\x1b[0m\nsecond line\n",
			want:    0,
		},
		{
			name:    "cursor on third line",
			content: "line 0\nline 1\n\x1b[7mhighlighted\x1b[0m\nline 3\n",
			want:    2,
		},
		{
			name:    "no cursor present",
			content: "line 0\nline 1\nline 2\n",
			want:    0,
		},
		{
			name:    "empty content",
			content: "",
			want:    0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findCursorLine(tt.content)
			if got != tt.want {
				t.Errorf("findCursorLine() = %d, want %d", got, tt.want)
			}
		})
	}
}
