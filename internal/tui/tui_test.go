package tui

import (
	"os"
	"strings"
	"testing"

	envoymodel "github.com/DuncanDoyle/krp/internal/model"
	"github.com/DuncanDoyle/krp/internal/renderer"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// TestMain forces ANSI color output for all tui tests. Without this, lipgloss
// detects no TTY and strips ANSI escape codes, causing findCursorLine to always
// return 0 (no reverse-video code in the output) and scrollToCursor to never
// scroll. termenv.ANSI emits the 4-bit sequences needed by findCursorLine.
func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.ANSI)
	os.Exit(m.Run())
}

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

// TestBuildItems_EmptyHTTPFilters verifies that a filter chain whose HCM has a
// non-nil RouteConfig and routes but an empty HTTPFilters slice produces no items.
// The inner loop over fc.HCM.HTTPFilters simply yields nothing, so buildItems
// must return an empty slice rather than panicking or emitting partial refs.
func TestBuildItems_EmptyHTTPFilters(t *testing.T) {
	snapshot := &envoymodel.EnvoySnapshot{
		Listeners: []envoymodel.Listener{
			{
				Name: "listener~80",
				FilterChains: []envoymodel.NetworkFilterChain{
					{
						HCM: &envoymodel.HCMConfig{
							RouteConfigName: "listener~80",
							HTTPFilters:     []envoymodel.HTTPFilter{}, // empty — not nil
							RouteConfig: &envoymodel.RouteConfig{
								VirtualHosts: []envoymodel.VirtualHost{
									{
										Routes: []envoymodel.Route{
											{Match: envoymodel.RouteMatch{Prefix: "/"}},
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

	if len(items) != 0 {
		t.Errorf("expected 0 items for snapshot with empty HTTPFilters, got %d: %v", len(items), items)
	}
}

// TestSetContent_CursorAtFirstItem_ResetsOffset is the regression test for
// issue #23: after the viewport has been scrolled down by navigating to a later
// item, returning to cursor item 0 must reset the viewport offset to 0 so that
// Listener/FilterChain/HCM headers above the first navigable filter are visible.
//
// The snapshot deliberately has many routes so that the rendered output is long
// enough for the viewport to scroll meaningfully (viewport height is set to 3
// to ensure scrolling occurs). The test first navigates to the last item to
// scroll the viewport down, then returns to item 0 and asserts YOffset == 0.
func TestSetContent_CursorAtFirstItem_ResetsOffset(t *testing.T) {
	// 10 routes × 1 filter = 10 items; rendered content will be ~25+ lines.
	routes := make([]envoymodel.Route, 10)
	for i := range routes {
		routes[i] = envoymodel.Route{Match: envoymodel.RouteMatch{Prefix: "/"}}
	}
	snapshot := &envoymodel.EnvoySnapshot{
		Listeners: []envoymodel.Listener{
			{
				Name: "listener~80",
				FilterChains: []envoymodel.NetworkFilterChain{
					{
						HCM: &envoymodel.HCMConfig{
							RouteConfigName: "listener~80",
							HTTPFilters:     []envoymodel.HTTPFilter{{Name: "envoy.filters.http.router"}},
							RouteConfig: &envoymodel.RouteConfig{
								VirtualHosts: []envoymodel.VirtualHost{{Routes: routes}},
							},
						},
					},
				},
			},
		},
	}

	items := buildItems(snapshot)
	vp := viewport.New(80, 3) // narrow viewport so navigation causes scrolling
	m := model{
		snapshot: snapshot,
		items:    items,
		cursor:   len(items) - 1, // start at last item
		expanded: make(map[renderer.FilterRef]bool),
		viewport: vp,
	}

	// Render with cursor at last item — the viewport should scroll down.
	m.setContent()
	if m.viewport.YOffset == 0 {
		t.Fatal("test precondition failed: viewport did not scroll down when cursor was at last item; increase snapshot size")
	}

	// Navigate back to the first item — setContent() must reset offset to 0.
	m.cursor = 0
	m.setContent()

	if m.viewport.YOffset != 0 {
		t.Errorf("setContent() with cursor=0: expected YOffset=0 after scrolling back up, got %d", m.viewport.YOffset)
	}
}

// TestScrollToCursor_CursorInViewport verifies that the viewport offset is not
// changed when the cursor line is already within the visible area.
// This is the regression test for issue #22: on first render the cursor sits a
// few lines into the content (after Listener/FilterChain headers), and the
// viewport must stay at offset 0 so those headers are visible and scrollable.
func TestScrollToCursor_CursorInViewport(t *testing.T) {
	m := model{
		viewport: viewport.New(80, 23),
	}
	// Cursor is at line 5, which is within the 23-line viewport (offset 0).
	m.scrollToCursor(5)
	if m.viewport.YOffset != 0 {
		t.Errorf("scrollToCursor(5): expected YOffset=0 (cursor already in view), got %d", m.viewport.YOffset)
	}
}

// TestScrollToCursor_CursorAboveViewport verifies that the viewport scrolls up
// when the cursor line is above the current top of the visible area.
func TestScrollToCursor_CursorAboveViewport(t *testing.T) {
	m := model{
		viewport: viewport.New(80, 10),
	}
	// Provide enough content for the viewport to allow scrolling.
	m.viewport.SetContent(strings.Repeat("line\n", 30))
	m.viewport.SetYOffset(10) // viewport currently shows lines 10–19
	m.scrollToCursor(5)       // cursor at line 5, above the visible top
	if m.viewport.YOffset != 5 {
		t.Errorf("scrollToCursor(5): expected YOffset=5, got %d", m.viewport.YOffset)
	}
}

// TestScrollToCursor_CursorBelowViewport verifies that the viewport scrolls down
// when the cursor line is below the bottom of the visible area.
func TestScrollToCursor_CursorBelowViewport(t *testing.T) {
	m := model{
		viewport: viewport.New(80, 10),
	}
	// Provide enough content for the viewport to allow scrolling (cursor at 15 needs ≥16 lines).
	m.viewport.SetContent(strings.Repeat("line\n", 30))
	// Viewport at offset 0 shows lines 0–9; cursor at line 15 is out of view.
	m.scrollToCursor(15)
	// Expected: SetYOffset(15 - 10 + 1) = 6 so cursor appears at the bottom of viewport.
	if m.viewport.YOffset != 6 {
		t.Errorf("scrollToCursor(15): expected YOffset=6, got %d", m.viewport.YOffset)
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
