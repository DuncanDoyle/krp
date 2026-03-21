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
- `internal/renderer` — two changes:
  1. New file `renderer_interactive.go`: `FilterRef`, `RenderOpts`, `RenderInteractive`.
  2. `renderer.go`: `renderHTTPFilters` and `renderHCMContent` are refactored to accept an optional `*interactiveContext` (nil = static, byte-identical to current behaviour). `renderer.Render` is not modified.
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

The internal helpers `renderHTTPFilters` and `renderHCMContent` are refactored in `renderer.go` to accept an optional `*interactiveContext`:

```go
// interactiveContext carries cursor and expansion state into the internal
// render helpers. A nil pointer means static mode — behaviour is identical
// to the pre-Phase-3 code path.
//
// lineCount and cursorLine are mutable fields updated during rendering:
// renderHTTPFilters increments lineCount for every line it writes, and
// records cursorLine when it writes the cursor item's line. After rendering,
// RenderInteractive reads cursorLine to compute the exact viewport offset.
type interactiveContext struct {
    ref        FilterRef           // coordinates of the current filter being rendered
    cursor     *FilterRef          // nil = no cursor
    expanded   map[FilterRef]bool
    lineCount  int                 // running total of lines written so far
    cursorLine int                 // line index of the cursor item (set during rendering)
}
```

`Render` passes `nil` for the context, so its output is byte-for-byte identical to the pre-Phase-3 output. `RenderInteractive` builds an `interactiveContext` from `opts` and passes it through the call chain.

```go
// RenderInteractive produces the same styled tree as [Render] with two
// additions driven by opts. renderHTTPFilters and renderHCMContent are
// refactored to accept an optional *interactiveContext; passing nil
// (as Render does) preserves the existing byte-identical output.
// The cursor highlight and inline typed-config expansion are injected
// only at the filter-name level inside renderHTTPFilters.
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

`renderer.Render` is not modified. `renderHTTPFilters` and `renderHCMContent` gain the optional `*interactiveContext` parameter but their nil-context code paths are unchanged.

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

#### Help bar

A static one-line footer is rendered below the viewport in `View()`:

```
↑/↓ navigate • Enter/Space expand • a toggle all • q quit
```

The help bar is rendered with a dimmed style (e.g. `lipgloss.NewStyle().Foreground(lipgloss.Color("8"))`). It is always visible — no toggle needed.

#### Viewport sizing

The model handles `tea.WindowSizeMsg` to set the viewport's width and height. The viewport height is `termHeight - 1` (one line reserved for the help bar). Width is the full terminal width. This is required by `bubbles/viewport` — without explicit sizing the viewport has zero height and displays nothing. On startup before the first `WindowSizeMsg` arrives, the viewport is initialised with a default size of 80 wide × 23 tall (24 − 1 for the help bar).

#### Init and viewport content

`Init()` returns `nil` (no initial commands needed). The initial `viewport.SetContent` call is made in `Update()` when the first `tea.WindowSizeMsg` is received — this is the standard bubbletea pattern for viewport initialisation.

On every state-changing `Update()` (cursor move, expand toggle, window resize), `viewport.SetContent(renderer.RenderInteractive(snapshot, opts))` is called with the new opts before returning the updated model. `View()` does not call `RenderInteractive` directly; it only calls `viewport.View()`.

If `len(items) == 0`, `opts.Cursor` is set to `nil`. `RenderInteractive` is always called with a valid `*FilterRef` (i.e. `&items[cursor]` where `0 <= cursor < len(items)`) or a nil pointer — never a `FilterRef` pointing outside the snapshot. Since `items` is built from the same snapshot passed to `RenderInteractive`, and the snapshot is immutable during the TUI session, out-of-bounds cursor access cannot occur.

After `SetContent`, scroll to keep the cursor item visible by calling `viewport.SetYOffset(ctx.cursorLine)`. The cursor line is tracked exactly by `interactiveContext.cursorLine` during rendering — `renderHTTPFilters` increments `ctx.lineCount` for every line written (including multi-line inline JSON) and records `ctx.cursorLine` when the cursor item is written. This is exact regardless of how many items are expanded or how large their JSON output is. `viewport.SetYOffset` clamps to content height internally, so there is no risk of panic or out-of-bounds access.

#### View

`View()` returns `viewport.View() + "\n" + helpBar`, where `helpBar` is the static dimmed keybinding footer. All tree content is set via `viewport.SetContent` in `Update`.

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

`--interactive` composes naturally with `--route` / `--route-ns` / `--rule`: the HTTPRoute filter is applied to the snapshot before `tui.Run` is called, so the TUI shows only the filtered view.

If `buildItems` returns an empty slice (e.g. the filtered snapshot has no HCM filters), `tui.Run` prints a one-line warning to stderr — `"no expandable filters found"` — and returns without launching the bubbletea program. The CLI exits cleanly with no further output.

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

## Implementation Notes

- `HTTPFilter.TypedConfig` has a stale comment in `internal/model/envoy.go` (`// raw typed config (for Phase 2)`). Update it to `// raw typed config; displayed in interactive mode (Phase 3)` during implementation.

## Deferred

The following are explicitly out of scope for Phase 3:

- K8S resource correlation (Phase 4)
- Side-by-side Envoy config + K8S manifest view (Phase 5)
- `--json` output flag (Phase 5)
- Search within the TUI (Phase 5)
