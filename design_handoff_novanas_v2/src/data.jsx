/* globals React */

// ─────── Storage ───────
const POOLS = [
  { name: "fast",    tier: "hot",  disks: 8,  used: 4.9e12,  total: 7.6e12, protection: "rep×2", devices: "NVMe", state: "Healthy", throughput: { r: 1420, w: 680 }, iops: { r: 24100, w: 8900 }, scrubLast: "3 days ago", scrubNext: "in 4 days" },
  { name: "bulk",    tier: "warm", disks: 10, used: 23.1e12, total: 60e12,  protection: "EC 6+2", devices: "HDD",  state: "Healthy", throughput: { r: 340, w: 210 },  iops: { r: 1400, w: 900 },  scrubLast: "5 days ago", scrubNext: "in 2 days" },
  { name: "archive", tier: "cold", disks: 6,  used: 14e12,   total: 30e12,  protection: "EC 4+2", devices: "HDD",  state: "Healthy", throughput: { r: 120, w: 80 },   iops: { r: 420, w: 260 },   scrubLast: "12 days ago", scrubNext: "in 18 days" },
  { name: "meta",    tier: "hot",  disks: 3,  used: 12e9,    total: 500e9,  protection: "rep×3", devices: "NVMe", state: "Healthy", throughput: { r: 60, w: 30 },    iops: { r: 8200, w: 2100 }, scrubLast: "1 day ago", scrubNext: "in 6 days" },
];

const VDEV_TREE = {
  fast: [
    { name: "mirror-0", type: "mirror-2", disks: ["nvme0n1","nvme1n1"], state: "ONLINE" },
    { name: "mirror-1", type: "mirror-2", disks: ["nvme2n1","nvme3n1"], state: "ONLINE" },
    { name: "mirror-2", type: "mirror-2", disks: ["nvme4n1","nvme5n1"], state: "ONLINE" },
    { name: "log",      type: "log",      disks: ["nvme6n1"], state: "ONLINE" },
    { name: "spare",    type: "spare",    disks: ["nvme7n1"], state: "AVAIL" },
  ],
  bulk: [
    { name: "raidz2-0", type: "raidz2-8", disks: ["sda","sdb","sdc","sdd","sde","sdf","sdg","sdh"], state: "ONLINE" },
    { name: "cache",    type: "cache",    disks: ["sdi","sdj"], state: "ONLINE" },
  ],
  archive: [
    { name: "raidz2-0", type: "raidz2-6", disks: ["sdk","sdl","sdm","sdn","sdo","sdp"], state: "ONLINE" },
  ],
  meta: [
    { name: "mirror-0", type: "mirror-3", disks: ["nvme8n1","nvme9n1","nvme10n1"], state: "ONLINE" },
  ],
};

const DISKS = Array.from({ length: 24 }, (_, i) => {
  const slot = i + 1;
  if (slot >= 23) return { slot, state: "EMPTY" };
  const isNvme = slot <= 8;
  const isDegraded = slot === 13;
  return {
    slot,
    model: isNvme ? "SAMSUNG PM9A3" : (slot <= 16 ? "WDC ULTRASTAR 20" : "SEAGATE EXOS X20"),
    serial: `S${(7349824 + slot * 137).toString(36).toUpperCase()}-${1000+slot}`,
    cap: isNvme ? 1920e9 : 20e12,
    class: isNvme ? "NVMe" : "HDD",
    pool: isNvme ? "fast" : (slot <= 18 ? "bulk" : "archive"),
    state: slot === 8 ? "SPARE" : (isDegraded ? "DEGRADED" : "ACTIVE"),
    temp: 34 + (slot % 12),
    hours: 8000 + slot * 130,
    reason: isDegraded ? "5 reallocated sectors" : null,
    smart: {
      reallocated: isDegraded ? 5 : 0,
      pending: 0,
      uncorrectable: 0,
      passed: !isDegraded,
    },
  };
});

const DATASETS = [
  { name: "family-media",  pool: "bulk",    used: 4.1e12, quota: 6e12,  proto: "SMB+NFS", snap: 142, enc: true,  comp: "lz4", recordsize: "1M",   atime: "off" },
  { name: "pascal/docs",   pool: "fast",    used: 92e9,   quota: 200e9, proto: "SMB",     snap: 96,  enc: true,  comp: "zstd", recordsize: "128K", atime: "on"  },
  { name: "pascal/photos", pool: "bulk",    used: 640e9,  quota: 1e12,  proto: "SMB",     snap: 72,  enc: true,  comp: "zstd", recordsize: "1M",   atime: "off" },
  { name: "vm-disks",      pool: "fast",    used: 412e9,  quota: 1e12,  proto: "block",   snap: 54,  enc: true,  comp: "lz4", recordsize: "16K",  atime: "off" },
  { name: "app-configs",   pool: "fast",    used: 8.2e9,  quota: 80e9,  proto: "block",   snap: 320, enc: true,  comp: "zstd", recordsize: "32K",  atime: "off" },
  { name: "isos",          pool: "bulk",    used: 84e9,   quota: 200e9, proto: "NFS",     snap: 4,   enc: false, comp: "off",  recordsize: "1M",   atime: "off" },
  { name: "backups",       pool: "archive", used: 6.4e12, quota: 10e12, proto: "bucket",  snap: 28,  enc: true,  comp: "zstd", recordsize: "1M",   atime: "off" },
];

