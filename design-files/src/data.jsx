/* globals React */

// ───────── mock data seeds ─────────
const SEEDS = {
  healthy: {
    status: "ok",
    statusLabel: "All systems healthy",
    disksDegraded: 0,
    disksFailed: 0,
    activeAlerts: 0,
    rebuild: null,
  },
  degraded: {
    status: "warn",
    statusLabel: "1 disk degraded · protection intact",
    disksDegraded: 1,
    disksFailed: 0,
    activeAlerts: 2,
    rebuild: null,
  },
  rebuilding: {
    status: "warn",
    statusLabel: "Rebuilding onto spare · ETA 2h 14m",
    disksDegraded: 0,
    disksFailed: 1,
    activeAlerts: 3,
    rebuild: { pool: "fast", progress: 0.43, eta: "2h 14m" },
  },
};

const POOLS = [
  {
    name: "fast", tier: "hot", disks: 8, used: 4.9e12, total: 7.6e12,
    protection: "rep×2", devices: "NVMe", state: "Healthy",
    throughput: { r: 1420, w: 680 }, iops: { r: 24100, w: 8900 },
  },
  {
    name: "bulk", tier: "warm", disks: 10, used: 23.1e12, total: 60e12,
    protection: "EC 6+2", devices: "HDD", state: "Healthy",
    throughput: { r: 340, w: 210 }, iops: { r: 1400, w: 900 },
  },
  {
    name: "archive", tier: "cold", disks: 6, used: 14e12, total: 30e12,
    protection: "EC 4+2", devices: "HDD", state: "Healthy",
    throughput: { r: 120, w: 80 }, iops: { r: 420, w: 260 },
  },
  {
    name: "meta", tier: "hot", disks: 3, used: 12e9, total: 500e9,
    protection: "rep×3", devices: "NVMe", state: "Healthy",
    throughput: { r: 60, w: 30 }, iops: { r: 8200, w: 2100 },
  },
];

// 24-slot enclosure
const DISKS = [
  { slot: 1, wwn: "5000c500a1b2c3d4", model: "SAMSUNG PM9A3", cap: 1920e9, class: "NVMe", pool: "fast",    state: "ACTIVE",  role: "data",  temp: 41, hours: 9840 },
  { slot: 2, wwn: "5000c500a1b2c3d5", model: "SAMSUNG PM9A3", cap: 1920e9, class: "NVMe", pool: "fast",    state: "ACTIVE",  role: "data",  temp: 42, hours: 9840 },
  { slot: 3, wwn: "5000c500a1b2c3d6", model: "SAMSUNG PM9A3", cap: 1920e9, class: "NVMe", pool: "fast",    state: "ACTIVE",  role: "data",  temp: 40, hours: 9840 },
  { slot: 4, wwn: "5000c500a1b2c3d7", model: "SAMSUNG PM9A3", cap: 1920e9, class: "NVMe", pool: "fast",    state: "ACTIVE",  role: "data",  temp: 43, hours: 9840 },
  { slot: 5, wwn: "5000c500a1b2c3d8", model: "KIOXIA CM7-R",  cap: 1920e9, class: "NVMe", pool: "fast",    state: "ACTIVE",  role: "data",  temp: 44, hours: 5220 },
  { slot: 6, wwn: "5000c500a1b2c3d9", model: "KIOXIA CM7-R",  cap: 1920e9, class: "NVMe", pool: "fast",    state: "ACTIVE",  role: "data",  temp: 45, hours: 5220 },
  { slot: 7, wwn: "5000c500a1b2c3da", model: "KIOXIA CM7-R",  cap: 1920e9, class: "NVMe", pool: "fast",    state: "ACTIVE",  role: "data",  temp: 43, hours: 5220 },
  { slot: 8, wwn: "5000c500a1b2c3db", model: "KIOXIA CM7-R",  cap: 1920e9, class: "NVMe", pool: "fast",    state: "SPARE",   role: "spare", temp: 38, hours: 120 },

  { slot: 9,  wwn: "5000cca2a1b2c310", model: "SEAGATE EXOS X22", cap: 22e12, class: "HDD", pool: "bulk", state: "ACTIVE",   role: "data", temp: 36, hours: 18220 },
  { slot: 10, wwn: "5000cca2a1b2c311", model: "SEAGATE EXOS X22", cap: 22e12, class: "HDD", pool: "bulk", state: "ACTIVE",   role: "data", temp: 37, hours: 18220 },
  { slot: 11, wwn: "5000cca2a1b2c312", model: "SEAGATE EXOS X22", cap: 22e12, class: "HDD", pool: "bulk", state: "ACTIVE",   role: "data", temp: 35, hours: 18220 },
  { slot: 12, wwn: "5000cca2a1b2c313", model: "SEAGATE EXOS X22", cap: 22e12, class: "HDD", pool: "bulk", state: "ACTIVE",   role: "data", temp: 38, hours: 18220 },
  { slot: 13, wwn: "5000cca2a1b2c314", model: "WDC ULTRASTAR 20", cap: 20e12, class: "HDD", pool: "bulk", state: "DEGRADED", role: "data", temp: 48, hours: 34120, reason: "5 reallocated sectors" },
  { slot: 14, wwn: "5000cca2a1b2c315", model: "WDC ULTRASTAR 20", cap: 20e12, class: "HDD", pool: "bulk", state: "ACTIVE",   role: "data", temp: 39, hours: 18220 },
  { slot: 15, wwn: "5000cca2a1b2c316", model: "WDC ULTRASTAR 20", cap: 20e12, class: "HDD", pool: "bulk", state: "ACTIVE",   role: "data", temp: 40, hours: 18220 },
  { slot: 16, wwn: "5000cca2a1b2c317", model: "WDC ULTRASTAR 20", cap: 20e12, class: "HDD", pool: "bulk", state: "ACTIVE",   role: "data", temp: 36, hours: 18220 },
  { slot: 17, wwn: "5000cca2a1b2c318", model: "TOSHIBA MG10",     cap: 20e12, class: "HDD", pool: "bulk", state: "ACTIVE",   role: "data", temp: 37, hours: 6210 },
  { slot: 18, wwn: "5000cca2a1b2c319", model: "TOSHIBA MG10",     cap: 20e12, class: "HDD", pool: "bulk", state: "ACTIVE",   role: "data", temp: 37, hours: 6210 },

  { slot: 19, wwn: "5000cca2a1b2c320", model: "SEAGATE EXOS X20", cap: 20e12, class: "HDD", pool: "archive", state: "ACTIVE", role: "data", temp: 34, hours: 8110 },
  { slot: 20, wwn: "5000cca2a1b2c321", model: "SEAGATE EXOS X20", cap: 20e12, class: "HDD", pool: "archive", state: "ACTIVE", role: "data", temp: 35, hours: 8110 },
  { slot: 21, wwn: "5000cca2a1b2c322", model: "SEAGATE EXOS X20", cap: 20e12, class: "HDD", pool: "archive", state: "ACTIVE", role: "data", temp: 34, hours: 8110 },
  { slot: 22, wwn: "5000cca2a1b2c323", model: "SEAGATE EXOS X20", cap: 20e12, class: "HDD", pool: "archive", state: "ACTIVE", role: "data", temp: 34, hours: 8110 },

  { slot: 23, wwn: null, model: null, cap: null, class: null, pool: null, state: "EMPTY", role: null },
  { slot: 24, wwn: null, model: null, cap: null, class: null, pool: null, state: "EMPTY", role: null },
];

