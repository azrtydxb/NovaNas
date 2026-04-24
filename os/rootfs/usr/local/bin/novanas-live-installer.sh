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
BUNDLE=/run/live/medium/novanas/novanas.raucb
INSTALLER=/usr/local/bin/novanas-installer

# Always echo to the serial console so `virsh console` / packer VNC shows it.
exec > >(tee -a "$LOG" > /dev/console) 2>&1

echo "==== novanas-live-installer: starting $(date -Is) ===="
echo "kernel cmdline: $(cat /proc/cmdline)"

# Honor packer dry-run opt-in via kernel cmdline. grub.cfg on ISOs built
# with NOVANAS_ISO_PACKER_MODE=1 appends novanas.installer.mode=dryrun.
if grep -qE 'novanas\.installer\.mode=dryrun' /proc/cmdline; then
    export NOVANAS_INSTALLER_DRY_RUN=1
    echo "novanas-live-installer: dry-run enabled via kernel cmdline"
fi

if [[ ! -x "$INSTALLER" ]]; then
    echo "FATAL: $INSTALLER missing; cannot continue" >&2
else
    if [[ ! -f "$BUNDLE" ]]; then
        echo "WARN: bundle $BUNDLE not found; trying alternate locations" >&2
        for alt in /cdrom/novanas/novanas.raucb /media/cdrom/novanas/novanas.raucb /run/live/medium/novanas/novanas.raucb; do
            if [[ -f "$alt" ]]; then
                BUNDLE="$alt"
                echo "using bundle: $BUNDLE"
                break
            fi
        done
    fi

    echo "invoking: $INSTALLER --auto --bundle=$BUNDLE"
    "$INSTALLER" --auto --bundle="$BUNDLE" --log-file="$LOG"
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
