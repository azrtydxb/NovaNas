// Package system manages host-level system configuration and queries:
// hostname, NTP/timezone via timedatectl, generic host info gleaned from
// /proc and /etc, and reboot/shutdown via systemctl/shutdown.
//
// All shell-out goes through exec.Runner so tests can stub it. Files
// under /proc and /etc are read directly; the roots are configurable on
// Manager so tests can use fixtures.
package system

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/novanas/nova-nas/internal/host/exec"
)

// Duration wraps time.Duration so JSON serializes as a string like "1h2m3s".
type Duration time.Duration

// MarshalJSON implements json.Marshaler.
func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

// UnmarshalJSON implements json.Unmarshaler.
func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	*d = Duration(v)
	return nil
}

// Info is the host's general system information.
type Info struct {
	Hostname      string      `json:"hostname"`
	OSPretty      string      `json:"osPretty"`
	KernelRelease string      `json:"kernelRelease"`
	Architecture  string      `json:"architecture"`
	Uptime        Duration    `json:"uptime"`
	LoadAvg       [3]float64  `json:"loadAvg"`
	Memory        MemoryStats `json:"memory"`
	CPU           CPUInfo     `json:"cpu"`
	ZFSVersion    string      `json:"zfsVersion,omitempty"`
}

// MemoryStats is a subset of /proc/meminfo.
type MemoryStats struct {
	TotalKB     uint64 `json:"totalKB"`
	AvailKB     uint64 `json:"availableKB"`
	SwapTotalKB uint64 `json:"swapTotalKB"`
	SwapFreeKB  uint64 `json:"swapFreeKB"`
}

// CPUInfo summarizes /proc/cpuinfo.
type CPUInfo struct {
	Model   string `json:"model"`
	Sockets int    `json:"sockets"`
	Cores   int    `json:"cores"`
	Threads int    `json:"threads"`
}

// TimeConfig describes timesync state.
type TimeConfig struct {
	Timezone     string   `json:"timezone"`
	NTP          bool     `json:"ntp"`
	NTPServers   []string `json:"ntpServers,omitempty"`
	Synchronized bool     `json:"synchronized"`
}

// Manager wires command bins, /proc, and /etc roots.
type Manager struct {
	Runner         exec.Runner
	HostnamectlBin string
	TimedatectlBin string
	SystemctlBin   string
	UnameBin       string
	ZpoolBin       string
	ShutdownBin    string
	ProcRoot       string
	EtcRoot        string
	// ZoneinfoRoot is the directory used to validate timezones; defaults
	// to /usr/share/zoneinfo. Overridable for tests.
	ZoneinfoRoot string
}

func (m *Manager) hostnamectl() string {
	if m.HostnamectlBin == "" {
		return "/usr/bin/hostnamectl"
	}
	return m.HostnamectlBin
}

func (m *Manager) timedatectl() string {
	if m.TimedatectlBin == "" {
		return "/usr/bin/timedatectl"
	}
	return m.TimedatectlBin
}

func (m *Manager) systemctl() string {
	if m.SystemctlBin == "" {
		return "/usr/bin/systemctl"
	}
	return m.SystemctlBin
}

func (m *Manager) uname() string {
	if m.UnameBin == "" {
		return "/usr/bin/uname"
	}
	return m.UnameBin
}

func (m *Manager) zpool() string {
	if m.ZpoolBin == "" {
		return "/sbin/zpool"
	}
	return m.ZpoolBin
}

func (m *Manager) shutdownBin() string {
	if m.ShutdownBin == "" {
		return "/usr/sbin/shutdown"
	}
	return m.ShutdownBin
}

func (m *Manager) procRoot() string {
	if m.ProcRoot == "" {
		return "/proc"
	}
	return m.ProcRoot
}

func (m *Manager) etcRoot() string {
	if m.EtcRoot == "" {
		return "/etc"
	}
	return m.EtcRoot
}

func (m *Manager) zoneinfoRoot() string {
	if m.ZoneinfoRoot == "" {
		return "/usr/share/zoneinfo"
	}
	return m.ZoneinfoRoot
}

func (m *Manager) run(ctx context.Context, bin string, args ...string) ([]byte, error) {
	r := m.Runner
	if r == nil {
		r = exec.Run
	}
	return r(ctx, bin, args...)
}

// ---------- GetInfo ----------