const DATASETS = [
  { name: "family-media",  pool: "bulk", size: 5.2e12, used: 4.1e12, quota: 6e12, proto: "SMB + NFS", owner: "family", prot: "EC 6+2", snap: 142, enc: true },
  { name: "pascal/docs",   pool: "fast", size: 180e9,  used: 92e9,   quota: 200e9, proto: "SMB",       owner: "pascal", prot: "rep×2",  snap: 96,  enc: true },
  { name: "pascal/photos", pool: "bulk", size: 820e9,  used: 640e9,  quota: 1e12,  proto: "SMB",       owner: "pascal", prot: "EC 6+2", snap: 72,  enc: true },
  { name: "vm-disks",      pool: "fast", size: 900e9,  used: 412e9,  quota: 1e12,  proto: "block",     owner: "system", prot: "rep×2",  snap: 54,  enc: true },
  { name: "app-configs",   pool: "fast", size: 40e9,   used: 8.2e9,  quota: 80e9,  proto: "block",     owner: "system", prot: "rep×3",  snap: 320, enc: true },
  { name: "isos",          pool: "bulk", size: 120e9,  used: 84e9,   quota: 200e9, proto: "NFS",       owner: "system", prot: "EC 6+2", snap: 4,   enc: false },
  { name: "backups",       pool: "archive", size: 8.2e12, used: 6.4e12, quota: 10e12, proto: "bucket", owner: "system", prot: "EC 4+2", snap: 28, enc: true },
];

