package components

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// Input wraps bubbles/textinput with a label.
type Input struct {
	Label string
	Model textinput.Model
}

// NewInput returns a focused text input.
func NewInput(label, placeholder string) *Input {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.Focus()
	ti.CharLimit = 253
	ti.Width = 40
	return &Input{Label: label, Model: ti}
}

// Update forwards to the inner model.
func (i *Input) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	i.Model, cmd = i.Model.Update(msg)
	return cmd
}

// View renders label + field.
func (i *Input) View() string {
	return i.Label + ": " + i.Model.View()
}

// Value is a shortcut.
func (i *Input) Value() string { return i.Model.Value() }
