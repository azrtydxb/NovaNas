// novanas-installer — text-mode curses installer for the NovaNas OS.
//
// Runs inside the live ISO environment, walks the operator through language,
// timezone, disk, and network selection, then writes the OS to the chosen
// target via RAUC bundle extraction, installs GRUB, and initializes the
// persistent partition. See README.md for details.
package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/azrtydxb/novanas/installer/internal/app"
	"github.com/azrtydxb/novanas/installer/internal/logging"
)

func main() {
	bundle := flag.String("bundle", "/cdrom/novanas.raucb", "path to the RAUC bundle to install")
	debug := flag.Bool("debug", false, "verbose logging")
	skipNet := flag.Bool("skip-network", false, "skip the network step (first boot uses DHCP)")
	autoDisk := flag.String("auto-disk", "", "unattended: select this disk without prompting")
	iAmSure := flag.Bool("i-am-sure", false, "actually write partition tables and extract the bundle")
	logPath := flag.String("log-file", logging.DefaultLogPath, "path to the install log")
	flag.Parse()

	log := logging.New(*logPath, *debug)
	defer log.Close()
	log.Infof("novanas-installer starting bundle=%s dryRun=%v", *bundle, !*iAmSure)

	m := app.New(app.Options{
		BundlePath:  *bundle,
		Debug:       *debug,
		SkipNetwork: *skipNet,
		AutoDisk:    *autoDisk,
		DryRun:      !*iAmSure,
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
