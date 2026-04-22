package wizard

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/azrtydxb/novanas/installer/internal/disks"
	"github.com/azrtydxb/novanas/installer/internal/tui/components"
)

// DisksStep shows the available disks and lets the user pick one (single) or
// two (mirrored) as the OS target.
type DisksStep struct {
	state     *State
	available []disks.Disk
	list      *components.List
	confirm   *components.YesNo
	phase     int // 0 pick, 1 confirm (if 2 picked)
	err       error
}

// NewDisksStep scans the system and returns a ready step.
func NewDisksStep(s *State, scanner *disks.Scanner) *DisksStep {
	ds := &DisksStep{state: s}
	available, err := scanner.Scan()
	if err != nil {
		ds.err = err
		return ds
	}
	ds.available = available
	items := make([]components.Item, 0, len(available))
	for _, d := range available {
		hint := fmt.Sprintf("%s  %s  %s", disks.HumanSize(d.SizeBytes), d.Model, d.Transport)
		items = append(items, components.Item{Label: d.Path, Value: d.Path, Hint: strings.TrimSpace(hint)})
	}
	ds.list = components.NewList("Select OS disk(s) — space to toggle, enter to confirm", items, true)
	return ds
}

// Init is a no-op.
func (s *DisksStep) Init() tea.Cmd { return nil }

// Update handles list navigation + the optional mirror confirmation.
func (s *DisksStep) Update(msg tea.Msg) (Step, tea.Cmd) {
	if s.err != nil || s.list == nil {
		return s, nil
	}
	switch s.phase {
	case 0:
		s.list.Update(msg)
		if km, ok := msg.(tea.KeyMsg); ok && km.String() == "enter" {
			chosen := s.list.Chosen()
			if len(chosen) == 0 && s.list.Cursor >= 0 {
				// Treat Enter without space-toggles as "pick the cursor item".
				chosen = []int{s.list.Cursor}
			}
			switch len(chosen) {
			case 1:
				s.state.Disks = []disks.Disk{s.available[chosen[0]]}
				s.state.Mirror = false
			case 2:
				s.state.Disks = []disks.Disk{s.available[chosen[0]], s.available[chosen[1]]}
				s.state.Mirror = true
				s.confirm = &components.YesNo{Question: "Create RAID-1 mirror across these two disks?"}
				s.phase = 1
				return s, nil
			default:
				// Too many / zero: reset.
				s.state.Disks = nil
				return s, nil
			}
		}
	case 1:
		s.confirm.Update(msg)
		if s.confirm.Answer == 0 {
			// User said no; drop back to selection.
			s.state.Disks = nil
			s.state.Mirror = false
			s.phase = 0
			s.confirm = nil
		}
	}
	return s, nil
}

// View renders current UI.
func (s *DisksStep) View() string {
	if s.err != nil {
		return "Disk scan failed: " + s.err.Error()
	}
	if s.list == nil || len(s.available) == 0 {
		return "No suitable OS disks found. An OS disk must be >= 16 GB, non-removable, and not the install medium."
	}
	if s.phase == 1 && s.confirm != nil {
		return s.confirm.View()
	}
	return s.list.View()
}

// IsComplete reports whether an OS target is chosen.
func (s *DisksStep) IsComplete() bool {
	if len(s.state.Disks) == 0 {
		return false
	}
	if s.state.Mirror {
		return s.confirm != nil && s.confirm.Answer == 1
	}
	return true
}

// Result returns the disk paths.
func (s *DisksStep) Result() any {
	paths := make([]string, 0, len(s.state.Disks))
	for _, d := range s.state.Disks {
		paths = append(paths, d.Path)
	}
	return map[string]any{"disks": paths, "mirror": s.state.Mirror}
}
