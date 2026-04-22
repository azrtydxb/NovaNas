package components

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// YesNo is a simple y/n prompt.
type YesNo struct {
	Question string
	Answer   int // -1 none, 0 no, 1 yes
}

// Update handles keypresses.
func (p *YesNo) Update(msg tea.Msg) tea.Cmd {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil
	}
	switch strings.ToLower(km.String()) {
	case "y":
		p.Answer = 1
	case "n":
		p.Answer = 0
	}
	return nil
}

// View renders the prompt.
func (p YesNo) View() string {
	return p.Question + " [y/N]"
}