const SNAPSHOTS = [
  { name: "family-media@auto-14:58",   pool: "bulk",    size: 1.4e9, t: "2 min ago",  hold: false, schedule: "auto-15min" },
  { name: "pascal/docs@daily-04:00",   pool: "fast",    size: 240e6, t: "10 h ago",   hold: true,  schedule: "daily" },
  { name: "vm-disks@pre-update",       pool: "fast",    size: 4.1e9, t: "yesterday",  hold: true,  schedule: "manual" },
  { name: "backups@weekly-W17",        pool: "archive", size: 18e9,  t: "3 d ago",    hold: false, schedule: "weekly" },
  { name: "pascal/photos@trip",        pool: "bulk",    size: 3.2e9, t: "1 w ago",    hold: false, schedule: "manual" },
  { name: "app-configs@pre-immich-up", pool: "fast",    size: 12e6,  t: "1 w ago",    hold: false, schedule: "manual" },
  { name: "family-media@daily-04:00",  pool: "bulk",    size: 8.4e9, t: "today",      hold: false, schedule: "daily" },
];

const SNAPSHOT_SCHEDULES = [
  { id: "s1", name: "auto-15min",   datasets: ["family-media","pascal/docs"],     cron: "*/15 * * * *",   keep: 96,  enabled: true },
  { id: "s2", name: "daily",        datasets: ["family-media","pascal/*","vm-disks"], cron: "0 4 * * *",  keep: 30,  enabled: true },
  { id: "s3", name: "weekly",       datasets: ["backups","family-media"],         cron: "0 5 * * 0",      keep: 12,  enabled: true },
  { id: "s4", name: "monthly",      datasets: ["backups"],                        cron: "0 6 1 * *",      keep: 12,  enabled: true },
  { id: "s5", name: "pre-app-update", datasets: ["app-configs"],                  cron: "manual",         keep: 8,   enabled: true },
];

const SCRUB_POLICIES = [
  { id: "p1", name: "weekly-fast",     pools: ["fast","meta"],    cron: "0 2 * * 0", priority: "high",   builtin: false },
  { id: "p2", name: "biweekly-bulk",   pools: ["bulk"],           cron: "0 3 */14 * *", priority: "medium", builtin: false },
  { id: "p3", name: "monthly-archive", pools: ["archive"],        cron: "0 4 1 * *", priority: "low",    builtin: false },
  { id: "p4", name: "default",         pools: ["*"],              cron: "0 4 * * 0", priority: "medium", builtin: true },
];

// ─────── Replication ───────
const REPL_TARGETS = [
  { id: "t1", name: "offsite-borg",    host: "backup.example.net", protocol: "ssh+zfs",   ssh_user: "novanas", port: 22 },
  { id: "t2", name: "b2-cloud",        host: "s3.us-west-002.backblazeb2.com", protocol: "s3", region: "us-west-002" },
  { id: "t3", name: "secondary-nas",   host: "nas2.lan", protocol: "ssh+zfs",   ssh_user: "novanas", port: 22 },
];

const REPL_JOBS = [
  { id: "r1", name: "photos→offsite",   source: "pascal/photos",  target: "offsite-borg",  schedule: "0 1 * * *", lastRun: "12:40 today", state: "OK",       lastBytes: 1.4e9, lastDur: "8m 12s" },
  { id: "r2", name: "docs→offsite",     source: "pascal/docs",    target: "offsite-borg",  schedule: "0 1 * * *", lastRun: "12:39 today", state: "OK",       lastBytes: 320e6, lastDur: "1m 04s" },
  { id: "r3", name: "backups→b2",       source: "backups",        target: "b2-cloud",      schedule: "0 3 * * 0", lastRun: "Sun 03:00",   state: "OK",       lastBytes: 84e9,  lastDur: "2h 18m" },
  { id: "r4", name: "vm-disks→nas2",    source: "vm-disks",       target: "secondary-nas", schedule: "0 */6 * * *", lastRun: "12:00 today", state: "RUNNING", lastBytes: 0,     lastDur: "—" },
  { id: "r5", name: "family→offsite",   source: "family-media",   target: "offsite-borg",  schedule: "0 2 * * *", lastRun: "02:00 today", state: "FAILED",   lastBytes: 0,     lastDur: "0s",   error: "ssh: connection timeout after 60s" },
];

// ─────── Encryption ───────
const ENCRYPTED_DATASETS = [
  { name: "family-media",  status: "available",  keyformat: "raw",        keylocation: "tpm:sealed",  rotated: "62 days ago" },
  { name: "pascal/docs",   status: "available",  keyformat: "raw",        keylocation: "tpm:sealed",  rotated: "30 days ago" },
  { name: "pascal/photos", status: "available",  keyformat: "raw",        keylocation: "tpm:sealed",  rotated: "30 days ago" },
  { name: "vm-disks",      status: "available",  keyformat: "raw",        keylocation: "tpm:sealed",  rotated: "92 days ago" },
  { name: "app-configs",   status: "available",  keyformat: "passphrase", keylocation: "prompt",       rotated: "—" },
  { name: "backups",       status: "unavailable",keyformat: "raw",        keylocation: "tpm:sealed",   rotated: "8 days ago" },
];

// ─────── Disks/Network ───────
const NETWORK_INTERFACES = [
  { name: "eno1",    type: "physical", state: "UP",   ipv4: "192.168.1.10/24",  mac: "52:54:00:9a:1c:01", mtu: 9000, speed: "10 Gb/s", driver: "ixgbe" },
  { name: "eno2",    type: "physical", state: "UP",   ipv4: "192.168.1.11/24",  mac: "52:54:00:9a:1c:02", mtu: 9000, speed: "10 Gb/s", driver: "ixgbe" },
  { name: "ens3f0",  type: "physical", state: "UP",   ipv4: "10.0.10.1/24",     mac: "0c:42:a1:48:0b:30", mtu: 9000, speed: "100 Gb/s", driver: "mlx5_core" },
  { name: "ens3f1",  type: "physical", state: "DOWN", ipv4: null,                mac: "0c:42:a1:48:0b:31", mtu: 1500, speed: "100 Gb/s", driver: "mlx5_core" },
  { name: "bond0",   type: "bond",     state: "UP",   ipv4: "192.168.1.10/24",   mac: "52:54:00:9a:1c:01", mtu: 9000, speed: "20 Gb/s", driver: "bonding", members: ["eno1","eno2"], mode: "802.3ad" },
  { name: "vlan20",  type: "vlan",     state: "UP",   ipv4: "10.20.0.1/24",      mac: "52:54:00:9a:1c:01", mtu: 1500, speed: "—",         driver: "8021q",   parent: "bond0", tag: 20 },
];

