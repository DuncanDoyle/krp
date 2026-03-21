# Phase 3 — Filter Config Detail: Design

**Date:** 2026-03-21
**Status:** Approved

---

## Overview

Phase 3 adds the ability to inspect what each HTTP filter is configured to do. The static output mode (default) is preserved unchanged. A new `--interactive` / `-i` flag launches a bubbletea TUI where the user can navigate to any filter and expand its typed config inline as pretty-printed JSON.

The model and parser packages are not changed. The typed config data (`HTTPFilter.TypedConfig` and `Route.TypedPerFilterConfig`) is already parsed and available in the model from Phase 1/2.

---

## Architecture

The pipeline is unchanged. The only new branch is at the CLI output step:

```
Parser → [Route Filter] → renderer.Render(snapshot)       (default, static)
                        → tui.Run(snapshot)                (--interactive / -i)
```

**Packages affected:**
- `internal/renderer` — additive only: new `FilterRef`, `RenderOpts`, `RenderInteractive` in a new file; existing `renderer.go` is untouched.
- `internal/tui` — new package; owns all bubbletea state.
- `cmd/krp/main.go` — add `--interactive` / `-i` flag; branch on it in `runDump`.

**Packages unchanged:** `internal/model`, `internal/parser`, `internal/filter`, `internal/envoy`.

---

## Section 1 — Renderer changes

### New file: `internal/renderer/renderer_interactive.go`

#### `FilterRef`

Uniquely identifies one filter instance in the rendered tree. The Envoy model path is:

```
Listener[ListenerIdx]
  └─ FilterChain[FilterChainIdx]
       └─ HCM
            └─ RouteConfig
                 └─ VirtualHost[VirtualHostIdx]
                      └─ Route[RouteIdx]
                           └─ hcm.HTTPFilters[FilterIdx]
```

`FilterRef` represents one rendering of the HCM HTTP filter at `FilterIdx` in the specific context of the route at `RouteIdx`. The same HCM filter appears once per route in the output tree, so the full path is required to distinguish instances.

```go
// FilterRef uniquely identifies a single HTTP filter instance as rendered
// under a specific route. The Envoy path is:
// Listener[ListenerIdx] → FilterChain[FilterChainIdx] → HCM → RouteConfig →
// VirtualHost[VirtualHostIdx] → Route[RouteIdx] → hcm.HTTPFilters[FilterIdx].
type FilterRef struct {
    ListenerIdx    int
    FilterChainIdx int
    VirtualHostIdx int
    RouteIdx       int
    FilterIdx      int // zero-based index into hcm.HTTPFilters
}
```

#### `RenderOpts`

Carries interactive state for a single render pass:

```go
// RenderOpts carries the cursor position and expansion state for
// [RenderInteractive]. Both fields are optional: a nil Cursor means no item
// is highlighted; an empty or nil Expanded map means no items are expanded.
type RenderOpts struct {
    Cursor   *FilterRef
    Expanded map[FilterRef]bool
}
```

#### `RenderInteractive`

Signature:

```go
// RenderInteractive produces the same styled tree as [Render] with two
// additions driven by opts. It reuses the same internal render helpers
// (renderListener, renderFilterChain, renderHCMContent, renderHTTPFilters,
// renderRoutePolicies) so that the output with an empty RenderOpts is
// byte-for-byte identical to [Render]. The cursor highlight and inline
// typed-config expansion are injected only at the filter-name level inside
// renderHTTPFilters, keeping all other rendering logic shared.
//   - The item at opts.Cursor (if non-nil) is rendered with a cursorStyle
//     (lipgloss.NewStyle().Reverse(true)) so the user can see where the cursor is.
//   - For each item in opts.Expanded, the filter's typed config is printed
//     inline below the filter name as indented JSON.
//
// Config resolution for an expanded filter: the key used for lookup in
// Route.TypedPerFilterConfig is HTTPFilter.Name (e.g. "io.solo.transformation").
// This is the same key already used by [Render] to detect per-route activation
// of disabled-at-HCM filters. If Route.TypedPerFilterConfig[filter.Name] is
// non-nil it is shown (per-route override) — including an empty map {}, which
// is rendered as "{}"; otherwise HTTPFilter.TypedConfig (HCM-level config) is
// shown (same non-nil check). If neither is set, "(no typed config)" is printed.
//
// Inline JSON is formatted with json.MarshalIndent using a two-space indent
// and no line prefix (i.e. json.MarshalIndent(v, "", "  ")).
//
// RenderInteractive is a pure function — it performs no I/O and can be called
// from tests without starting a bubbletea program.
func RenderInteractive(snapshot *model.EnvoySnapshot, opts RenderOpts) string
```

