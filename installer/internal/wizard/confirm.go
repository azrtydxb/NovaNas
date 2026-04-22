package wizard

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/azrtydxb/novanas/installer/internal/tui/components"
)

// ConfirmStep summarizes choices and requires the user to type "yes".
type ConfirmStep struct {
	state   *State
	input   *components.Input
	ok      bool
}

// NewConfirmStep returns a confirmation step.
func NewConfirmStep(s *State) *ConfirmStep {
	return &ConfirmStep{
		state: s,
		input: components.NewInput(`Type "yes" to proceed`, "yes"),
	}
}

// Init is a no-op.
func (s *ConfirmStep) Init() tea.Cmd { return nil }

// Update watches for the literal text "yes".
func (s *ConfirmStep) Update(msg tea.Msg) (Step, tea.Cmd) {
	cmd := s.input.Update(msg)
	if km, ok := msg.(tea.KeyMsg); ok && km.String() == "enter" {
		if strings.TrimSpace(strings.ToLower(s.input.Value())) == "yes" {
			s.ok = true
			s.state.Confirmed = true
		}
	}
	return s, cmd
}

// View renders the summary + prompt.
func (s *ConfirmStep) View() string {
	var b strings.Builder
	b.WriteString("Review your choices:\n\n")
	fmt.Fprintf(&b, "  Language:  %s (kb: %s)\n", s.state.Language, s.state.Keyboard)
	fmt.Fprintf(&b, "  Timezone:  %s\n", s.state.Timezone)
	paths := make([]string, 0, len(s.state.Disks))
	for _, d := range s.state.Disks {
		paths = append(paths, d.Path)
	}
	fmt.Fprintf(&b, "  Disks:     %s", strings.Join(paths, ", "))
	if s.state.Mirror {
		b.WriteString("  (RAID-1 mirror)")
	}
	b.WriteString("\n")
	fmt.Fprintf(&b, "  Hostname:  %s\n", s.state.Hostname)
	fmt.Fprintf(&b, "  Interface: %s\n", s.state.Iface)
	if s.state.DHCP {
		b.WriteString("  Network:   DHCP\n")
	} else {
		fmt.Fprintf(&b, "  Network:   static %s gw=%s dns=%s\n",
			s.state.StaticAddr, s.state.StaticGW, strings.Join(s.state.StaticDNS, ","))
	}
	b.WriteString("\nWARNING: The selected disk(s) will be completely wiped.\n\n")
	b.WriteString(s.input.View())
	return b.String()
}

// IsComplete reports whether "yes" was typed.
func (s *ConfirmStep) IsComplete() bool { return s.ok }

// Result is a boolean ack.
func (s *ConfirmStep) Result() any { return s.ok }