const RDMA_DEVICES = [
  { name: "mlx5_0", port: 1, state: "ACTIVE", speed: "100 Gb/s", lid: 0, gid: "fe80::e42:a1ff:fe48:b30" },
  { name: "mlx5_0", port: 2, state: "DOWN",   speed: "100 Gb/s", lid: 0, gid: "fe80::e42:a1ff:fe48:b31" },
];

// ─────── Apps / Plugins / VMs ───────
// First-pass AppCenter mock (superseded by Package Center in apps-installable).
// `cat` here is a backend displayCategory value — same enum as the Package
// Center uses (internal/plugins/manifest.go DisplayCategory).
const APPS = [
  { slug: "plex",          name: "Plex",          cat: "multimedia",    color: "oklch(0.62 0.16 70)",  installed: true,  ver: "1.41.2" },
  { slug: "jellyfin",      name: "Jellyfin",      cat: "multimedia",    color: "oklch(0.62 0.16 290)", installed: false, ver: "10.9.7" },
  { slug: "immich",        name: "Immich",        cat: "photos",        color: "oklch(0.62 0.16 240)", installed: true,  ver: "1.140.0" },
  { slug: "nextcloud",     name: "Nextcloud",     cat: "files",         color: "oklch(0.62 0.16 220)", installed: true,  ver: "29.0.4" },
  { slug: "homeassistant", name: "Home Assistant",cat: "home",          color: "oklch(0.62 0.16 200)", installed: true,  ver: "2026.4" },
  { slug: "gitea",         name: "Gitea",         cat: "developer",     color: "oklch(0.62 0.16 160)", installed: false, ver: "1.22.0" },
  { slug: "vaultwarden",   name: "Vaultwarden",   cat: "security",      color: "oklch(0.62 0.16 20)",  installed: true,  ver: "1.32.0" },
  { slug: "paperless",     name: "Paperless",     cat: "productivity",  color: "oklch(0.62 0.16 45)",  installed: false, ver: "2.13.0" },
  { slug: "adguard",       name: "AdGuard",       cat: "network",       color: "oklch(0.62 0.16 140)", installed: true,  ver: "0.107" },
  { slug: "frigate",       name: "Frigate",       cat: "surveillance",  color: "oklch(0.62 0.16 10)",  installed: false, ver: "0.14.0" },
  { slug: "sonarr",        name: "Sonarr",        cat: "multimedia",    color: "oklch(0.62 0.16 205)", installed: true,  ver: "4.0.9" },
  { slug: "grafana",       name: "Grafana",       cat: "observability", color: "oklch(0.62 0.16 30)",  installed: false, ver: "11.2.0" },
];

const WORKLOADS = [
  { release: "immich",        chart: "immich/immich",      version: "1.140.0", ns: "apps",   status: "Deployed", updated: "2 d ago",  pods: "5/5",   cpu: "0.42",  mem: "2.4 GiB" },
  { release: "plex",          chart: "plex/plex",          version: "1.41.2",  ns: "apps",   status: "Deployed", updated: "12 d ago", pods: "1/1",   cpu: "0.28",  mem: "1.8 GiB" },
  { release: "homeassistant", chart: "ha/homeassistant",   version: "2026.4",  ns: "apps",   status: "Deployed", updated: "1 d ago",  pods: "1/1",   cpu: "0.04",  mem: "612 MiB" },
  { release: "nextcloud",     chart: "nextcloud/nextcloud",version: "29.0.4",  ns: "apps",   status: "Deployed", updated: "5 d ago",  pods: "3/3",   cpu: "0.18",  mem: "1.1 GiB" },
  { release: "vaultwarden",   chart: "vw/vaultwarden",     version: "1.32.0",  ns: "apps",   status: "Deployed", updated: "9 d ago",  pods: "1/1",   cpu: "0.01",  mem: "82 MiB" },
  { release: "adguard",       chart: "adguard/adguard",    version: "0.107",   ns: "infra",  status: "Deployed", updated: "3 d ago",  pods: "2/2",   cpu: "0.02",  mem: "120 MiB" },
  { release: "sonarr",        chart: "media/sonarr",       version: "4.0.9",   ns: "apps",   status: "Pending",  updated: "now",      pods: "0/1",   cpu: "—",      mem: "—" },
];

const VMS = [
  { name: "windows-11", os: "Win 11",       cpu: 4, ram: 8192,  state: "Running", ip: "192.168.1.101", uptime: "3d 4h",  disk: "120 GiB", mac: "52:54:00:a1:01:11" },
  { name: "ubuntu-dev", os: "Ubuntu 24.04", cpu: 8, ram: 16384, state: "Running", ip: "192.168.1.102", uptime: "12d 2h", disk: "200 GiB", mac: "52:54:00:a1:01:12" },
  { name: "pfsense",    os: "FreeBSD 14",   cpu: 2, ram: 2048,  state: "Running", ip: "192.168.1.1",   uptime: "32d 7h", disk: "16 GiB",  mac: "52:54:00:a1:01:13" },
  { name: "ha-vm",      os: "HA OS 12",     cpu: 2, ram: 4096,  state: "Running", ip: "192.168.1.104", uptime: "8d 1h",  disk: "32 GiB",  mac: "52:54:00:a1:01:14" },
  { name: "win-gaming", os: "Win 11",       cpu: 8, ram: 32768, state: "Stopped", ip: null,             uptime: "—",      disk: "500 GiB", mac: "52:54:00:a1:01:15" },
  { name: "k3s-test",   os: "Talos 1.7",    cpu: 4, ram: 8192,  state: "Paused",  ip: "192.168.1.106", uptime: "1h",     disk: "40 GiB",  mac: "52:54:00:a1:01:16" },
];

