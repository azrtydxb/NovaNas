package app

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/azrtydxb/novanas/installer/internal/disks"
	"github.com/azrtydxb/novanas/installer/internal/wizard"
)

// Options configures the Model.
type Options struct {
	BundlePath  string
	Debug       bool
	SkipNetwork bool
	AutoDisk    string
	DryRun      bool // true unless --i-am-sure
	Log         func(format string, args ...any)
	DiskScanner *disks.Scanner
}

// Model is the root bubbletea model holding the list of steps + cursor.
type Model struct {
	opts    Options
	state   *wizard.State
	theme   Theme
	steps   []wizard.Step
	current int
	quit    bool
	reboot  bool
}

// New constructs the root Model and all steps.
func New(opts Options) *Model {
	if opts.Log == nil {
		opts.Log = func(string, ...any) {}
	}
	if opts.DiskScanner == nil {
		opts.DiskScanner = disks.NewScanner()
	}

	st := &wizard.State{}
	m := &Model{
		opts:  opts,
		state: st,
		theme: DefaultTheme(),
	}

	netStep := wizard.NewNetworkStep(st)
	if opts.SkipNetwork {
		netStep.SkipAll()
	}

	m.steps = []wizard.Step{
		wizard.NewLanguageStep(st),
		wizard.NewTimezoneStep(st),
		wizard.NewDisksStep(st, opts.DiskScanner),
		netStep,
		wizard.NewConfirmStep(st),
		wizard.NewInstallStep(st, wizard.InstallConfig{
			BundlePath: opts.BundlePath,
			DryRun:     opts.DryRun,
			Log:        opts.Log,
		}),
		wizard.NewDoneStep(st),
	}
	return m
}

// State returns the current wizard state (used by tests).
func (m *Model) State() *wizard.State { return m.state }

// Reboot reports whether the user asked to reboot at the end.
func (m *Model) Reboot() bool { return m.reboot }

// Init starts the current (first) step.
func (m *Model) Init() tea.Cmd {
	if m.current < len(m.steps) {
		return m.steps[m.current].Init()
	}
	return nil
}

// Update dispatches messages to the current step and advances on completion.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok && km.String() == "ctrl+c" {
		m.quit = true
		return m, tea.Quit
	}
	if m.current >= len(m.steps) {
		m.quit = true
		return m, tea.Quit
	}
	step := m.steps[m.current]
	var cmd tea.Cmd
	step, cmd = step.Update(msg)
	m.steps[m.current] = step

	if step.IsComplete() {
		m.current++
		if m.current >= len(m.steps) {
			if ds, ok := m.steps[len(m.steps)-1].(*wizard.DoneStep); ok {
				m.reboot = ds.Reboot
			}
			return m, tea.Quit
		}
		if initCmd := m.steps[m.current].Init(); initCmd != nil {
			return m, tea.Batch(cmd, initCmd)
		}
	}
	return m, cmd
}

// View renders the active step with a common header.
func (m *Model) View() string {
	if m.quit {
		return ""
	}
	var b strings.Builder
	b.WriteString(m.theme.Title.Render("NovaNas Installer"))
	b.WriteString("\n")
	b.WriteString(m.theme.Step.Render(fmt.Sprintf("Step %d of %d", m.current+1, len(m.steps))))
	b.WriteString("\n\n")
	if m.current < len(m.steps) {
		b.WriteString(m.steps[m.current].View())
	}
	b.WriteString("\n\n")
	b.WriteString(m.theme.Help.Render("↑/↓ move · enter select · ctrl+c quit"))
	return m.theme.Border.Render(b.String())
}
