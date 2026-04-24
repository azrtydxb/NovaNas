packer {
  required_plugins {
    qemu = {
      source  = "github.com/hashicorp/qemu"
      version = ">= 1.1.0"
    }
  }
}

source "qemu" "novanas" {
  vm_name            = "novanas-${var.version}"
  iso_url            = var.iso_path
  iso_checksum       = "none"
  disk_size          = "${var.disk_size_mb}M"
  format             = "raw"
  accelerator        = "kvm"
  headless           = true
  memory             = var.memory_mb
  cpus               = var.cpus
  net_device         = "virtio-net"
  disk_interface     = "virtio"
  machine_type       = "q35"
  efi_boot           = true
  # Ubuntu 24.04 (noble) ovmf 2024.02 ships the 4M-variant firmware as
  # the canonical path; OVMF_CODE.fd / OVMF_VARS.fd without suffix no
  # longer exist. The 4M variant is what Q35 + modern qemu expect.
  efi_firmware_code  = "/usr/share/OVMF/OVMF_CODE_4M.fd"
  efi_firmware_vars  = "/usr/share/OVMF/OVMF_VARS_4M.fd"
  boot_wait          = "10s"

  # The NovaNas installer runs non-interactively when booted with
  # novanas.installer=1 + preseed file shipped in the ISO. We simply wait for
  # shutdown on success.
  shutdown_command   = "true"
  # 8m gives the watchdog (5m) + a little slack for qemu teardown.
  # Dry-run install powers off in <1m on the happy path.
  shutdown_timeout   = "8m"

  # No SSH in the installer phase; packer needs some signal. The installer
  # powers the VM off when done. boot_command is empty because GRUB autoselect
  # handles it.
  boot_command       = []
  communicator       = "none"

  # Pipe the VM's serial console (ttyS0) to a log file so the CI job can
  # upload it on failure. Kernel cmdline in build-iso.sh already has
  # console=ttyS0,115200n8 on the default menuentry so this captures the
  # full boot sequence including live-boot progress and installer logs.
  qemuargs = [
    ["-serial", "file:${var.out_dir}/packer-serial.log"],
  ]

  output_directory   = "${var.out_dir}/packer-novanas-${var.version}"
}

build {
  name    = "novanas"
  sources = ["source.qemu.novanas"]

  post-processor "shell-local" {
    inline = [
      "cp '${var.out_dir}/packer-novanas-${var.version}/novanas-${var.version}' '${var.out_dir}/novanas-${var.version}.raw'",
    ]
  }
}