// GetInfo aggregates host information from /proc, /etc, uname, and zpool.
// Best-effort: errors reading individual sources are tolerated; only a
// hard failure to read /proc/meminfo (which is required for memory) is
// returned.
func (m *Manager) GetInfo(ctx context.Context) (*Info, error) {
	info := &Info{}

	// Hostname: prefer /etc/hostname, fall back to os.Hostname.
	if b, err := os.ReadFile(filepath.Join(m.etcRoot(), "hostname")); err == nil {
		info.Hostname = strings.TrimSpace(string(b))
	}
	if info.Hostname == "" {
		if h, err := os.Hostname(); err == nil {
			info.Hostname = h
		}
	}

	// /etc/os-release PRETTY_NAME
	if b, err := os.ReadFile(filepath.Join(m.etcRoot(), "os-release")); err == nil {
		info.OSPretty = parseOSReleasePretty(b)
	}

	// uname -r and uname -m
	if out, err := m.run(ctx, m.uname(), "-r"); err == nil {
		info.KernelRelease = strings.TrimSpace(string(out))
	}
	if out, err := m.run(ctx, m.uname(), "-m"); err == nil {
		info.Architecture = strings.TrimSpace(string(out))
	}

	// /proc/uptime
	if b, err := os.ReadFile(filepath.Join(m.procRoot(), "uptime")); err == nil {
		if d, ok := parseUptime(b); ok {
			info.Uptime = Duration(d)
		}
	}

	// /proc/loadavg
	if b, err := os.ReadFile(filepath.Join(m.procRoot(), "loadavg")); err == nil {
		if l, ok := parseLoadAvg(b); ok {
			info.LoadAvg = l
		}
	}

	// /proc/meminfo (required)
	mb, err := os.ReadFile(filepath.Join(m.procRoot(), "meminfo"))
	if err != nil {
		return nil, fmt.Errorf("read meminfo: %w", err)
	}
	info.Memory = parseMeminfo(mb)

	// /proc/cpuinfo
	if b, err := os.ReadFile(filepath.Join(m.procRoot(), "cpuinfo")); err == nil {
		info.CPU = parseCPUinfo(b)
	}

	// zfs version (best-effort)
	if v, err := m.run(ctx, m.zpool(), "version"); err == nil {
		info.ZFSVersion = parseZpoolVersion(v)
	}

	return info, nil
}

func parseOSReleasePretty(b []byte) string {
	scanner := bufio.NewScanner(bytes.NewReader(b))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "PRETTY_NAME=") {
			continue
		}
		v := strings.TrimPrefix(line, "PRETTY_NAME=")
		v = strings.TrimSpace(v)
		// strip surrounding quotes
		if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
			v = v[1 : len(v)-1]
		}
		return v
	}
	return ""
}

func parseUptime(b []byte) (time.Duration, bool) {
	fields := strings.Fields(string(b))
	if len(fields) < 1 {
		return 0, false
	}
	secs, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, false
	}
	return time.Duration(secs * float64(time.Second)), true
}

func parseLoadAvg(b []byte) ([3]float64, bool) {
	fields := strings.Fields(string(b))
	if len(fields) < 3 {
		return [3]float64{}, false
	}
	var out [3]float64
	for i := 0; i < 3; i++ {
		v, err := strconv.ParseFloat(fields[i], 64)
		if err != nil {
			return [3]float64{}, false
		}
		out[i] = v
	}
	return out, true
}

func parseMeminfo(b []byte) MemoryStats {
	var out MemoryStats
	scanner := bufio.NewScanner(bytes.NewReader(b))
	for scanner.Scan() {
		line := scanner.Text()
		colon := strings.IndexByte(line, ':')
		if colon < 0 {
			continue
		}
		key := line[:colon]
		rest := strings.TrimSpace(line[colon+1:])
		// values are typically "1234 kB"
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			continue
		}
		v, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			continue
		}
		switch key {
		case "MemTotal":
			out.TotalKB = v
		case "MemAvailable":
			out.AvailKB = v
		case "SwapTotal":
			out.SwapTotalKB = v
		case "SwapFree":
			out.SwapFreeKB = v
		}
	}
	return out
}

