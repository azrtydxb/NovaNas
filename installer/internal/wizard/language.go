package wizard

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/azrtydxb/novanas/installer/internal/tui/components"
)

// LanguageStep picks a UI language + keyboard layout.
type LanguageStep struct {
	state    *State
	langList *components.List
	kbList   *components.List
	phase    int // 0 = language, 1 = keyboard
}

// NewLanguageStep returns a fresh step.
func NewLanguageStep(s *State) *LanguageStep {
	items := make([]components.Item, 0, len(Languages))
	for _, l := range Languages {
		hint := ""
		if l.Code != "en" {
			hint = "(translations pending)"
		}
		items = append(items, components.Item{Label: l.Label, Value: l.Code, Hint: hint})
	}
	return &LanguageStep{
		state:    s,
		langList: components.NewList("Select your language", items, false),
	}
}

// Init is a no-op.
func (s *LanguageStep) Init() tea.Cmd { return nil }

// Update forwards to the active sub-list.
func (s *LanguageStep) Update(msg tea.Msg) (Step, tea.Cmd) {
	switch s.phase {
	case 0:
		s.langList.Update(msg)
		if s.langList.Picked >= 0 {
			lang := Languages[s.langList.Picked]
			s.state.Language = lang.Code
			// Build keyboard list for chosen language.
			kbs := Keyboards[lang.Code]
			items := make([]components.Item, 0, len(kbs))
			for _, kb := range kbs {
				items = append(items, components.Item{Label: kb, Value: kb})
			}
			s.kbList = components.NewList("Select a keyboard layout", items, false)
			s.phase = 1
		}
	case 1:
		s.kbList.Update(msg)
		if s.kbList.Picked >= 0 {
			s.state.Keyboard = s.kbList.Items[s.kbList.Picked].Value
		}
	}
	return s, nil
}

// View renders the step.
func (s *LanguageStep) View() string {
	if s.phase == 0 || s.kbList == nil {
		return s.langList.View()
	}
	return s.kbList.View()
}

// IsComplete reports whether the step has a language + keyboard.
func (s *LanguageStep) IsComplete() bool {
	return s.state.Language != "" && s.state.Keyboard != ""
}

// Result returns the tuple.
func (s *LanguageStep) Result() any {
	return map[string]string{"language": s.state.Language, "keyboard": s.state.Keyboard}
}
