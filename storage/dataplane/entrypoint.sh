#!/bin/sh
set -e

# DPDK EAL scans /proc/mounts for hugetlbfs entries to locate hugepage
# backing files.  Inside a container the host hugetlbfs mount may not be
# visible in /proc/mounts even when /dev/hugepages is bind-mounted from
# the host.  Re-mount hugetlbfs so DPDK can discover it.
echo "=== hugepage debug ==="
echo "--- /proc/mounts (hugepage entries) ---"
grep -i huge /proc/mounts 2>/dev/null || echo "(none)"
echo "--- /dev/hugepages contents ---"
ls -la /dev/hugepages/ 2>/dev/null || echo "(not accessible)"
echo "--- hugepage meminfo ---"
grep -i huge /proc/meminfo 2>/dev/null || echo "(no meminfo)"
echo "--- /var/run writability ---"
mkdir -p /var/run/dpdk 2>/dev/null && echo "/var/run/dpdk created OK" || echo "FAILED to create /var/run/dpdk"
echo "--- /dev/shm writability ---"
touch /dev/shm/test_write 2>/dev/null && rm /dev/shm/test_write && echo "/dev/shm writable" || echo "FAILED /dev/shm write"
echo "--- /tmp writability ---"
touch /tmp/test_write 2>/dev/null && rm /tmp/test_write && echo "/tmp writable" || echo "FAILED /tmp write"
echo "=== end debug ==="

# Ensure hugetlbfs is visible in /proc/mounts for DPDK EAL.
if ! grep -q hugetlbfs /proc/mounts 2>/dev/null; then
    echo "No hugetlbfs in /proc/mounts, mounting over /dev/hugepages..."
    mount -t hugetlbfs nodev /dev/hugepages || echo "WARNING: mount hugetlbfs failed"
    echo "--- /proc/mounts after mount ---"
    grep -i huge /proc/mounts 2>/dev/null || echo "(still none)"
fi

# Pre-load vfio-pci kernel module for SPDK NVMe device access.
# The NVMe device must be unbound from the kernel nvme driver and bound
# to vfio-pci before SPDK can attach it. Loading the module here avoids
# needing nsenter from the SPDK reactor thread.
echo "Loading vfio-pci module..."
if [ -x /sbin/modprobe ]; then
    modprobe vfio-pci 2>/dev/null || true
else
    # Container may not have modprobe; use nsenter to host (privileged pod)
    nsenter -t 1 -m -- modprobe vfio-pci 2>/dev/null || true
fi
# Verify
if [ -d /sys/bus/pci/drivers/vfio-pci ]; then
    echo "vfio-pci driver loaded"
else
    echo "WARNING: vfio-pci driver not available"
fi

# Pre-load the NBD kernel module so `spdk_nbd_start` can bind chunk-backed
# volumes to /dev/nbdN (used by the Go meta service to mount the metadata
# volume and open BadgerDB on top of it). nbds_max=16 matches
# `NbdManager::MAX_NBD_SLOTS`; raise in lockstep if more are needed.
echo "Loading nbd kernel module (nbds_max=16)..."
if [ -x /sbin/modprobe ]; then
    modprobe nbd nbds_max=16 2>/dev/null || true
else
    nsenter -t 1 -m -- modprobe nbd nbds_max=16 2>/dev/null || true
fi
if [ -e /dev/nbd0 ]; then
    echo "nbd module loaded (/dev/nbd0 present)"
else
    echo "WARNING: nbd device nodes not available — ExportMetadataVolumeNBD will fail"
fi

exec /usr/local/bin/novanas-dataplane "$@"
