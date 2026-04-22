package wizard

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/azrtydxb/novanas/installer/internal/tui/components"
)

// DoneStep offers a reboot prompt.
type DoneStep struct {
	state  *State
	prompt *components.YesNo
	Reboot bool
}

// NewDoneStep returns a fresh step.
func NewDoneStep(s *State) *DoneStep {
	return &DoneStep{
		state:  s,
		prompt: &components.YesNo{Question: "Installation complete. Reboot now?"},
	}
}

// Init is a no-op.
func (s *DoneStep) Init() tea.Cmd { return nil }

// Update watches for y/n.
func (s *DoneStep) Update(msg tea.Msg) (Step, tea.Cmd) {
	s.prompt.Update(msg)
	if s.prompt.Answer == 1 {
		s.Reboot = true
	}
	return s, nil
}

// View renders the prompt.
func (s *DoneStep) View() string {
	if s.state.InstallDone {
		return s.prompt.View()
	}
	return "Install finished with errors; check /var/log/novanas-installer.log. Press n to exit."
}

// IsComplete reports once user chose y or n.
func (s *DoneStep) IsComplete() bool { return s.prompt.Answer >= 0 }

// Result returns whether a reboot was requested.
func (s *DoneStep) Result() any { return s.Reboot }
