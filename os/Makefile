# NovaNas OS build orchestrator.
# Delegates to build/*.sh scripts. Image builds require root + loop devices;
# typically invoked inside a privileged build container from CI.

SHELL        := /bin/bash
.SHELLFLAGS  := -eu -o pipefail -c

VERSION      ?= 0.0.0-dev
CHANNEL      ?= dev
ARCH         ?= amd64
BUILD_DIR    ?= $(CURDIR)/build/out
ROOTFS_DIR   ?= $(CURDIR)/rootfs
RECIPE_DIR   ?= $(CURDIR)/recipes
RAUC_DIR     ?= $(CURDIR)/rauc
PACKER_DIR   ?= $(CURDIR)/packer

INSTALLER_BINARY ?= $(CURDIR)/../installer/bin/novanas-installer

BASE_TAR     := $(BUILD_DIR)/base-rootfs-$(VERSION).tar
LAYERED_IMG  := $(BUILD_DIR)/rootfs-$(VERSION).img
BOOT_IMG     := $(BUILD_DIR)/boot-$(VERSION).img
BUNDLE       := $(BUILD_DIR)/novanas-$(VERSION).raucb
ISO          := $(BUILD_DIR)/novanas-$(VERSION).iso
VA_RAW       := $(BUILD_DIR)/novanas-$(VERSION).raw
VA_QCOW2     := $(BUILD_DIR)/novanas-$(VERSION).qcow2
VA_VMDK      := $(BUILD_DIR)/novanas-$(VERSION).vmdk
VA_OVA       := $(BUILD_DIR)/novanas-$(VERSION).ova

export VERSION CHANNEL ARCH BUILD_DIR ROOTFS_DIR RAUC_DIR INSTALLER_BINARY

.PHONY: all base layered bundle iso iso-only va va-only clean lint help

help:
	@echo "NovaNas OS build targets:"
	@echo "  make base       - Debian minimal rootfs tarball"
	@echo "  make layered    - NovaNas-flavored rootfs image (ext4)"
	@echo "  make bundle     - RAUC update bundle (.raucb)"
	@echo "  make iso        - Hybrid installer ISO"
	@echo "  make va         - Virtual appliance images (raw/qcow2/vmdk/ova)"
	@echo "  make all        - Full matrix"
	@echo "  make clean      - Remove $(BUILD_DIR)"
	@echo "  make lint       - Static checks (bash -n, make -n, etc.)"
	@echo ""
	@echo "Variables: VERSION=$(VERSION) CHANNEL=$(CHANNEL) ARCH=$(ARCH)"

all: bundle iso va

$(BUILD_DIR):
	mkdir -p $(BUILD_DIR)

base: | $(BUILD_DIR)
	$(CURDIR)/build/build-rootfs.sh --stage=base \
		--version=$(VERSION) --channel=$(CHANNEL) --arch=$(ARCH) \
		--out=$(BASE_TAR)

layered: base
	$(CURDIR)/build/build-rootfs.sh --stage=layered \
		--version=$(VERSION) --channel=$(CHANNEL) --arch=$(ARCH) \
		--in=$(BASE_TAR) --out=$(LAYERED_IMG) --boot-out=$(BOOT_IMG)

bundle: layered
	$(CURDIR)/build/build-rauc-bundle.sh \
		--version=$(VERSION) --channel=$(CHANNEL) \
		--rootfs=$(LAYERED_IMG) --boot=$(BOOT_IMG) \
		--manifest=$(RAUC_DIR)/manifest.raucm \
		--cert=$(RAUC_DIR)/cert.pem --key=$(RAUC_DIR)/key.pem \
		--out=$(BUNDLE)

iso: bundle
	$(CURDIR)/build/build-iso.sh \
		--version=$(VERSION) --channel=$(CHANNEL) \
		--installer-binary=$(INSTALLER_BINARY) \
		--bundle=$(BUNDLE) \
		--out=$(ISO)

# iso-only assumes $(BUNDLE) already exists (e.g. downloaded as a CI
# artifact) and skips the base → layered → bundle rebuild chain.
iso-only: | $(BUILD_DIR)
	@test -f $(BUNDLE) || { echo "BUNDLE not found: $(BUNDLE)" >&2; exit 1; }
	$(CURDIR)/build/build-iso.sh \
		--version=$(VERSION) --channel=$(CHANNEL) \
		--installer-binary=$(INSTALLER_BINARY) \
		--bundle=$(BUNDLE) \
		--out=$(ISO)

va: iso
	$(CURDIR)/build/build-va.sh \
		--version=$(VERSION) --channel=$(CHANNEL) \
		--iso=$(ISO) \
		--packer-dir=$(PACKER_DIR) \
		--out-dir=$(BUILD_DIR)

# va-only assumes $(ISO) already exists. Mirrors iso-only for the VA job.
va-only: | $(BUILD_DIR)
	@test -f $(ISO) || { echo "ISO not found: $(ISO)" >&2; exit 1; }
	$(CURDIR)/build/build-va.sh \
		--version=$(VERSION) --channel=$(CHANNEL) \
		--iso=$(ISO) \
		--packer-dir=$(PACKER_DIR) \
		--out-dir=$(BUILD_DIR)

clean:
	rm -rf $(BUILD_DIR)

lint:
	@echo "==> bash -n on build scripts"
	@for f in $(CURDIR)/build/*.sh $(CURDIR)/tests/*.sh $(ROOTFS_DIR)/usr/local/bin/*.sh; do \
		bash -n "$$f" && echo "ok: $$f"; \
	done
	@echo "==> make -n all"
	@$(MAKE) -n all >/dev/null && echo "ok: make -n all"
	@echo "==> packer validate (if packer is installed)"
	@if command -v packer >/dev/null 2>&1; then \
		packer validate $(PACKER_DIR)/novanas.pkr.hcl && echo "ok: packer validate"; \
	else \
		echo "skip: packer not installed"; \
	fi
