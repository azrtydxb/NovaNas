package wizard

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/azrtydxb/novanas/installer/internal/disks"
	"github.com/azrtydxb/novanas/installer/internal/install"
	"github.com/azrtydxb/novanas/installer/internal/network"
	"github.com/azrtydxb/novanas/installer/internal/tui/components"
)

// InstallStep runs the actual install pipeline, reporting progress through
// tea messages.
type InstallStep struct {
	state      *State
	bundlePath string
	dryRun     bool
	log        func(format string, args ...any)

	progress float64
	label    string
	err      error
	done     bool
}

// InstallConfig parameterizes the step.
type InstallConfig struct {
	BundlePath string
	DryRun     bool
	Log        func(format string, args ...any)
}

// NewInstallStep returns a fresh step.
func NewInstallStep(s *State, cfg InstallConfig) *InstallStep {
	return &InstallStep{
		state:      s,
		bundlePath: cfg.BundlePath,
		dryRun:     cfg.DryRun,
		log:        cfg.Log,
		label:      "waiting",
	}
}

// progressMsg is sent after each pipeline stage.
type progressMsg struct {
	pct   float64
	label string
}

type doneMsg struct{ err error }

// Init kicks off the pipeline.
func (s *InstallStep) Init() tea.Cmd {
	return s.run()
}

// Update consumes our own messages.
func (s *InstallStep) Update(msg tea.Msg) (Step, tea.Cmd) {
	switch m := msg.(type) {
	case progressMsg:
		s.progress = m.pct
		s.label = m.label
	case doneMsg:
		s.done = true
		if m.err != nil {
			s.err = m.err
		} else {
			s.state.InstallDone = true
		}
	}
	return s, nil
}

// View renders progress bar + current step.
func (s *InstallStep) View() string {
	var b strings.Builder
	b.WriteString("Installing NovaNas\n\n")
	b.WriteString(components.Progress{Width: 50, Percent: s.progress, Label: s.label}.View())
	if s.err != nil {
		b.WriteString("\n\nFAILED: ")
		b.WriteString(s.err.Error())
	} else if s.done {
		b.WriteString("\n\nDone.")
	}
	return b.String()
}

// IsComplete reports pipeline completion (success or failure — UI moves on).
func (s *InstallStep) IsComplete() bool { return s.done }

// Result returns error (nil on success).
func (s *InstallStep) Result() any { return s.err }

// run returns a tea.Cmd that executes the pipeline synchronously. In a real
// system we'd stream progress messages back; for a first cut we run
// end-to-end and emit a final doneMsg. This still exercises the full plan
// structure and is trivially upgraded to a goroutine+channel model later.
func (s *InstallStep) run() tea.Cmd {
	return func() tea.Msg {
		if err := s.doInstall(); err != nil {
			return doneMsg{err: err}
		}
		return doneMsg{}
	}
}

func (s *InstallStep) logf(format string, args ...any) {
	if s.log != nil {
		s.log(format, args...)
	}
}

func (s *InstallStep) doInstall() error {
	if len(s.state.Disks) == 0 {
		return fmt.Errorf("no target disks selected")
	}
	target := s.state.Disks[0].Path
	runner := disks.NewRunner(s.dryRun, s.logf)

	// 1. Optional RAID-1 mirror.
	if s.state.Mirror && len(s.state.Disks) == 2 {
		s.logf("building mdadm mirror")
		plan, err := disks.BuildMirrorPlan("/dev/md0", []string{s.state.Disks[0].Path, s.state.Disks[1].Path})
		if err != nil {
			return fmt.Errorf("mirror plan: %w", err)
		}
		for _, c := range plan.Commands {
			s.logf("exec: %v", c)
			if !s.dryRun {
				if err := runner.Exec(c[0], c[1:]...); err != nil {
					return err
				}
			}
		}
		target = "/dev/md0"
	}

	// 2. Partition + format.
	s.logf("partitioning %s", target)
	partPlan := disks.BuildPartitionPlan(target, disks.DefaultLayout())
	if err := runner.Apply(partPlan); err != nil {
		return fmt.Errorf("partition: %w", err)
	}

	p1 := disks.PartName(target, 1) // EFI
	p2 := disks.PartName(target, 2) // Boot
	p3 := disks.PartName(target, 3) // OS-A
	p5 := disks.PartName(target, 5) // Persistent

	// 3. Mount OS-A.
	osRoot := "/mnt/osroot"
	efi := "/mnt/efi"
	boot := "/mnt/boot"
	persistent := "/mnt/persistent"

	mounts := [][]string{
		{"mkdir", "-p", osRoot, efi, boot, persistent},
		{"mount", p3, osRoot},
		{"mkdir", "-p", osRoot + "/boot"},
		{"mount", p2, boot},
		{"mount", p1, efi},
		{"mount", p5, persistent},
	}
	for _, c := range mounts {
		s.logf("exec: %v", c)
		if !s.dryRun {
			if err := runner.Exec(c[0], c[1:]...); err != nil {
				return fmt.Errorf("mount: %w", err)
			}
		}
	}

	// 4. Clone the live squashfs rootfs onto OS-A. The live ISO
	//    authoritative rootfs is already mounted by live-boot; we
	//    unsquashfs it to the target partition. No .raucb in the
	//    ISO; the RAUC bundle flow is reserved for A/B update
	//    post-install.
	unsquash := &install.SquashfsExtractor{DryRun: s.dryRun, Log: s.logf}
	if err := unsquash.Extract(osRoot); err != nil {
		return fmt.Errorf("squashfs extract: %w", err)
	}

	// 5. GRUB.
	grub := &install.GrubInstaller{DryRun: s.dryRun, Log: s.logf, Exec: runner.Exec}
	if err := grub.Install(efi, boot); err != nil {
		return fmt.Errorf("grub: %w", err)
	}

	// 6. Persistent partition.
	var staticCfg *network.StaticConfig
	if !s.state.DHCP {
		staticCfg = &network.StaticConfig{
			Interface: s.state.Iface,
			Hostname:  s.state.Hostname,
			Address:   s.state.StaticAddr,
			Gateway:   s.state.StaticGW,
			DNS:       s.state.StaticDNS,
		}
	}
	yaml := network.RenderNmstate(s.state.Iface, s.state.Hostname, staticCfg)

	seeder := &install.PersistentSeeder{DryRun: s.dryRun, Log: s.logf}
	if err := seeder.Seed(persistent, yaml, "stable", "0.0.0-dev"); err != nil {
		return fmt.Errorf("persistent seed: %w", err)
	}

	// Let the UI catch up briefly.
	time.Sleep(50 * time.Millisecond)
	return nil
}
