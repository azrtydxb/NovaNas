// novanas-installer — text-mode curses installer for the NovaNas OS.
//
// Runs inside the live ISO environment, walks the operator through language,
// timezone, disk, and network selection, then writes the OS to the chosen
// target via RAUC bundle extraction, installs GRUB, and initializes the
// persistent partition. See README.md for details.
//
// In --auto mode the TUI is bypassed entirely: the first suitable disk is
// picked, DHCP is assumed, and the installer runs end-to-end unattended.
// The systemd unit shipped in the rootfs (novanas-installer.service)
// invokes this mode on live boot.
package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/azrtydxb/novanas/installer/internal/app"
	"github.com/azrtydxb/novanas/installer/internal/disks"
	"github.com/azrtydxb/novanas/installer/internal/install"
	"github.com/azrtydxb/novanas/installer/internal/logging"
)

func main() {
	bundle := flag.String("bundle", "", "DEPRECATED: ignored. The installer now clones the live squashfs directly onto the target partition.")
	debug := flag.Bool("debug", false, "verbose logging")
	skipNet := flag.Bool("skip-network", false, "skip the network step (first boot uses DHCP)")
	autoDisk := flag.String("auto-disk", "", "unattended: select this disk without prompting")
	autoMode := flag.Bool("auto", false, "fully unattended install: no TUI, first suitable disk, DHCP, power off on exit")
	iAmSure := flag.Bool("i-am-sure", false, "actually write partition tables and extract the bundle")
	logPath := flag.String("log-file", logging.DefaultLogPath, "path to the install log")
	flag.Parse()

	log := logging.New(*logPath, *debug)
	defer log.Close()

	// Honor a runtime dry-run toggle so the systemd unit can set
	// NOVANAS_INSTALLER_DRY_RUN=1 under packer for fast CI turnaround
	// without the installer actually touching the target disk.
	dryRun := !*iAmSure
	if v := os.Getenv("NOVANAS_INSTALLER_DRY_RUN"); v == "1" || v == "true" {
		dryRun = true
		log.Infof("NOVANAS_INSTALLER_DRY_RUN set; forcing dry-run regardless of --i-am-sure")
	}

	log.Infof("novanas-installer starting bundle=%s dryRun=%v auto=%v", *bundle, dryRun, *autoMode)

	if *autoMode {
		if err := runAuto(*bundle, *autoDisk, dryRun, log.Infof); err != nil {
			log.Errorf("auto install failed: %v", err)
			fmt.Fprintln(os.Stderr, "installer error:", err)
			os.Exit(1)
		}
		log.Infof("auto install complete")
		return
	}

	m := app.New(app.Options{
		BundlePath:  *bundle,
		Debug:       *debug,
		SkipNetwork: *skipNet,
		AutoDisk:    *autoDisk,
		DryRun:      dryRun,
		Log:         log.Infof,
	})

	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		log.Errorf("tea program exited: %v", err)
		fmt.Fprintln(os.Stderr, "installer error:", err)
		os.Exit(1)
	}

	if m.Reboot() {
		log.Infof("operator requested reboot")
		if err := rebootSystem(); err != nil {
			log.Errorf("reboot failed: %v (exiting cleanly; caller should reboot)", err)
			fmt.Fprintln(os.Stderr, "could not reboot automatically:", err)
			os.Exit(2)
		}
	}
}

// runAuto is the headless installer path. It picks the first disk that
// passes the scanner's candidate filter (or honors --auto-disk if given),
// then runs partition + RAUC extract + GRUB + persistent seed directly.
// Every shell-out is idempotent; errors are returned with context so the
// caller (systemd + log file) can triage.
func runAuto(bundlePath, wantDisk string, dryRun bool, logf func(string, ...any)) error {
	scanner := disks.NewScanner()
	candidates, err := scanner.Scan()
	if err != nil {
		return fmt.Errorf("scan disks: %w", err)
	}
	if len(candidates) == 0 {
		return fmt.Errorf("no installable disks found (need >=16GB, non-removable, type=disk)")
	}

	var target string
	if wantDisk != "" {
		for _, c := range candidates {
			if c.Path == wantDisk || c.Name == wantDisk {
				target = c.Path
				break
			}
		}
		if target == "" {
			return fmt.Errorf("requested --auto-disk %q not found among candidates", wantDisk)
		}
	} else {
		target = candidates[0].Path
	}
	logf("auto: target disk %s (dryRun=%v)", target, dryRun)

	if _, err := os.Stat(bundlePath); err != nil {
		return fmt.Errorf("bundle not accessible at %s: %w", bundlePath, err)
	}

	runner := disks.NewRunner(dryRun, logf)

	logf("auto: partitioning %s", target)
	partPlan := disks.BuildPartitionPlan(target, disks.DefaultLayout())
	if err := runner.Apply(partPlan); err != nil {
		return fmt.Errorf("partition: %w", err)
	}

	p1 := disks.PartName(target, 1) // EFI
	p2 := disks.PartName(target, 2) // Boot
	p3 := disks.PartName(target, 3) // OS-A
	p5 := disks.PartName(target, 5) // Persistent

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
		logf("auto: exec %v", c)
		if !dryRun {
			if err := runner.Exec(c[0], c[1:]...); err != nil {
				return fmt.Errorf("mount %v: %w", c, err)
			}
		}
	}

	rauc := &install.RAUCExtractor{DryRun: dryRun, Log: logf}
	if err := rauc.Verify(bundlePath); err != nil && !dryRun {
		return fmt.Errorf("rauc verify: %w", err)
	}
	if err := rauc.Extract(bundlePath, osRoot); err != nil {
		return fmt.Errorf("rauc extract: %w", err)
	}

	grub := &install.GrubInstaller{DryRun: dryRun, Log: logf, Exec: runner.Exec}
	if err := grub.Install(efi, boot); err != nil {
		return fmt.Errorf("grub: %w", err)
	}

	seeder := &install.PersistentSeeder{DryRun: dryRun, Log: logf}
	// Auto mode assumes DHCP on all interfaces — the first boot of the
	// installed system re-runs enrollment and can re-render nmstate.
	if err := seeder.Seed(persistent, "", "stable", "0.0.0-dev"); err != nil {
		return fmt.Errorf("persistent seed: %w", err)
	}

	return nil
}
