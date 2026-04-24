#!/bin/bash
# NovaNas live installer launcher.
#
# Invoked by novanas-installer.service on live boot. Runs the Go installer
# binary in --auto mode, tees output to journal + serial + a log file, and
# unconditionally powers the VM off afterwards so packer's shutdown_timeout
# trips cleanly regardless of install success or failure.
#
# Idempotent: safe to re-run; the installer itself handles re-entrancy.
# Loud-on-error: every shell command echoes to serial and the log file.

set +e  # we handle exit status explicitly — do not abort early

LOG=/var/log/novanas-installer.log
INSTALLER=/usr/local/bin/novanas-installer

# Always echo to the serial console so `virsh console` / packer VNC shows it.
exec > >(tee -a "$LOG" > /dev/console) 2>&1

echo "==== novanas-live-installer: starting $(date -Is) ===="
echo "kernel cmdline: $(cat /proc/cmdline)"

# Single-ISO policy: the same image serves bare-metal operators and the
# packer VA CI build. Runtime environment detection picks the mode:
#
#   * Bare-metal / real VM (operator laptop, production NAS, real
#     hypervisor) — drop into the interactive TUI installer. The
#     operator selects disks, confirms the destructive step, and the
#     installer writes the bundle to the chosen hardware.
#
#   * QEMU TCG or KVM-backed CI VM (packer build) — no TTY attached,
#     SMBIOS product name is "Standard PC (…)" / sys_vendor is "QEMU"
#     or "Red Hat". Run --auto with NOVANAS_INSTALLER_DRY_RUN=1 so the
#     installer exercises the full pipeline without writing to the
#     disk, then powers off.
#
# The kernel cmdline override (novanas.installer.mode=dryrun or
# novanas.installer.mode=real) still wins if explicitly set — useful
# for forcing dry-run on real hardware when smoke-testing the ISO.
MODE=""
case "$(cat /proc/cmdline)" in
    *novanas.installer.mode=dryrun*) MODE=dryrun ;;
    *novanas.installer.mode=real*)   MODE=real ;;
esac

if [[ -z "$MODE" ]]; then
    sys_vendor=$(cat /sys/class/dmi/id/sys_vendor 2>/dev/null || echo unknown)
    product=$(cat /sys/class/dmi/id/product_name 2>/dev/null || echo unknown)
    echo "dmi sys_vendor='$sys_vendor' product='$product'"
    case "$sys_vendor" in
        QEMU|"Red Hat"|Bochs|innotek\ GmbH) MODE=dryrun ;;
        *) MODE=real ;;
    esac
fi
echo "novanas-live-installer: detected mode=$MODE"

if [[ ! -x "$INSTALLER" ]]; then
    echo "FATAL: $INSTALLER missing; cannot continue" >&2
elif [[ "$MODE" == "real" ]]; then
    # Interactive TUI path. Hand the controlling TTY to the installer
    # so its bubbletea renderer can draw. Do NOT tee stdout away from
    # it — that breaks termios. We still capture to the log via the
    # installer's own --log-file flag.
    exec > /dev/console 2>&1 < /dev/console
    echo "==== launching interactive installer; no poweroff after exit ===="
    "$INSTALLER" --log-file="$LOG"
    rc=$?
    echo "==== installer exited rc=$rc $(date -Is) ===="
    # After a successful real install the installer reboots itself;
    # if it returns without rebooting, drop the operator to a shell
    # rather than pulling the rug.
    echo "drop to rescue shell — Ctrl-D to poweroff"
    exec /bin/bash -l
else
    # Dry-run auto path (CI / packer).
    export NOVANAS_INSTALLER_DRY_RUN=1
    echo "invoking: $INSTALLER --auto"
    "$INSTALLER" --auto --log-file="$LOG"
    rc=$?
    echo "==== installer exited rc=$rc $(date -Is) ===="
fi

# Give journald + tee one second to flush before we power off.
sync
sleep 1

echo "==== powering off ===="
# Try the clean path first, then force.
/bin/systemctl poweroff --no-block || /sbin/poweroff -f || echo o > /proc/sysrq-trigger
# If we're still here after 30s, sysrq the box.
sleep 30
echo b > /proc/sysrq-trigger
