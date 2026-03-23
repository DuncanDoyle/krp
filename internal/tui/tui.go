// Package tui provides the interactive bubbletea TUI for krp.
// It wraps an [envoymodel.EnvoySnapshot] in a scrollable, navigable terminal
// UI where the user can expand HTTP filter typed configs inline.
package tui

import (
	"fmt"
	"os"
	"strings"

	envoymodel "github.com/DuncanDoyle/krp/internal/model"
	"github.com/DuncanDoyle/krp/internal/renderer"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	defaultWidth  = 80
	defaultHeight = 23 // 24 rows minus 1 for the help bar
)

var helpBarStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

const helpText = "↑/↓ navigate • Enter/Space expand • a toggle all • q quit"

// model holds all mutable state for the interactive TUI session.
type model struct {
	snapshot *envoymodel.EnvoySnapshot
	items    []renderer.FilterRef
	cursor   int
	expanded map[renderer.FilterRef]bool
	viewport viewport.Model
}

// buildItems returns the flat ordered list of all navigable FilterRefs in the
// snapshot. It follows the canonical traversal order so that items[N] always
// corresponds to the N-th filter rendered on screen by [renderer.RenderInteractive].
//
// Filter chains with a nil HCM or a nil RouteConfig are skipped — they produce
// no navigable filters (same as the renderer, which shows "[no HCM]" / "[RDS not found]").
func buildItems(snapshot *envoymodel.EnvoySnapshot) []renderer.FilterRef {
	var items []renderer.FilterRef
	for lIdx, listener := range snapshot.Listeners {
		for fcIdx, fc := range listener.FilterChains {
			if fc.HCM == nil || fc.HCM.RouteConfig == nil {
				continue
			}
			for vhIdx, vh := range fc.HCM.RouteConfig.VirtualHosts {
				for rIdx := range vh.Routes {
					for fIdx := range fc.HCM.HTTPFilters {
						items = append(items, renderer.FilterRef{
							ListenerIdx:    lIdx,
							FilterChainIdx: fcIdx,
							VirtualHostIdx: vhIdx,
							RouteIdx:       rIdx,
							FilterIdx:      fIdx,
						})
					}
				}
			}
		}
	}
	return items
}

// findCursorLine finds the line number of the cursor item in the rendered
// content by locating the ANSI reverse-video code emitted by cursorStyle.
// Returns 0 if no cursor item is present (no cursor set or ANSI disabled).
func findCursorLine(content string) int {
	const reverseCode = "\x1b[7m"
	idx := strings.Index(content, reverseCode)
	if idx < 0 {
		return 0
	}
	return strings.Count(content[:idx], "\n")
}

// renderOpts builds the RenderOpts for the current model state.
func (m model) renderOpts() renderer.RenderOpts {
	if len(m.items) == 0 {
		return renderer.RenderOpts{}
	}
	cursor := m.items[m.cursor]
	return renderer.RenderOpts{
		Cursor:   &cursor,
		Expanded: m.expanded,
	}
}

// scrollToCursor adjusts the viewport offset to keep cursorLine visible
// without moving the viewport when the cursor is already in view.
// If the cursor is above the top of the viewport it scrolls up; if it is
// below the bottom it scrolls down; otherwise the offset is left unchanged.
// This preserves access to content rendered above the first navigable item
// (e.g. Listener and FilterChain headers) that would otherwise be hidden if
// the viewport were unconditionally pinned to the cursor line.
func (m *model) scrollToCursor(cursorLine int) {
	if cursorLine < m.viewport.YOffset {
		m.viewport.SetYOffset(cursorLine)
	} else if cursorLine >= m.viewport.YOffset+m.viewport.Height {
		m.viewport.SetYOffset(cursorLine - m.viewport.Height + 1)
	}
}

// setContent re-renders the snapshot with the current interactive state and
// sets the viewport content. Called on every state change.
func (m *model) setContent() {
	content := renderer.RenderInteractive(m.snapshot, m.renderOpts())
	m.viewport.SetContent(content)
	m.scrollToCursor(findCursorLine(content))
}

// initialModel constructs the initial TUI model for the given snapshot.
// It calls buildItems once to populate the flat navigation list and initialises
// the viewport with default dimensions; the actual terminal size is applied when
// the first tea.WindowSizeMsg is received in Update.
func initialModel(snapshot *envoymodel.EnvoySnapshot) model {
	vp := viewport.New(defaultWidth, defaultHeight)
	m := model{
		snapshot: snapshot,
		items:    buildItems(snapshot),
		cursor:   0,
		expanded: make(map[renderer.FilterRef]bool),
		viewport: vp,
	}
	return m
}

// Init implements tea.Model. No initial commands are needed.
func (m model) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - 1 // reserve one line for the help bar
		m.setContent()
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m.setContent()
			}
			return m, nil

		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
				m.setContent()
			}
			return m, nil

		case "enter", " ":
			if len(m.items) > 0 {
				ref := m.items[m.cursor]
				if m.expanded[ref] {
					delete(m.expanded, ref)
				} else {
					m.expanded[ref] = true
				}
				m.setContent()
			}
			return m, nil

		case "a":
			if len(m.items) == 0 {
				return m, nil
			}
			if len(m.expanded) == len(m.items) {
				// All expanded — collapse all.
				m.expanded = make(map[renderer.FilterRef]bool)
			} else {
				// Some or none expanded — expand all.
				for _, ref := range m.items {
					m.expanded[ref] = true
				}
			}
			m.setContent()
			return m, nil
		}
	}

	// Forward remaining messages (e.g. mouse events) to the viewport.
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// View implements tea.Model.
func (m model) View() string {
	return m.viewport.View() + "\n" + helpBarStyle.Render(helpText)
}

// Run starts the interactive TUI for the given snapshot. It prints a warning
// to stderr and returns immediately (without launching the bubbletea program)
// if the snapshot contains no navigable filters. It blocks until the user quits
// (q or Ctrl+C) and returns any bubbletea program error.
func Run(snapshot *envoymodel.EnvoySnapshot) error {
	m := initialModel(snapshot)
	if len(m.items) == 0 {
		fmt.Fprintln(os.Stderr, "no expandable filters found")
		return nil
	}
	// Content is initialised in Update() when the first tea.WindowSizeMsg arrives.
	// This is the standard bubbletea pattern — do not call setContent() here.
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