const APPS_CATALOG = [
  { slug: "plex",        name: "Plex",          cat: "Media",    color: "oklch(0.55 0.14 70)",  installed: true,  desc: "Streaming media server for your personal library.", tags: ["official", "media"] },
  { slug: "jellyfin",    name: "Jellyfin",      cat: "Media",    color: "oklch(0.55 0.14 290)", installed: false, desc: "Free software media system to manage and stream.",  tags: ["official", "media"] },
  { slug: "immich",      name: "Immich",        cat: "Photos",   color: "oklch(0.55 0.14 240)", installed: true,  desc: "High-performance, self-hosted photo & video backup.", tags: ["official", "photos"] },
  { slug: "nextcloud",   name: "Nextcloud",     cat: "Files",    color: "oklch(0.55 0.14 220)", installed: true,  desc: "Files, calendar, contacts and collaboration suite.", tags: ["official", "files"] },
  { slug: "homeassistant", name: "Home Assistant", cat: "Home", color: "oklch(0.55 0.14 200)", installed: true,  desc: "Open-source home automation that puts local first.", tags: ["official", "home"] },
  { slug: "gitea",       name: "Gitea",         cat: "Dev",      color: "oklch(0.55 0.14 160)", installed: false, desc: "Self-hosted Git service with a familiar workflow.",  tags: ["official", "dev"] },
  { slug: "vaultwarden", name: "Vaultwarden",   cat: "Utility",  color: "oklch(0.55 0.14 20)",  installed: true,  desc: "Self-hosted Bitwarden-compatible password manager.", tags: ["official", "security"] },
  { slug: "paperless",   name: "Paperless-ngx", cat: "Utility",  color: "oklch(0.55 0.14 45)",  installed: false, desc: "Scan, index and archive all your physical documents.", tags: ["official", "docs"] },
  { slug: "adguard",     name: "AdGuard Home",  cat: "Network",  color: "oklch(0.55 0.14 140)", installed: false, desc: "Network-wide ads and trackers blocking DNS server.", tags: ["official", "network"] },
  { slug: "frigate",     name: "Frigate",       cat: "Home",     color: "oklch(0.55 0.14 10)",  installed: false, desc: "Realtime AI object detection for local NVR.",        tags: ["official", "home"] },
  { slug: "sonarr",      name: "Sonarr",        cat: "Media",    color: "oklch(0.55 0.14 205)", installed: true,  desc: "Smart PVR for newsgroup and BitTorrent users.",       tags: ["official", "arr"] },
  { slug: "radarr",      name: "Radarr",        cat: "Media",    color: "oklch(0.55 0.14 50)",  installed: true,  desc: "Movie collection manager for Usenet and torrents.",   tags: ["official", "arr"] },
  { slug: "code-server", name: "code-server",   cat: "Dev",      color: "oklch(0.55 0.14 265)", installed: false, desc: "Run VS Code in the browser on NovaNas.",             tags: ["official", "dev"] },
  { slug: "grafana",     name: "Grafana",       cat: "Observability", color: "oklch(0.55 0.14 30)", installed: false, desc: "Dashboards for your user-owned Prom data.",   tags: ["official", "observe"] },
  { slug: "postgres",    name: "PostgreSQL",    cat: "Databases", color: "oklch(0.55 0.14 230)", installed: false, desc: "The world's most advanced open-source DB.",         tags: ["official", "db"] },
  { slug: "wiki-js",     name: "Wiki.js",       cat: "Utility",   color: "oklch(0.55 0.14 180)", installed: false, desc: "Modern, lightweight and powerful wiki engine.",    tags: ["official", "docs"] },
];

const VMS = [
  { name: "windows-11",  owner: "pascal",  os: "Win 11",         cpu: 4, ramMiB: 8192, state: "Running", disks: 2, gpu: "RTX 4080", ip: "192.168.1.101" },
  { name: "ubuntu-dev",  owner: "pascal",  os: "Ubuntu 24.04",   cpu: 8, ramMiB: 16384, state: "Running", disks: 1, gpu: null,      ip: "192.168.1.102" },
  { name: "pfsense",     owner: "admin",   os: "FreeBSD 14",     cpu: 2, ramMiB: 2048, state: "Running", disks: 1, gpu: null,      ip: "192.168.1.1"   },
  { name: "homeassistant-vm", owner: "admin", os: "HA OS 12",    cpu: 2, ramMiB: 4096, state: "Running", disks: 1, gpu: null,      ip: "192.168.1.104" },
  { name: "win-gaming",  owner: "eli",     os: "Win 11",         cpu: 8, ramMiB: 32768, state: "Stopped", disks: 2, gpu: null,     ip: null },
  { name: "proxy-edge",  owner: "admin",   os: "Alpine 3.20",    cpu: 1, ramMiB: 1024, state: "Running", disks: 1, gpu: null,      ip: "192.168.1.105" },
];

const ACTIVITY = [
  { t: "14:02",  tone: "ok",   text: <><strong>pascal</strong> started <strong>ubuntu-dev</strong></> },
  { t: "13:58",  tone: "info", text: <>Snapshot <strong>family-media@auto-14:58</strong> created (1.4 GB)</> },
  { t: "13:40",  tone: "info", text: <>Scrub of pool <strong>bulk</strong> completed · 0 errors</> },
  { t: "13:22",  tone: "warn", text: <>SMART warning on <strong>disk-13</strong> · 5 reallocated sectors</> },
  { t: "12:40",  tone: "info", text: <>Replication <strong>photos → offsite</strong> finished · 612 MB sent</> },
  { t: "12:04",  tone: "ok",   text: <>App <strong>immich</strong> updated to <span className="mono">1.140.0</span></> },
  { t: "11:20",  tone: "info", text: <>New disk detected in slot <strong>23</strong> · awaiting assignment</> },
  { t: "09:00",  tone: "ok",   text: <>Daily short SMART test complete on 22 disks</> },
];

window.SEEDS = SEEDS;
window.POOLS = POOLS;
window.DISKS = DISKS;
window.DATASETS = DATASETS;
window.APPS_CATALOG = APPS_CATALOG;
window.VMS = VMS;
window.ACTIVITY = ACTIVITY;
