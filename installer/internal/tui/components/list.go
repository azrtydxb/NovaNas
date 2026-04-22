// Package components provides reusable TUI building blocks.
package components

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Item is a generic labeled choice.
type Item struct {
	Label string
	Value string
	Hint  string
}

// List is a minimal selectable list. Arrow keys move; Enter picks.
type List struct {
	Items   []Item
	Cursor  int
	Picked  int // -1 until picked
	Title   string
	Multi   bool
	chosen  map[int]bool
}

// NewList returns a List.
func NewList(title string, items []Item, multi bool) *List {
	return &List{
		Title:  title,
		Items:  items,
		Picked: -1,
		Multi:  multi,
		chosen: map[int]bool{},
	}
}

// Update processes messages.
func (l *List) Update(msg tea.Msg) tea.Cmd {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil
	}
	switch km.String() {
	case "up", "k":
		if l.Cursor > 0 {
			l.Cursor--
		}
	case "down", "j":
		if l.Cursor < len(l.Items)-1 {
			l.Cursor++
		}
	case " ":
		if l.Multi {
			l.chosen[l.Cursor] = !l.chosen[l.Cursor]
		}
	case "enter":
		l.Picked = l.Cursor
	}
	return nil
}

// Chosen returns all indexes that are currently selected (multi-mode).
func (l *List) Chosen() []int {
	out := make([]int, 0, len(l.chosen))
	for i, v := range l.chosen {
		if v {
			out = append(out, i)
		}
	}
	return out
}

// Toggle lets callers programmatically select.
func (l *List) Toggle(idx int) {
	l.chosen[idx] = !l.chosen[idx]
}

// View renders the list.
func (l *List) View() string {
	var b strings.Builder
	if l.Title != "" {
		b.WriteString(lipgloss.NewStyle().Bold(true).Render(l.Title))
		b.WriteString("\n\n")
	}
	for i, it := range l.Items {
		prefix := "  "
		if i == l.Cursor {
			prefix = "> "
		}
		mark := ""
		if l.Multi {
			if l.chosen[i] {
				mark = "[x] "
			} else {
				mark = "[ ] "
			}
		}
		line := prefix + mark + it.Label
		if it.Hint != "" {
			line += "  " + lipgloss.NewStyle().Faint(true).Render(it.Hint)
		}
		if i == l.Cursor {
			line = lipgloss.NewStyle().Bold(true).Render(line)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}