const VM_TEMPLATES = [
  { name: "ubuntu-24.04-server",  os: "Ubuntu 24.04", cpu: 2, ram: 4096,  disk: 40,  source: "cloud-image" },
  { name: "ubuntu-24.04-desktop", os: "Ubuntu 24.04", cpu: 4, ram: 8192,  disk: 80,  source: "iso" },
  { name: "windows-11-22h2",      os: "Windows 11",   cpu: 4, ram: 8192,  disk: 120, source: "iso" },
  { name: "talos-1.7",            os: "Talos Linux",  cpu: 2, ram: 4096,  disk: 16,  source: "iso" },
  { name: "freebsd-14",           os: "FreeBSD 14",   cpu: 2, ram: 2048,  disk: 16,  source: "iso" },
];

const VM_SNAPSHOTS = [
  { name: "windows-11/pre-update",   vm: "windows-11", t: "2 d ago",  size: "8.4 GiB" },
  { name: "ubuntu-dev/clean-install",vm: "ubuntu-dev", t: "5 d ago",  size: "12 GiB" },
  { name: "windows-11/games-clean",  vm: "windows-11", t: "1 w ago",  size: "11 GiB" },
];

const PLUGINS = [
  { name: "novanas-cockpit",     ver: "1.4.2",  source: "novanas-official", status: "running",  perms: ["nova:system:read","nova:logs:read"],                     deps: [],                      updated: "2 d ago" },
  { name: "novanas-photo-ai",    ver: "0.8.1",  source: "novanas-official", status: "running",  perms: ["nova:storage:read","nova:system:read"],                  deps: ["novanas-ml-runtime"],  updated: "5 d ago" },
  { name: "novanas-ml-runtime",  ver: "1.0.0",  source: "novanas-official", status: "running",  perms: ["nova:system:read"],                                       deps: [],                      updated: "5 d ago" },
  { name: "tc-prometheus",       ver: "2.55.0", source: "truecharts",       status: "stopped",  perms: ["nova:system:read","nova:logs:read"],                     deps: [],                      updated: "12 d ago" },
  { name: "internal-billing",    ver: "0.3.0",  source: "internal-mirror",  status: "error",    perms: ["nova:audit:read","nova:sessions:read"],                  deps: [],                      updated: "yesterday", error: "container exited (1)" },
];

// Permissions: nova-api scope strings (see internal/auth/rbac.go).
//   nova:<domain>:<verb> where verb ∈ {read, write, admin, recover}.
//
// displayCategory: one of the 14 values from internal/plugins/manifest.go
// DisplayCategory enum — must match exactly. (privilegeCategory is the
// orthogonal access-axis used by the engine; not surfaced here.)
//
// Per-plugin "signed" flags do NOT exist in the backend — trust is
// established at the marketplace level via cosign verification of the
// index, then propagated to every artifact in that index. Likewise no
// rating/downloads (no marketplace-stats service yet).
const MARKETPLACE_PLUGINS = [
  { name: "novanas-photo-ai",    ver: "0.8.1",  source: "novanas-official", displayCategory: "photos",        author: "NovaNAS Project", desc: "On-device face & object recognition for Immich and File Station.", tags: ["ai","photos","face-recognition"], installed: true,  perms: ["nova:storage:read","nova:system:read"],                   deps: ["novanas-ml-runtime"], size: 184_000_000 },
  { name: "novanas-cockpit",     ver: "1.4.2",  source: "novanas-official", displayCategory: "utilities",     author: "NovaNAS Project", desc: "Embedded Cockpit terminal & systemd inspector for power users.",   tags: ["admin","terminal","systemd"],     installed: true,  perms: ["nova:system:read","nova:logs:read"],                      deps: [],                      size:  41_000_000 },
  { name: "novanas-uptimekuma",  ver: "1.23.13",source: "novanas-official", displayCategory: "observability", author: "NovaNAS Project", desc: "Service-uptime monitoring with notifications and status pages.",   tags: ["monitoring","uptime"],            installed: false, perms: ["nova:network:read","nova:notifications:write"],          deps: [],                      size:  72_000_000 },
  { name: "novanas-tailscale",   ver: "1.74.0", source: "novanas-official", displayCategory: "network",       author: "NovaNAS Project", desc: "Mesh-VPN integration with magic-DNS and subnet routing.",          tags: ["vpn","mesh","wireguard"],         installed: false, perms: ["nova:network:write","nova:system:write"],                deps: [],                      size:  28_000_000 },
  { name: "novanas-collabora",   ver: "23.05",  source: "novanas-official", displayCategory: "productivity",  author: "NovaNAS Project", desc: "Office documents online — connects to Nextcloud.",                  tags: ["office","collaboration"],         installed: false, perms: ["nova:network:read"],                                      deps: [],                      size: 312_000_000 },
  { name: "tc-prometheus",       ver: "2.55.0", source: "truecharts",       displayCategory: "observability", author: "TrueCharts",      desc: "Prometheus metrics scraper, federated cluster mode.",              tags: ["metrics","prometheus"],           installed: true,  perms: ["nova:system:read","nova:logs:read"],                      deps: [],                      size:  96_000_000 },
  { name: "tc-loki",             ver: "3.2.0",  source: "truecharts",       displayCategory: "observability", author: "TrueCharts",      desc: "Loki log aggregator — long-term storage with S3 backend.",         tags: ["logs","loki"],                    installed: false, perms: ["nova:logs:read","nova:storage:read"],                     deps: [],                      size:  88_000_000 },
  { name: "tc-traefik",          ver: "3.1.0",  source: "truecharts",       displayCategory: "network",       author: "TrueCharts",      desc: "Cloud-native reverse proxy with auto-cert.",                       tags: ["proxy","ingress","tls"],          installed: false, perms: ["nova:network:write"],                                     deps: [],                      size:  44_000_000 },
  { name: "ct-pihole",           ver: "2024.07",source: "community-trust",  displayCategory: "network",       author: "@netadmin42",     desc: "Network-wide DNS sinkhole with web admin.",                        tags: ["dns","adblock"],                  installed: false, perms: ["nova:network:read","nova:network:write"],                deps: [],                      size:  19_000_000 },
  { name: "ct-radarr-fork",      ver: "5.4.6",  source: "community-trust",  displayCategory: "multimedia",    author: "@mediafan",       desc: "Movie collection manager — community fork.",                       tags: ["movies","arr"],                   installed: false, perms: ["nova:storage:write","nova:network:read"],                deps: [],                      size:  56_000_000 },
  { name: "internal-billing",    ver: "0.3.0",  source: "internal-mirror",  displayCategory: "productivity",  author: "Acme IT",         desc: "Internal billing dashboard for tenant chargeback.",                tags: ["billing","internal"],             installed: true,  perms: ["nova:audit:read","nova:sessions:read"],                  deps: [],                      size:  22_000_000 },
  { name: "internal-monitoring", ver: "1.2.0",  source: "internal-mirror",  displayCategory: "observability", author: "Acme IT",         desc: "Internal monitoring stack with custom dashboards.",                tags: ["monitoring","grafana"],           installed: false, perms: ["nova:logs:read","nova:system:read"],                     deps: [],                      size: 134_000_000 },
];

