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
  efi_firmware_code  = "/usr/share/OVMF/OVMF_CODE.fd"
  efi_firmware_vars  = "/usr/share/OVMF/OVMF_VARS.fd"
  boot_wait          = "10s"

  # The NovaNas installer runs non-interactively when booted with
  # novanas.installer=1 + preseed file shipped in the ISO. We simply wait for
  # shutdown on success.
  shutdown_command   = "true"
  shutdown_timeout   = "30m"

  # No SSH in the installer phase; packer needs some signal. The installer
  # powers the VM off when done. boot_command is empty because GRUB autoselect
  # handles it.
  boot_command       = []
  communicator       = "none"

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
