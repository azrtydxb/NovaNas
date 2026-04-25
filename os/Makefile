# NovaNas OS build orchestrator.
#
# Driven by mkosi (os/mkosi.conf + mkosi.conf.d/). The build graph is:
#
#   mkosi-update    -> build/out/mkosi/update/image/      (rootfs tree)
#   build-images.sh -> rootfs.img / boot.img / kernel.*
#   bundle          -> novanas-<ver>.raucb                (via rauc bundle)
#   iso             -> novanas-<ver>.iso                  (via d-i repack)
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

MKOSI_UPDATE_DIR  := $(BUILD_DIR)/mkosi/update
# mkosi v25 names the output directory after ImageId (set in mkosi.conf), not
# a literal "image".
UPDATE_ROOTFS     := $(MKOSI_UPDATE_DIR)/novanas

LAYERED_IMG  := $(BUILD_DIR)/rootfs.img
BOOT_IMG     := $(BUILD_DIR)/boot.img
BUNDLE       := $(BUILD_DIR)/novanas-$(VERSION).raucb
ISO          := $(BUILD_DIR)/novanas-$(VERSION).iso

export VERSION CHANNEL ARCH BUILD_DIR ROOTFS_DIR RAUC_DIR K3S_VERSION

MKOSI        ?= mkosi
MKOSI_COMMON := \
    --directory=$(CURDIR) \
    --workspace-dir=$(WORKSPACE_DIR) \
    --environment=NOVANAS_VERSION=$(VERSION) \
    --environment=NOVANAS_CHANNEL=$(CHANNEL) \
    --environment=NOVANAS_K3S_VERSION=$(K3S_VERSION)

.PHONY: all mkosi-update update-artifacts \
        bundle iso iso-only va va-only clean lint help

help:
	@echo "NovaNas OS build targets (mkosi-driven):"
	@echo "  make mkosi-update     - Bootstrap RAUC-slot rootfs tree via mkosi"
	@echo "  make update-artifacts - ext4 rootfs.img + boot.img for the bundle"
	@echo "  make bundle           - RAUC .raucb update bundle"
	@echo "  make iso              - Hybrid installer ISO (d-i repack)"
	@echo "  make iso-only         - ISO only (requires bundle already built)"
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

mkosi-update: | $(BUILD_DIR)
	$(MKOSI) $(MKOSI_COMMON) \
	    --profile=update \
	    --output-dir=$(MKOSI_UPDATE_DIR) \
	    --environment=NOVANAS_PROFILE=update \
	    build

# ---------- artefact extraction ----------

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

iso: bundle
	OUT_ISO=$(ISO) RAUC_BUNDLE=$(BUNDLE) $(CURDIR)/build/build-iso.sh

# iso-only assumes the RAUC bundle is already on disk (downloaded as CI
# artefact). Skips the mkosi/bundle rebuild.
iso-only: | $(BUILD_DIR)
	@test -f $(BUNDLE) || { echo "BUNDLE missing: $(BUNDLE)" >&2; exit 1; }
	OUT_ISO=$(ISO) RAUC_BUNDLE=$(BUNDLE) $(CURDIR)/build/build-iso.sh

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