func parseCPUinfo(b []byte) CPUInfo {
	var out CPUInfo
	physIDs := map[string]struct{}{}
	coreIDsBySocket := map[string]map[string]struct{}{}
	threads := 0
	currentPhys := ""

	scanner := bufio.NewScanner(bytes.NewReader(b))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			currentPhys = ""
			continue
		}
		colon := strings.IndexByte(line, ':')
		if colon < 0 {
			continue
		}
		key := strings.TrimSpace(line[:colon])
		val := strings.TrimSpace(line[colon+1:])
		switch key {
		case "processor":
			threads++
		case "model name":
			if out.Model == "" {
				out.Model = val
			}
		case "physical id":
			physIDs[val] = struct{}{}
			currentPhys = val
			if _, ok := coreIDsBySocket[val]; !ok {
				coreIDsBySocket[val] = map[string]struct{}{}
			}
		case "core id":
			if currentPhys == "" {
				// arch w/o physical id (e.g. arm) — fold into "0"
				if _, ok := coreIDsBySocket[""]; !ok {
					coreIDsBySocket[""] = map[string]struct{}{}
				}
				coreIDsBySocket[""][val] = struct{}{}
			} else {
				coreIDsBySocket[currentPhys][val] = struct{}{}
			}
		}
	}
	out.Threads = threads
	out.Sockets = len(physIDs)
	if out.Sockets == 0 && threads > 0 {
		out.Sockets = 1
	}
	cores := 0
	for _, m := range coreIDsBySocket {
		cores += len(m)
	}
	if cores == 0 {
		cores = threads
	}
	out.Cores = cores
	return out
}

// parseZpoolVersion takes the output of `zpool version` and returns the
// first line, which is conventionally "zfs-2.2.4-1".
func parseZpoolVersion(b []byte) string {
	scanner := bufio.NewScanner(bytes.NewReader(b))
	if scanner.Scan() {
		return strings.TrimSpace(scanner.Text())
	}
	return ""
}

// ---------- GetTimeConfig ----------

// GetTimeConfig parses `timedatectl show` and reads the upstream NTP
// servers from /etc/systemd/timesyncd.conf if present.
func (m *Manager) GetTimeConfig(ctx context.Context) (*TimeConfig, error) {
	out, err := m.run(ctx, m.timedatectl(), "show", "--no-pager")
	if err != nil {
		return nil, err
	}
	tc := &TimeConfig{}
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		key := line[:eq]
		val := line[eq+1:]
		switch key {
		case "Timezone":
			tc.Timezone = val
		case "NTP":
			tc.NTP = (val == "yes" || val == "true")
		case "NTPSynchronized":
			tc.Synchronized = (val == "yes" || val == "true")
		}
	}

	// Best-effort: read /etc/systemd/timesyncd.conf for upstream servers.
	confPath := filepath.Join(m.etcRoot(), "systemd", "timesyncd.conf")
	if b, err := os.ReadFile(confPath); err == nil {
		tc.NTPServers = parseTimesyncdNTP(b)
	}
	return tc, nil
}

// parseTimesyncdNTP returns the values of the NTP= key under the [Time]
// section. Multiple NTP= lines are concatenated.
func parseTimesyncdNTP(b []byte) []string {
	var section string
	var servers []string
	scanner := bufio.NewScanner(bytes.NewReader(b))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(line[1 : len(line)-1])
			continue
		}
		if section != "time" {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		if key == "NTP" && val != "" {
			servers = append(servers, strings.Fields(val)...)
		}
	}
	if len(servers) == 0 {
		return nil
	}
	return servers
}

// ---------- SetHostname ----------

// validateHostname enforces RFC 1123: 1-63 chars, alphanumeric + hyphen,
// no leading/trailing hyphen, no NUL.
func validateHostname(name string) error {
	if name == "" {
		return errors.New("hostname required")
	}
	if len(name) > 63 {
		return fmt.Errorf("hostname too long (%d > 63)", len(name))
	}
	if strings.Contains(name, "\x00") {
		return errors.New("hostname contains NUL")
	}
	if name[0] == '-' || name[len(name)-1] == '-' {
		return fmt.Errorf("hostname must not start or end with hyphen: %q", name)
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-':
		default:
			return fmt.Errorf("hostname contains invalid character %q: %q", r, name)
		}
	}
	return nil
}

// SetHostname applies the new hostname via hostnamectl.
func (m *Manager) SetHostname(ctx context.Context, name string) error {
	if err := validateHostname(name); err != nil {
		return err
	}
	_, err := m.run(ctx, m.hostnamectl(), "set-hostname", name)
	return err
}

// ---------- SetTimezone ----------

