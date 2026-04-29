// Command nova-iscsi-restore re-applies the saved LIO/iSCSI target
// configuration at boot by invoking `targetctl restore`.
//
// On Debian, the targetcli package ships its own systemd unit
// (targetclid.service / rtslib-fb-targetctl.service) that auto-restores
// /etc/rtslib-fb-target/saveconfig.json on boot. On distros where that
// unit isn't shipped, this binary serves as a portable equivalent:
// systemd invokes nova-iscsi-restore.service, which calls targetctl
// restore. If targetcli is not installed, this binary logs an info
// message and exits 0 (not an error).
package main

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{}))
	slog.SetDefault(logger)

	bin, err := exec.LookPath("targetctl")
	if err != nil {
		logger.Info("targetctl not present; skipping iSCSI restore", "err", err)
		os.Exit(0)
	}

	ctx := context.Background()
	out, err := exec.CommandContext(ctx, bin, "restore").CombinedOutput()
	if err != nil {
		logger.Error("targetctl restore failed", "err", err, "output", string(out))
		os.Exit(1)
	}
	logger.Info("iSCSI configuration restored", "output", string(out))
}