// trustKeyFingerprint = lowercase hex sha256 of the cosign public key PEM
// bytes (the backend stores the PEM in marketplaces.trust_key_pem; the
// fingerprint is derived for display). Format: `sha256:<64-hex>`.
//
// pluginCount is derived from the merged index — render as a count when
// the engine returns one, otherwise omit.
const MARKETPLACES = [
  { id: "novanas-official", name: "NovaNAS Official", url: "https://raw.githubusercontent.com/azrtydxb/NovaNas-packages/main/index.json", trustKeyFingerprint: "sha256:bd2f06c1a4ee71a3c19f8b3c79d1c1e5f4e02bb5c8db1c18d7c4fa9e6e01b7d3", added: "boot",      locked: true,  enabled: true,  pluginCount: 142 },
  { id: "truecharts",       name: "TrueCharts",       url: "https://truecharts.org/charts/index.json",                                    trustKeyFingerprint: "sha256:1c2e9f813a047d22e4b1089212fe33ab5d70c4192a8b6f01c3a0e74d92f8a116", added: "82 d ago", locked: false, enabled: true,  pluginCount: 380 },
  { id: "community-trust",  name: "Community Trust",  url: "https://community.novanas.io/store/index.json",                              trustKeyFingerprint: "sha256:ab447d1102448e924c0171ff8821deaf2d5c0e9b71a3f4815e9c2087113b6420", added: "30 d ago", locked: false, enabled: true,  pluginCount: 64  },
  { id: "internal-mirror",  name: "Acme Internal",    url: "https://store.acme.internal/index.json",                                      trustKeyFingerprint: "sha256:7f013b2ac8011e449023bb127e912a4406d2c91f3b4e8a705fc1e9d81b3a4221", added: "12 d ago", locked: false, enabled: true,  pluginCount: 8   },
  { id: "experimental",     name: "Experimental Lab", url: "https://lab.example.org/index.json",                                          trustKeyFingerprint: "sha256:00bbe8112c449f017d9301ea44889912b6e91a472f0c1d3e845a9f1a0b6c2734", added: "3 d ago",  locked: false, enabled: false, pluginCount: 22  },
];

// ─────── Identity / Sessions ───────
const USERS = [
  { name: "pascal",   role: "nova-admin",     email: "pascal@watteel.com",   created: "2024-01-04", lastLogin: "12:02 today", mfa: true,  status: "active" },
  { name: "anna",     role: "nova-operator",  email: "anna@watteel.com",     created: "2024-02-14", lastLogin: "yesterday",   mfa: true,  status: "active" },
  { name: "viewer",   role: "nova-viewer",    email: "viewer@watteel.com",   created: "2024-04-22", lastLogin: "8 d ago",     mfa: false, status: "active" },
  { name: "alerts",   role: "service",        email: "—",                     created: "2024-05-01", lastLogin: "automated",   mfa: false, status: "active" },
  { name: "deploy",   role: "service",        email: "—",                     created: "2024-06-12", lastLogin: "automated",   mfa: false, status: "active" },
  { name: "old-user", role: "nova-viewer",    email: "—",                     created: "2023-08-01", lastLogin: "6 mo ago",    mfa: false, status: "disabled" },
];

const SESSIONS = [
  { id: "sess-a1f2", user: "pascal", ip: "192.168.1.50",   ua: "Firefox 128 / macOS",   started: "08:14 today",  current: true },
  { id: "sess-9c44", user: "pascal", ip: "192.168.1.50",   ua: "novanas-cli/2.4",       started: "11:02 today",  current: false },
  { id: "sess-b021", user: "anna",   ip: "192.168.1.62",   ua: "Chrome 130 / Windows",  started: "yesterday",     current: false },
  { id: "sess-4d80", user: "deploy", ip: "10.0.10.8",       ua: "novanas-cli/2.4",      started: "2 h ago",       current: false },
];