// validateTimezone rejects obviously bogus inputs (NUL, leading dash,
// path traversal) and confirms the zoneinfo file exists.
func (m *Manager) validateTimezone(tz string) error {
	if tz == "" {
		return errors.New("timezone required")
	}
	if strings.Contains(tz, "\x00") {
		return errors.New("timezone contains NUL")
	}
	if strings.HasPrefix(tz, "-") {
		return fmt.Errorf("timezone must not start with '-': %q", tz)
	}
	if strings.HasPrefix(tz, "/") {
		return fmt.Errorf("timezone must not be absolute: %q", tz)
	}
	for _, seg := range strings.Split(tz, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return fmt.Errorf("invalid timezone path %q", tz)
		}
	}
	// Best-effort: the zoneinfo file must exist on disk.
	path := filepath.Join(m.zoneinfoRoot(), tz)
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("unknown timezone %q: %w", tz, err)
	}
	return nil
}

// SetTimezone applies the new timezone via timedatectl.
func (m *Manager) SetTimezone(ctx context.Context, tz string) error {
	if err := m.validateTimezone(tz); err != nil {
		return err
	}
	_, err := m.run(ctx, m.timedatectl(), "set-timezone", tz)
	return err
}

// ---------- SetNTP ----------

// validateNTPServer keeps obviously hostile values out of timesyncd.conf.
func validateNTPServer(s string) error {
	if s == "" {
		return errors.New("empty NTP server")
	}
	if strings.ContainsAny(s, " \t\r\n\x00") {
		return fmt.Errorf("NTP server contains whitespace or NUL: %q", s)
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("NTP server contains control character: %q", s)
		}
	}
	return nil
}

// SetNTP enables/disables timesyncd. If servers != nil, the list is
// written into /etc/systemd/timesyncd.conf as the [Time] NTP= line
// before applying. When enabled, systemd-timesyncd is restarted.
func (m *Manager) SetNTP(ctx context.Context, enabled bool, servers []string) error {
	if servers != nil {
		for _, s := range servers {
			if err := validateNTPServer(s); err != nil {
				return err
			}
		}
		if err := m.writeTimesyncdConf(servers); err != nil {
			return err
		}
	}
	state := "false"
	if enabled {
		state = "true"
	}
	if _, err := m.run(ctx, m.timedatectl(), "set-ntp", state); err != nil {
		return err
	}
	if enabled {
		if _, err := m.run(ctx, m.systemctl(), "restart", "systemd-timesyncd"); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) writeTimesyncdConf(servers []string) error {
	dir := filepath.Join(m.etcRoot(), "systemd")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	var buf bytes.Buffer
	buf.WriteString("# Managed by nova-nas. Edits will be overwritten.\n")
	buf.WriteString("[Time]\n")
	buf.WriteString("NTP=")
	buf.WriteString(strings.Join(servers, " "))
	buf.WriteByte('\n')
	buf.WriteString("FallbackNTP=\n")
	path := filepath.Join(dir, "timesyncd.conf")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename %s: %w", path, err)
	}
	return nil
}

// ---------- Reboot / Shutdown / Cancel ----------

// minutesFromSeconds rounds delaySeconds up to the next whole minute.
// 0 returns 0; negative input is clamped to 0.
func minutesFromSeconds(delaySeconds int) int {
	if delaySeconds <= 0 {
		return 0
	}
	return (delaySeconds + 59) / 60
}

// Reboot reboots the host. With delaySeconds <= 0, `systemctl reboot
// --no-block` is invoked. Otherwise `shutdown -r +<min>` is scheduled.
func (m *Manager) Reboot(ctx context.Context, delaySeconds int) error {
	if delaySeconds <= 0 {
		_, err := m.run(ctx, m.systemctl(), "reboot", "--no-block")
		return err
	}
	mins := minutesFromSeconds(delaySeconds)
	_, err := m.run(ctx, m.shutdownBin(), "-r", fmt.Sprintf("+%d", mins))
	return err
}

// Shutdown powers off the host. With delaySeconds <= 0, `systemctl
// poweroff --no-block` is invoked. Otherwise `shutdown -h +<min>`.
func (m *Manager) Shutdown(ctx context.Context, delaySeconds int) error {
	if delaySeconds <= 0 {
		_, err := m.run(ctx, m.systemctl(), "poweroff", "--no-block")
		return err
	}
	mins := minutesFromSeconds(delaySeconds)
	_, err := m.run(ctx, m.shutdownBin(), "-h", fmt.Sprintf("+%d", mins))
	return err
}

// Cancel cancels a scheduled reboot/shutdown.
func (m *Manager) Cancel(ctx context.Context) error {
	_, err := m.run(ctx, m.shutdownBin(), "-c")
	return err
}
