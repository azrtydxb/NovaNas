# NovaNas OS build orchestrator.
#
# Driven by mkosi (os/mkosi.conf + mkosi.conf.d/). The build graph is:
#
#   mkosi-live      -> build/out/mkosi/live/image/        (rootfs tree)
#   mkosi-update    -> build/out/mkosi/update/image/      (rootfs tree)
#   build-images.sh -> filesystem.squashfs / rootfs.img / boot.img / kernel.*
#   bundle          -> novanas-<ver>.raucb                (via rauc bundle)
#   iso             -> novanas-<ver>.iso                  (via grub-mkrescue)
#
# Image builds need root for loop mounts; CI invokes this via sudo.

SHELL        := /bin/bash
.SHELLFLAGS  := -eu -o pipefail -c

VERSION      ?= 0.0.0-dev
CHANNEL      ?= dev
ARCH         ?= amd64
K3S_VERSION  ?= v1.31.3+k3s1
BUILD_DIR    ?= $(CURDIR)/build/out
# mkosi v25 refuses a workspace dir that lives under any source dir (i.e.
# under $(CURDIR)). Put it at the repo root instead.
WORKSPACE_DIR ?= $(abspath $(CURDIR)/..)/.mkosi-workspace
ROOTFS_DIR   ?= $(CURDIR)/rootfs
RAUC_DIR     ?= $(CURDIR)/rauc
PACKER_DIR   ?= $(CURDIR)/packer

INSTALLER_BINARY ?= $(CURDIR)/../installer/bin/novanas-installer

MKOSI_LIVE_DIR    := $(BUILD_DIR)/mkosi/live
MKOSI_UPDATE_DIR  := $(BUILD_DIR)/mkosi/update
# mkosi v25 names the output directory after ImageId (set in mkosi.conf), not
# a literal "image".
LIVE_ROOTFS       := $(MKOSI_LIVE_DIR)/novanas
UPDATE_ROOTFS     := $(MKOSI_UPDATE_DIR)/novanas

LAYERED_IMG  := $(BUILD_DIR)/rootfs.img
BOOT_IMG     := $(BUILD_DIR)/boot.img
SQUASHFS     := $(BUILD_DIR)/filesystem.squashfs
KERNEL       := $(BUILD_DIR)/kernel.vmlinuz
INITRD       := $(BUILD_DIR)/kernel.initrd
BUNDLE       := $(BUILD_DIR)/novanas-$(VERSION).raucb
ISO          := $(BUILD_DIR)/novanas-$(VERSION).iso

export VERSION CHANNEL ARCH BUILD_DIR ROOTFS_DIR RAUC_DIR INSTALLER_BINARY K3S_VERSION

MKOSI        ?= mkosi
MKOSI_COMMON := \
    --directory=$(CURDIR) \
    --workspace-dir=$(WORKSPACE_DIR) \
    --environment=NOVANAS_VERSION=$(VERSION) \
    --environment=NOVANAS_CHANNEL=$(CHANNEL) \
    --environment=NOVANAS_K3S_VERSION=$(K3S_VERSION) \
    --environment=NOVANAS_INSTALLER_BINARY=$(INSTALLER_BINARY)

.PHONY: all mkosi-live mkosi-update live-artifacts update-artifacts \
        bundle iso iso-only va va-only clean lint help

help:
	@echo "NovaNas OS build targets (mkosi-driven):"
	@echo "  make mkosi-live       - Bootstrap live-ISO rootfs tree via mkosi"
	@echo "  make mkosi-update     - Bootstrap RAUC-slot rootfs tree via mkosi"
	@echo "  make live-artifacts   - Squashfs + kernel + initrd for the ISO"
	@echo "  make update-artifacts - ext4 rootfs.img + boot.img for the bundle"
	@echo "  make bundle           - RAUC .raucb update bundle"
	@echo "  make iso              - Hybrid installer ISO"
	@echo "  make va               - Virtual appliance images (raw/qcow2/vmdk/ova)"
	@echo "  make all              - bundle + iso + va"
	@echo "  make clean            - rm -rf $(BUILD_DIR)"
	@echo "  make lint             - Static checks (shellcheck, make -n)"
	@echo ""
	@echo "Variables: VERSION=$(VERSION) CHANNEL=$(CHANNEL) ARCH=$(ARCH)"

all: bundle iso va

$(BUILD_DIR):
	mkdir -p $(BUILD_DIR)

# ---------- mkosi bootstrap ----------