`renderer.Render` remains the function it is today — no modifications.

---

## Section 2 — `internal/tui` package

### `internal/tui/tui.go`

#### Model

```go
// model holds all mutable state for the interactive TUI session.
type model struct {
    snapshot *envoymodel.EnvoySnapshot
    items    []renderer.FilterRef   // flat ordered list of all navigable filters
    cursor   int
    expanded map[renderer.FilterRef]bool
    viewport viewport.Model
}
```

`items` is populated once at model construction time by calling `buildItems(snapshot)`. Its signature:

```go
// buildItems returns the flat ordered list of all navigable FilterRefs in the
// snapshot, following the canonical traversal order defined below. It is called
// once when the TUI model is initialised and the result is stored in model.items.
func buildItems(snapshot *envoymodel.EnvoySnapshot) []renderer.FilterRef
```

The canonical traversal order — shared by both `buildItems` and `RenderInteractive` — is:

1. `snapshot.Listeners` by index (outer loop)
2. `listener.FilterChains` by index
3. Skip filter chains where `filterChain.HCM == nil` (same as `renderer.Render` — rendered as `[no HCM]`, no navigable filters)
4. Skip filter chains where `filterChain.HCM.RouteConfig == nil` (rendered as `[RDS not found]`, no routes)
5. `filterChain.HCM.RouteConfig.VirtualHosts` by index
6. `virtualHost.Routes` by index
7. `hcm.HTTPFilters` by index (innermost — one `FilterRef` emitted per filter per route)

This order is defined here as the canonical contract. Both `buildItems` and `RenderInteractive` must follow it so that `items[N]` always corresponds to the N-th filter rendered on screen.

#### Key bindings

| Key | Action |
|-----|--------|
| `↑` / `k` | Move cursor up |
| `↓` / `j` | Move cursor down |
| `Enter` / `Space` | Toggle expand/collapse current filter |
| `a` | Expand/collapse all: if `len(items) == 0`, no-op. If `len(expanded) == len(items)` (all expanded), collapse all (clear the map). Otherwise expand all. |
| `q` / `Ctrl+C` | Quit |

#### Viewport sizing

The model handles `tea.WindowSizeMsg` to set the viewport's width and height to the current terminal dimensions. This is required by `bubbles/viewport` — without it the viewport has zero height and displays nothing. On startup before the first `WindowSizeMsg` arrives, the viewport is initialised with a default size (e.g. 80×24).

#### Init and viewport content

`Init()` returns `nil` (no initial commands needed). The initial `viewport.SetContent` call is made in `Update()` when the first `tea.WindowSizeMsg` is received — this is the standard bubbletea pattern for viewport initialisation.

On every state-changing `Update()` (cursor move, expand toggle, window resize), `viewport.SetContent(renderer.RenderInteractive(snapshot, opts))` is called with the new opts before returning the updated model. `View()` does not call `RenderInteractive` directly; it only calls `viewport.View()`.