const LOGIN_HISTORY = [
  { at: "12:02 today",   user: "pascal", ip: "192.168.1.50",  result: "success",  method: "password+totp" },
  { at: "08:14 today",   user: "pascal", ip: "192.168.1.50",  result: "success",  method: "password+totp" },
  { at: "yesterday",     user: "anna",   ip: "192.168.1.62",  result: "success",  method: "password+totp" },
  { at: "yesterday",     user: "anna",   ip: "192.168.1.62",  result: "fail",     method: "password (bad-totp)" },
  { at: "2 d ago",       user: "—",      ip: "203.0.113.42",  result: "fail",     method: "password (no-such-user)" },
  { at: "2 d ago",       user: "pascal", ip: "192.168.1.50",  result: "success",  method: "password+totp" },
  { at: "3 d ago",       user: "viewer", ip: "192.168.1.51",  result: "success",  method: "password" },
];

const KRB5_PRINCIPALS = [
  { name: "host/nas.lan@LAN.NOVANAS.IO",     created: "2024-01-04", expires: "—",         keyver: 3, type: "host" },
  { name: "nfs/nas.lan@LAN.NOVANAS.IO",      created: "2024-01-04", expires: "—",         keyver: 2, type: "service" },
  { name: "cifs/nas.lan@LAN.NOVANAS.IO",     created: "2024-01-04", expires: "—",         keyver: 2, type: "service" },
  { name: "pascal@LAN.NOVANAS.IO",            created: "2024-01-04", expires: "2026-01-04",keyver: 4, type: "user" },
  { name: "anna@LAN.NOVANAS.IO",              created: "2024-02-14", expires: "2026-02-14",keyver: 1, type: "user" },
];

// ─────── Observability ───────
const ALERTS = [
  { fp: "a4f1c2", name: "DiskSMARTReallocated",    severity: "warning",  state: "firing", since: "13:22 today", labels: { instance: "disk-13", pool: "bulk" }, summary: "Disk slot 13 has 5 reallocated sectors" },
  { fp: "b7d281", name: "ReplicationFailed",       severity: "critical", state: "firing", since: "02:00 today", labels: { job: "family→offsite" }, summary: "Replication job family→offsite failed: ssh timeout" },
  { fp: "c11e44", name: "PoolCapacityHigh",        severity: "warning",  state: "firing", since: "1 d ago",      labels: { pool: "bulk" }, summary: "Pool bulk is 85% full" },
  { fp: "d901a2", name: "VMUnresponsive",          severity: "warning",  state: "resolved", since: "yesterday",  labels: { vm: "windows-11" }, summary: "VM windows-11 was unresponsive for 4m" },
  { fp: "e21c00", name: "BackupOverdue",           severity: "info",     state: "silenced", since: "3 d ago",     labels: { dataset: "backups" }, summary: "Backup dataset hasn't been written in 3d" },
];

const ALERT_SILENCES = [
  { id: "sil-7d12", matchers: [{n:"alertname",v:"BackupOverdue"}], creator: "pascal", comment: "vacation — disable until 5/3", starts: "today",      ends: "in 3 d" },
  { id: "sil-aa01", matchers: [{n:"instance",v:"vm-test-*"}],      creator: "anna",   comment: "test cluster maintenance",     starts: "yesterday",  ends: "in 1 h" },
];

const ALERT_RECEIVERS = [
  { name: "default",       integrations: ["smtp","webhook"] },
  { name: "ops-pagerduty", integrations: ["pagerduty"] },
  { name: "lab-slack",     integrations: ["slack"] },
];

const LOG_LABELS = ["job","instance","level","unit","pod","namespace","app"];

const LOG_LINES = [
  { t: "14:02:41.123", level: "info",  unit: "kubevirt-handler",  msg: "VM ubuntu-dev started, vcpu=8 ram=16384" },
  { t: "14:02:38.997", level: "info",  unit: "novanas-api",       msg: "POST /api/v1/vms/default/ubuntu-dev/start 202" },
  { t: "14:02:01.448", level: "info",  unit: "zfs-event-monitor", msg: "ereport.fs.zfs.checksum cleared on bulk/raidz2-0" },
  { t: "14:01:59.221", level: "warn",  unit: "smartd",            msg: "Device: /dev/sdm SMART Reallocated_Sector_Ct=5 (was 3)" },
  { t: "14:01:50.011", level: "info",  unit: "snapshot-runner",   msg: "Created snapshot family-media@auto-14:58 (1.4 GiB)" },
  { t: "14:01:40.730", level: "info",  unit: "novanas-api",       msg: "GET /api/v1/datasets 200 (62 ms)" },
  { t: "14:01:35.110", level: "error", unit: "replication-worker",msg: "ssh: connect to host backup.example.net port 22: connection timed out" },
  { t: "14:01:35.108", level: "error", unit: "replication-worker",msg: "job=family→offsite attempt=1/3 failed" },
  { t: "14:01:20.001", level: "info",  unit: "novanas-api",       msg: "user=pascal subject=storage:read action=GET /api/v1/pools" },
  { t: "14:01:10.488", level: "info",  unit: "loki",              msg: "ingester: chunk flushed (size=2.1MiB)" },
  { t: "14:00:58.222", level: "debug", unit: "scheduler",         msg: "tick: 4 schedules due" },
  { t: "14:00:05.001", level: "info",  unit: "kubelet",           msg: "Successfully pulled image immich-server:1.140.0" },
];

