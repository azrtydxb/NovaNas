package wizard

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/azrtydxb/novanas/installer/internal/network"
	"github.com/azrtydxb/novanas/installer/internal/tui/components"
)

// NetworkStep picks a NIC and configures DHCP or static.
type NetworkStep struct {
	state *State
	phase int
	err   error

	nics []network.Interface

	nicList  *components.List
	modeList *components.List
	hostname *components.Input
	addr     *components.Input
	gateway  *components.Input
	dns1     *components.Input
	dns2     *components.Input

	skipped bool
}

// NewNetworkStep enumerates NICs.
func NewNetworkStep(s *State) *NetworkStep {
	ns := &NetworkStep{state: s}
	nics, err := network.List()
	if err != nil {
		ns.err = err
		return ns
	}
	ns.nics = nics
	items := make([]components.Item, 0, len(nics))
	for _, n := range nics {
		link := "down"
		if n.LinkUp {
			link = "up"
		}
		items = append(items, components.Item{
			Label: n.Name,
			Value: n.Name,
			Hint:  fmt.Sprintf("%s  link:%s", n.MAC, link),
		})
	}
	ns.nicList = components.NewList("Select management NIC", items, false)
	return ns
}

// SkipAll marks the step complete without configuring anything (used by
// --skip-network).
func (s *NetworkStep) SkipAll() {
	s.skipped = true
	s.state.DHCP = true
	s.state.Hostname = "novanas"
	s.state.Iface = "auto"
}

// Init is a no-op.
func (s *NetworkStep) Init() tea.Cmd { return nil }

// Update handles all phases.
func (s *NetworkStep) Update(msg tea.Msg) (Step, tea.Cmd) {
	if s.err != nil || s.skipped {
		return s, nil
	}
	switch s.phase {
	case 0: // NIC
		s.nicList.Update(msg)
		if s.nicList.Picked >= 0 && len(s.nics) > 0 {
			s.state.Iface = s.nics[s.nicList.Picked].Name
			s.modeList = components.NewList("Configure via:", []components.Item{
				{Label: "DHCP", Value: "dhcp"},
				{Label: "Static IP", Value: "static"},
			}, false)
			s.phase = 1
		}
	case 1: // mode
		s.modeList.Update(msg)
		if s.modeList.Picked >= 0 {
			s.state.DHCP = s.modeList.Items[s.modeList.Picked].Value == "dhcp"
			s.hostname = components.NewInput("Hostname", "novanas")
			s.phase = 2
		}
	case 2: // hostname
		cmd := s.hostname.Update(msg)
		if km, ok := msg.(tea.KeyMsg); ok && km.String() == "enter" {
			s.state.Hostname = strings.TrimSpace(s.hostname.Value())
			if s.state.Hostname == "" {
				s.state.Hostname = "novanas"
			}
			if s.state.DHCP {
				s.phase = 99 // done
			} else {
				s.addr = components.NewInput("Address (CIDR)", "192.168.1.50/24")
				s.phase = 3
			}
		}
		return s, cmd
	case 3:
		cmd := s.addr.Update(msg)
		if km, ok := msg.(tea.KeyMsg); ok && km.String() == "enter" {
			s.state.StaticAddr = strings.TrimSpace(s.addr.Value())
			s.gateway = components.NewInput("Gateway", "192.168.1.1")
			s.phase = 4
		}
		return s, cmd
	case 4:
		cmd := s.gateway.Update(msg)
		if km, ok := msg.(tea.KeyMsg); ok && km.String() == "enter" {
			s.state.StaticGW = strings.TrimSpace(s.gateway.Value())
			s.dns1 = components.NewInput("Primary DNS", "1.1.1.1")
			s.phase = 5
		}
		return s, cmd
	case 5:
		cmd := s.dns1.Update(msg)
		if km, ok := msg.(tea.KeyMsg); ok && km.String() == "enter" {
			v := strings.TrimSpace(s.dns1.Value())
			if v != "" {
				s.state.StaticDNS = append(s.state.StaticDNS, v)
			}
			s.dns2 = components.NewInput("Secondary DNS (optional)", "1.0.0.1")
			s.phase = 6
		}
		return s, cmd
	case 6:
		cmd := s.dns2.Update(msg)
		if km, ok := msg.(tea.KeyMsg); ok && km.String() == "enter" {
			v := strings.TrimSpace(s.dns2.Value())
			if v != "" {
				s.state.StaticDNS = append(s.state.StaticDNS, v)
			}
			s.phase = 99
		}
		return s, cmd
	}
	return s, nil
}

// View renders the current phase.
func (s *NetworkStep) View() string {
	if s.err != nil {
		return "Network scan failed: " + s.err.Error()
	}
	if s.skipped {
		return "Network configuration skipped (DHCP on first boot)."
	}
	switch s.phase {
	case 0:
		return s.nicList.View()
	case 1:
		return s.modeList.View()
	case 2:
		return s.hostname.View() + "\n\n(enter to confirm)"
	case 3:
		return s.addr.View() + "\n\n(enter to confirm)"
	case 4:
		return s.gateway.View() + "\n\n(enter to confirm)"
	case 5:
		return s.dns1.View() + "\n\n(enter to confirm)"
	case 6:
		return s.dns2.View() + "\n\n(enter to confirm or leave empty)"
	}
	return "Network configured."
}

// IsComplete reports completion.
func (s *NetworkStep) IsComplete() bool {
	return s.skipped || s.phase >= 99
}

// Result returns the composite net config.
func (s *NetworkStep) Result() any {
	return map[string]any{
		"iface":    s.state.Iface,
		"hostname": s.state.Hostname,
		"dhcp":     s.state.DHCP,
		"address":  s.state.StaticAddr,
		"gateway":  s.state.StaticGW,
		"dns":      s.state.StaticDNS,
	}
}
