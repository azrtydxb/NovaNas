variable "version" {
  type        = string
  description = "NovaNas release version (CalVer YY.MM.patch)."
}

variable "channel" {
  type        = string
  description = "Release channel: dev | edge | beta | stable | lts."
  default     = "dev"
}

variable "iso_path" {
  type        = string
  description = "Absolute path to the NovaNas installer ISO to boot."
}

variable "out_dir" {
  type        = string
  description = "Directory for produced artifacts."
}

variable "memory_mb" {
  type    = number
  default = 4096
}

variable "cpus" {
  type    = number
  default = 4
}

variable "disk_size_mb" {
  type    = number
  default = 32768
}