const AUDIT = [
  { at: "14:02 today", actor: "pascal", action: "vm.start",         resource: "ubuntu-dev",      result: "ok",  ip: "192.168.1.50" },
  { at: "13:58 today", actor: "system", action: "snapshot.create",  resource: "family-media",    result: "ok",  ip: "—" },
  { at: "13:40 today", actor: "system", action: "scrub.complete",   resource: "bulk",            result: "ok",  ip: "—" },
  { at: "13:22 today", actor: "system", action: "alert.fire",       resource: "disk-13",         result: "warn",ip: "—" },
  { at: "12:40 today", actor: "system", action: "replication.run",  resource: "photos→offsite",  result: "ok",  ip: "—" },
  { at: "12:04 today", actor: "pascal", action: "workload.upgrade", resource: "immich",          result: "ok",  ip: "192.168.1.50" },
  { at: "11:20 today", actor: "system", action: "disk.detect",      resource: "slot-23",         result: "info",ip: "—" },
  { at: "11:02 today", actor: "anna",   action: "user.password.set",resource: "anna",            result: "ok",  ip: "192.168.1.62" },
  { at: "09:00 today", actor: "system", action: "smart.test",       resource: "all-disks",       result: "ok",  ip: "—" },
  { at: "yesterday",   actor: "anna",   action: "alert.silence.create", resource: "vm-test-*",   result: "ok",  ip: "192.168.1.62" },
  { at: "yesterday",   actor: "pascal", action: "plugin.install",   resource: "novanas-photo-ai",result: "ok",  ip: "192.168.1.50" },
  { at: "yesterday",   actor: "pascal", action: "marketplace.add",  resource: "internal-mirror", result: "ok",  ip: "192.168.1.50" },
];

const JOBS = [
  { id: "job-9a12", kind: "scrub.run",          target: "fast",          state: "running",   pct: 0.42, eta: "1h 12m", started: "13:00 today", log: "scrubbing vdev mirror-1 of 3" },
  { id: "job-7c80", kind: "replication.run",    target: "vm-disks→nas2", state: "running",   pct: 0.84, eta: "8m",     started: "12:00 today", log: "sending incremental @auto-13:45 → @auto-14:00" },
  { id: "job-3d01", kind: "smart.test",         target: "/dev/sdm",      state: "running",   pct: 0.18, eta: "44m",    started: "13:25 today", log: "long offline test in progress" },
  { id: "job-1f44", kind: "workload.upgrade",   target: "immich",        state: "succeeded", pct: 1.00, eta: "—",      started: "12:00 today", log: "helm upgrade immich/1.140.0 ok" },
  { id: "job-aa20", kind: "snapshot.create",    target: "family-media",  state: "succeeded", pct: 1.00, eta: "—",      started: "13:58 today", log: "auto-14:58 created" },
  { id: "job-bb01", kind: "replication.run",    target: "family→offsite",state: "failed",    pct: 0.04, eta: "—",      started: "02:00 today", log: "ssh: connection timeout" },
  { id: "job-cc34", kind: "pool.scrub",         target: "archive",       state: "queued",    pct: 0.00, eta: "—",      started: "—",            log: "queued behind job-9a12" },
];

const NOTIFICATIONS = [
  { id: "n1", at: "14:02", read: false, sev: "info",  title: "VM ubuntu-dev started",                src: "vm",          actor: "pascal" },
  { id: "n2", at: "13:58", read: false, sev: "info",  title: "Snapshot family-media@auto-14:58",     src: "snapshot",    actor: "system" },
  { id: "n3", at: "13:40", read: false, sev: "info",  title: "Scrub of bulk completed · 0 errors",   src: "scrub",       actor: "system" },
  { id: "n4", at: "13:22", read: false, sev: "warn",  title: "SMART warning on disk-13",             src: "alert",       actor: "system" },
  { id: "n5", at: "12:40", read: true,  sev: "info",  title: "Replication photos→offsite finished", src: "replication", actor: "system" },
  { id: "n6", at: "12:04", read: true,  sev: "ok",    title: "App immich updated to 1.140.0",       src: "workload",    actor: "pascal" },
  { id: "n7", at: "02:00", read: false, sev: "err",   title: "Replication family→offsite FAILED",   src: "alert",       actor: "system" },
  { id: "n8", at: "yesterday", read: true, sev: "ok", title: "Daily SMART test complete",            src: "smart",       actor: "system" },
];

// ─────── Shares ───────
const NFS_EXPORTS = [
  { name: "media",  path: "/bulk/family-media", clients: "192.168.1.0/24",  options: "rw,sync,sec=krb5p,no_subtree_check", active: true },
  { name: "isos",   path: "/bulk/isos",          clients: "192.168.1.0/24",  options: "ro,sync,sec=sys",                    active: true },
  { name: "backup", path: "/archive/backups",    clients: "10.0.10.0/24",     options: "rw,async,sec=krb5p,no_subtree_check",active: true },
];

const SMB_SHARES = [
  { name: "family",  path: "/bulk/family-media", users: "@family",          guest: false, recycle: true,  vfs: "shadow_copy2,acl_xattr" },
  { name: "pascal",  path: "/bulk/pascal",       users: "pascal",            guest: false, recycle: true,  vfs: "shadow_copy2,acl_xattr" },
  { name: "shared",  path: "/bulk/shared",       users: "@family,@guests",   guest: false, recycle: false, vfs: "acl_xattr" },
  { name: "scans",   path: "/fast/scans",        users: "scanner-svc",       guest: true,  recycle: false, vfs: "" },
];

const ISCSI_TARGETS = [
  { iqn: "iqn.2026-04.io.novanas:vm-disks", luns: 4, portals: ["192.168.1.10:3260","10.0.10.1:3260"], acls: 3, state: "active" },
  { iqn: "iqn.2026-04.io.novanas:db-vols",  luns: 2, portals: ["10.0.10.1:3260"],                       acls: 1, state: "active" },
];

