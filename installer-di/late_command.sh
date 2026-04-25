#!/bin/bash
# Runs inside /target chroot after d-i finishes the base install.
# Responsibilities:
#  1. Install RAUC (already in pkgsel/include) and configure /etc/rauc/system.conf
#  2. Configure GRUB to use RAUC's A/B boot ordering
#  3. Run `rauc install` to overlay the actual NovaNas bundle into slot A
#  4. Mark slot A as good

set -euo pipefail
log() { printf '[late_command] %s\n' "$*" >&2; }

log "writing /etc/rauc/system.conf"
mkdir -p /etc/rauc
cat > /etc/rauc/system.conf <<'EOF'
[system]
compatible=novanas-amd64
bootloader=grub
mountprefix=/run/rauc

[keyring]
path=/etc/rauc/keyring.pem

[slot.rootfs.0]
device=/dev/md2
type=ext4
bootname=A

[slot.rootfs.1]
device=/dev/md3
type=ext4
bootname=B

[slot.bootloader.0]
device=/dev/md1
type=ext4
EOF

log "installing RAUC keyring"
mkdir -p /etc/rauc
cp /var/cache/novanas-keyring.pem /etc/rauc/keyring.pem 2>/dev/null \
  || log "WARN: no keyring shipped; first 'rauc install' will fail signature check"

log "writing GRUB RAUC-aware menu fragment"
cat > /etc/default/grub.d/90-rauc.cfg <<'EOF'
GRUB_DISTRIBUTOR="NovaNas"
GRUB_CMDLINE_LINUX_DEFAULT="quiet rauc.slot=A"
GRUB_DISABLE_OS_PROBER=true
EOF

log "writing /etc/grub.d/30_rauc"
cat > /etc/grub.d/30_rauc <<'EOF'
#!/bin/sh
exec tail -n +3 $0
menuentry 'NovaNas (slot A)' { linux /vmlinuz root=/dev/md2 ro rauc.slot=A; initrd /initrd.img }
menuentry 'NovaNas (slot B)' { linux /vmlinuz root=/dev/md3 ro rauc.slot=B; initrd /initrd.img }
EOF
chmod +x /etc/grub.d/30_rauc
update-grub

log "writing /etc/fstab persistent mount"
echo "/dev/md4  /var/lib/novanas  ext4  defaults  0 2" >> /etc/fstab

if [[ -f /var/cache/novanas-initial.raucb ]]; then
  log "running rauc install of initial bundle"
  rauc install /var/cache/novanas-initial.raucb || log "WARN: rauc install failed; system will boot stock Debian on first boot"
  rauc status mark-good rootfs.0 || true
  rm -f /var/cache/novanas-initial.raucb
else
  log "no initial RAUC bundle present; system will boot stock Debian"
fi

log "late_command done"
