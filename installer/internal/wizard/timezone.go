package wizard

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/azrtydxb/novanas/installer/internal/tui/components"
)

// TimezoneStep offers a filterable IANA list.
type TimezoneStep struct {
	state  *State
	list   *components.List
	filter string
}

// NewTimezoneStep returns a fresh step.
func NewTimezoneStep(s *State) *TimezoneStep {
	return &TimezoneStep{
		state: s,
		list:  components.NewList("Select your timezone (type to filter)", tzItems(""), false),
	}
}

// Init is a no-op.
func (s *TimezoneStep) Init() tea.Cmd { return nil }

// Update handles filter typing + list navigation.
func (s *TimezoneStep) Update(msg tea.Msg) (Step, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		str := km.String()
		// Type-ahead: printable runes add to filter. Escape clears.
		if len(str) == 1 && str >= "a" && str <= "z" {
			s.filter += str
			s.list = components.NewList("Select your timezone: "+s.filter, tzItems(s.filter), false)
			return s, nil
		}
		if str == "backspace" && len(s.filter) > 0 {
			s.filter = s.filter[:len(s.filter)-1]
			s.list = components.NewList("Select your timezone: "+s.filter, tzItems(s.filter), false)
			return s, nil
		}
	}
	s.list.Update(msg)
	if s.list.Picked >= 0 && s.list.Picked < len(s.list.Items) {
		s.state.Timezone = s.list.Items[s.list.Picked].Value
	}
	return s, nil
}

// View renders the current list.
func (s *TimezoneStep) View() string { return s.list.View() }

// IsComplete reports whether a timezone was picked.
func (s *TimezoneStep) IsComplete() bool { return s.state.Timezone != "" }

// Result returns the chosen IANA tz name.
func (s *TimezoneStep) Result() any { return s.state.Timezone }

func tzItems(filter string) []components.Item {
	f := strings.ToLower(filter)
	out := make([]components.Item, 0, len(Timezones))
	for _, tz := range Timezones {
		if f != "" && !strings.Contains(strings.ToLower(tz), f) {
			continue
		}
		out = append(out, components.Item{Label: tz, Value: tz})
	}
	return out
}