const NVMEOF_SUBSYSTEMS = [
  { nqn: "nqn.2026-04.io.novanas:fast0",  ns: 4, ports: 2, hosts: 3, dhchap: true,  state: "active" },
  { nqn: "nqn.2026-04.io.novanas:lab",    ns: 1, ports: 1, hosts: 1, dhchap: false, state: "active" },
];

const PROTOCOL_SHARES = [
  { name: "family",      protocols: ["smb","nfs"],  path: "/bulk/family-media", clients: "lan",    state: "active" },
  { name: "pascal",      protocols: ["smb"],         path: "/bulk/pascal",       clients: "user:pascal", state: "active" },
  { name: "isos",        protocols: ["nfs"],         path: "/bulk/isos",         clients: "lan",     state: "active" },
  { name: "vm-disks",    protocols: ["iscsi","nvmeof"], path: "block:vm-disks",  clients: "ipsec-only", state: "active" },
  { name: "backup",      protocols: ["nfs","s3"],    path: "/archive/backups",   clients: "wan-vpn", state: "active" },
];

// ─────── System ───────
const SYSTEM_INFO = {
  hostname: "nas.lan",
  version: "NovaNAS 2.4.0",
  channel: "stable",
  build: "2026.04.18-r1",
  kernel: "6.8.12-novanas",
  uptime: "12 d 4 h 18 m",
  cpu: "AMD EPYC 4564P · 16 cores / 32 threads",
  ram: "128 GiB ECC · DDR5-4800",
  motherboard: "Supermicro H13SAE-MF",
  bmc: "Aspeed AST2600",
  serial: "SUP-A8842-29944",
};

const SYSTEM_UPDATE = {
  available: true,
  channel: "stable",
  current: "2.4.0",
  latest: "2.5.0",
  notes: "ZFS 2.3 backport · Loki 3.2 · KubeVirt 1.4 · plugin-engine v3 (multi-marketplace)",
  size: "184 MiB",
  releaseDate: "2026-04-22",
};

// ─────── SMTP ───────
const SMTP_CONFIG = {
  enabled: true,
  host: "smtp.fastmail.com",
  port: 587,
  username: "alerts@watteel.com",
  from: "novanas@watteel.com",
  tls: "starttls",
  lastTest: "2 d ago · ok",
};

// ─────── Activity feed ───────
const ACTIVITY = [
  { t: "14:02", tone: "ok",   text: "pascal started ubuntu-dev" },
  { t: "13:58", tone: "info", text: "Snapshot family-media@auto-14:58 created" },
  { t: "13:40", tone: "info", text: "Scrub of pool bulk completed · 0 errors" },
  { t: "13:22", tone: "warn", text: "SMART warning on disk-13 · 5 reallocated sectors" },
  { t: "12:40", tone: "info", text: "Replication photos → offsite finished" },
  { t: "12:04", tone: "ok",   text: "App immich updated to 1.140.0" },
  { t: "11:20", tone: "info", text: "New disk detected in slot 23" },
  { t: "09:00", tone: "ok",   text: "Daily SMART test complete on 22 disks" },
];

const FILES = [
  { name: "Family",       kind: "folder", size: null,   mod: "2d ago" },
  { name: "Photos",       kind: "folder", size: null,   mod: "5h ago" },
  { name: "Movies",       kind: "folder", size: null,   mod: "1w ago" },
  { name: "Documents",    kind: "folder", size: null,   mod: "3h ago" },
  { name: "Downloads",    kind: "folder", size: null,   mod: "12m ago" },
  { name: "Backups",      kind: "folder", size: null,   mod: "1d ago" },
  { name: "tax-2025.pdf", kind: "pdf",    size: 2.4e6,  mod: "Apr 12" },
  { name: "vacation.mp4", kind: "video",  size: 1.8e9,  mod: "Apr 22" },
  { name: "router.cfg",   kind: "text",   size: 14e3,   mod: "Mar 3" },
  { name: "screenshot.png", kind: "image", size: 412e3, mod: "Today" },
];

function fmtBytes(b, d = 1) {
  if (b == null) return "—";
  const u = ["B","KB","MB","GB","TB","PB"];
  let v = b, i = 0;
  while (v >= 1024 && i < u.length - 1) { v /= 1024; i++; }
  return `${v.toFixed(v >= 100 ? 0 : d)} ${u[i]}`;
}
function fmtPct(v) { return `${Math.round(v * 100)}%`; }

Object.assign(window, {
  POOLS, VDEV_TREE, DISKS, DATASETS, SNAPSHOTS, SNAPSHOT_SCHEDULES, SCRUB_POLICIES,
  REPL_TARGETS, REPL_JOBS, ENCRYPTED_DATASETS,
  NETWORK_INTERFACES, RDMA_DEVICES,
  APPS, WORKLOADS, VMS, VM_TEMPLATES, VM_SNAPSHOTS, PLUGINS, MARKETPLACE_PLUGINS, MARKETPLACES,
  USERS, SESSIONS, LOGIN_HISTORY, KRB5_PRINCIPALS,
  ALERTS, ALERT_SILENCES, ALERT_RECEIVERS, LOG_LABELS, LOG_LINES, AUDIT, JOBS, NOTIFICATIONS,
  NFS_EXPORTS, SMB_SHARES, ISCSI_TARGETS, NVMEOF_SUBSYSTEMS, PROTOCOL_SHARES,
  SYSTEM_INFO, SYSTEM_UPDATE, SMTP_CONFIG,
  ACTIVITY, FILES,
  fmtBytes, fmtPct,
});