mkosi-live: | $(BUILD_DIR)
	# Stage the installer binary into mkosi.extra-live/ so mkosi's ExtraTrees
	# mechanism copies it into the rootfs at build time. Previous attempt put
	# it in mkosi.build-src/ expecting /work/src access from postinst — but
	# mkosi.postinst.chroot runs INSIDE the nspawn chroot and never sees the
	# host build context, so the copy silently no-op'd and the live ISO
	# shipped without the installer binary.
	@if [ -f $(INSTALLER_BINARY) ]; then \
	  mkdir -p $(CURDIR)/mkosi.extra-live/usr/local/bin && \
	  install -m 0755 $(INSTALLER_BINARY) \
	    $(CURDIR)/mkosi.extra-live/usr/local/bin/novanas-installer ; \
	fi
	$(MKOSI) $(MKOSI_COMMON) \
	    --profile=live \
	    --output-dir=$(MKOSI_LIVE_DIR) \
	    --environment=NOVANAS_PROFILE=live \
	    build

mkosi-update: | $(BUILD_DIR)
	$(MKOSI) $(MKOSI_COMMON) \
	    --profile=update \
	    --output-dir=$(MKOSI_UPDATE_DIR) \
	    --environment=NOVANAS_PROFILE=update \
	    build

# ---------- artefact extraction ----------

live-artifacts: mkosi-live
	$(CURDIR)/build/build-images.sh \
	    --rootfs-dir=$(LIVE_ROOTFS) \
	    --profile=live \
	    --out-dir=$(BUILD_DIR)

update-artifacts: mkosi-update
	$(CURDIR)/build/build-images.sh \
	    --rootfs-dir=$(UPDATE_ROOTFS) \
	    --profile=update \
	    --out-dir=$(BUILD_DIR)

# ---------- RAUC bundle ----------

bundle: update-artifacts
	$(CURDIR)/build/build-rauc-bundle.sh \
	    --version=$(VERSION) --channel=$(CHANNEL) \
	    --rootfs=$(LAYERED_IMG) --boot=$(BOOT_IMG) \
	    --manifest=$(RAUC_DIR)/manifest.raucm \
	    --cert=$(RAUC_DIR)/cert.pem --key=$(RAUC_DIR)/key.pem \
	    --out=$(BUNDLE)

# ---------- Installer ISO ----------

iso: live-artifacts
	$(CURDIR)/build/build-iso.sh \
	    --version=$(VERSION) --channel=$(CHANNEL) \
	    --installer-binary=$(INSTALLER_BINARY) \
	    --squashfs=$(SQUASHFS) \
	    --kernel=$(KERNEL) \
	    --initrd=$(INITRD) \
	    --out=$(ISO)

# iso-only assumes live-artifacts are already on disk (downloaded as CI
# artefacts). Skips the mkosi rebuild.
iso-only: | $(BUILD_DIR)
	@test -f $(SQUASHFS) || { echo "SQUASHFS missing: $(SQUASHFS)" >&2; exit 1; }
	@test -f $(KERNEL)   || { echo "KERNEL missing: $(KERNEL)"     >&2; exit 1; }
	@test -f $(INITRD)   || { echo "INITRD missing: $(INITRD)"     >&2; exit 1; }
	$(CURDIR)/build/build-iso.sh \
	    --version=$(VERSION) --channel=$(CHANNEL) \
	    --installer-binary=$(INSTALLER_BINARY) \
	    --squashfs=$(SQUASHFS) \
	    --kernel=$(KERNEL) \
	    --initrd=$(INITRD) \
	    --out=$(ISO)

# ---------- Virtual appliances (Packer) ----------

va: iso
	$(CURDIR)/build/build-va.sh \
	    --version=$(VERSION) --channel=$(CHANNEL) \
	    --iso=$(ISO) \
	    --packer-dir=$(PACKER_DIR) \
	    --out-dir=$(BUILD_DIR)

va-only: | $(BUILD_DIR)
	@test -f $(ISO) || { echo "ISO not found: $(ISO)" >&2; exit 1; }
	$(CURDIR)/build/build-va.sh \
	    --version=$(VERSION) --channel=$(CHANNEL) \
	    --iso=$(ISO) \
	    --packer-dir=$(PACKER_DIR) \
	    --out-dir=$(BUILD_DIR)

clean:
	rm -rf $(BUILD_DIR) $(WORKSPACE_DIR) $(CURDIR)/mkosi.build-src

lint:
	@echo "==> bash -n on build scripts"
	@for f in $(CURDIR)/build/*.sh $(CURDIR)/tests/*.sh $(ROOTFS_DIR)/usr/local/bin/*.sh $(CURDIR)/mkosi.postinst; do \
	    bash -n "$$f" && echo "ok: $$f"; \
	done
	@echo "==> make -n all"
	@$(MAKE) -n all >/dev/null && echo "ok: make -n all"
	@echo "==> packer validate (if installed)"
	@if command -v packer >/dev/null 2>&1; then \
	    packer validate $(PACKER_DIR)/novanas.pkr.hcl && echo "ok: packer validate"; \
	else \
	    echo "skip: packer not installed"; \
	fi
	@echo "==> mkosi --version (if installed)"
	@if command -v mkosi >/dev/null 2>&1; then mkosi --version; else echo "skip: mkosi not installed"; fi
