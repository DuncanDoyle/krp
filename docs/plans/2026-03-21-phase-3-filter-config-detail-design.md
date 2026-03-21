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

Uniquely identifies one filter instance in the rendered tree. Since the same HCM `http_filters` list is rendered once per route, a ref requires the full path through the tree:

```go
// FilterRef uniquely identifies a single HTTP filter instance as rendered
// under a specific route. The same HCM filter appears once per route in the
// output, so the full path (listener → filter chain → virtual host → route →
// filter index) is required to distinguish instances.
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

```go
// RenderInteractive produces the same styled tree as [Render] with two
// additions driven by opts:
//   - The item at opts.Cursor (if non-nil) is rendered with a highlight style
//     (reversed foreground/background) so the user can see where the cursor is.
//   - For each item in opts.Expanded, the filter's typed config is printed
//     inline below the filter name as indented JSON.
//
// Config resolution for an expanded filter: Route.TypedPerFilterConfig[name]
// is shown if present (per-route override); otherwise HTTPFilter.TypedConfig
// (HCM-level config) is shown. If neither is set, "(no typed config)" is printed.
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

`items` is built once at init by `buildItems`, which walks the snapshot in the same traversal order used by `RenderInteractive` (Listener → FilterChain → VirtualHost → Route → HTTPFilter). This guarantees that `items[0]` is the first filter visible on screen.

#### Key bindings

| Key | Action |
|-----|--------|
| `↑` / `k` | Move cursor up |
| `↓` / `j` | Move cursor down |
| `Enter` / `Space` | Toggle expand/collapse current filter |
| `a` | Toggle expand/collapse all filters |
| `q` / `Ctrl+C` | Quit |

#### View

`View()` calls `renderer.RenderInteractive(snapshot, RenderOpts{Cursor: &items[cursor], Expanded: expanded})` and passes the result into the bubbletea viewport for scrolling.

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
| All expanded (`a` key effect) | All expandable items show their config |

### `internal/tui`

Unit test `buildItems` against a known snapshot: verify count and ordering without starting the bubbletea program.

Full TUI integration tests are not included — bubbletea programs are not easily driven headlessly.

### Existing tests

All existing tests in `internal/renderer`, `internal/parser`, `internal/filter` remain unchanged.

---

## Dependencies

- `github.com/charmbracelet/bubbletea` — already used indirectly via lipgloss; must be added as a direct dependency.
- `github.com/charmbracelet/bubbles/viewport` — bubbletea viewport component for scrollable output.

Both are part of the Charm ecosystem already partially in use (`lipgloss`). No unrelated new dependencies.

---

## Deferred

The following are explicitly out of scope for Phase 3:

- K8S resource correlation (Phase 4)
- Side-by-side Envoy config + K8S manifest view (Phase 5)
- `--json` output flag (Phase 5)
- Search within the TUI (Phase 5)