If `len(items) == 0`, `opts.Cursor` is set to `nil`. `RenderInteractive` is always called with a valid `*FilterRef` (i.e. `&items[cursor]` where `0 <= cursor < len(items)`) or a nil pointer — never a `FilterRef` pointing outside the snapshot. Since `items` is built from the same snapshot passed to `RenderInteractive`, and the snapshot is immutable during the TUI session, out-of-bounds cursor access cannot occur.

After `SetContent`, scroll to keep the cursor item visible: compute the approximate line offset of the cursor item in the rendered output and call `viewport.SetYOffset`. A reasonable approximation (e.g. cursor line ≈ `cursor * averageLinesPerItem`) is sufficient for Phase 3. `viewport.SetYOffset` clamps the offset to the content height internally, so passing an over-estimated value is safe and will not panic or produce incorrect output — the viewport will simply scroll to the end of the content.

#### View

`View()` returns `viewport.View()`. All content is set via `viewport.SetContent` in `Update`.

#### Public API

```go
// Run starts the interactive TUI for the given snapshot. It blocks until the
// user quits (q or Ctrl+C) and returns any bubbletea program error.
func Run(snapshot *envoymodel.EnvoySnapshot) error
```

---

## Section 3 — CLI changes

One new flag on the `dump` subcommand:

```
--interactive, -i    Launch the interactive TUI instead of printing static output
```

In `runDump`, after the parse + filter pipeline:

```go
if interactive {
    return tui.Run(snapshot)
}
fmt.Println(renderer.Render(snapshot))
return nil
```

The command doc comment at the top of `main.go` is updated to include the new flag in the usage examples.

---

## Section 4 — Testing

### `internal/renderer`

Extend `renderer_test.go` with unit tests for `RenderInteractive`:

| Test | What it covers |
|------|---------------|
| No cursor, empty expanded | Output equals `Render` output (regression guard) |
| Cursor on first filter | Highlighted item is present in output |
| Single item expanded — per-route config | Per-route JSON is shown inline |
| Single item expanded — HCM-level fallback | HCM-level JSON is shown when no per-route config |
| Single item expanded — no typed config | `(no typed config)` is shown |
| Single item expanded — empty map config `{}` | `{}` is shown (empty map is treated as "has config") |
| All expanded (`a` key effect) | All expandable items show their config |
| Filter chain with nil HCM | No crash; nil-HCM filter chains produce no items and are not rendered with cursor/expansion |

### `internal/tui`

Unit test `buildItems` against a known snapshot: verify count and ordering without starting the bubbletea program. Include a test case with a nil-HCM filter chain to verify it is skipped.

Full TUI integration tests are not included — bubbletea programs are not easily driven headlessly.

### Existing tests

All existing tests in `internal/renderer`, `internal/parser`, `internal/filter` remain unchanged.

---

## Dependencies

- `github.com/charmbracelet/bubbletea` **v1.x** — new direct dependency (minimum v1.0.0; compatible with lipgloss v1.1.0 and the `charmbracelet/x/*` packages already in `go.mod`).
- `github.com/charmbracelet/bubbles` **v0.21.x** — new direct dependency; provides `viewport.Model`. Minimum v0.21.0 — this is the first release that targets bubbletea v1.x (v0.20.x targets bubbletea v0.x and will produce a compile error when paired with bubbletea v1.x). Use `github.com/charmbracelet/bubbles/viewport`, not a separate module.

Both are part of the Charm ecosystem already partially in use (`lipgloss v1.1.0`). No unrelated new dependencies.

**Style variable naming:** `renderer_interactive.go` introduces a `cursorStyle` package-level variable. The implementer must verify it does not conflict with the existing style variables in `renderer.go` (listenerStyle, filterChainLabelStyle, tlsStyle, etc.). If a conflict exists, prefix with `interactive` (e.g. `interactiveCursorStyle`).

---

## Deferred

The following are explicitly out of scope for Phase 3:

- K8S resource correlation (Phase 4)
- Side-by-side Envoy config + K8S manifest view (Phase 5)
- `--json` output flag (Phase 5)
- Search within the TUI (Phase 5)
