package system

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeRunner is an exec.Runner that records every call and returns the
// next queued response.
type fakeRunner struct {
	mu    sync.Mutex
	calls []call
	// resp maps "bin args..." → (stdout, err). If a key is missing, the
	// runner returns ("", nil).
	resp map[string]fakeResp
}

type call struct {
	Bin  string
	Args []string
}

type fakeResp struct {
	out []byte
	err error
}

func (f *fakeRunner) run(ctx context.Context, bin string, args ...string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call{Bin: bin, Args: append([]string(nil), args...)})
	if f.resp == nil {
		return nil, nil
	}
	key := bin + " " + strings.Join(args, " ")
	if r, ok := f.resp[key]; ok {
		return r.out, r.err
	}
	return nil, nil
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{resp: map[string]fakeResp{}}
}

func newTestManager(t *testing.T) (*Manager, *fakeRunner) {
	t.Helper()
	fr := newFakeRunner()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	m := &Manager{
		Runner:         fr.run,
		HostnamectlBin: "/fake/hostnamectl",
		TimedatectlBin: "/fake/timedatectl",
		SystemctlBin:   "/fake/systemctl",
		UnameBin:       "/fake/uname",
		ZpoolBin:       "/fake/zpool",
		ShutdownBin:    "/fake/shutdown",
		ProcRoot:       filepath.Join(wd, "testdata", "proc"),
		EtcRoot:        filepath.Join(wd, "testdata", "etc"),
		ZoneinfoRoot:   filepath.Join(wd, "testdata", "zoneinfo"),
	}
	return m, fr
}

// ---------- /proc parsing ----------

func TestParseMeminfo(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("testdata", "proc", "meminfo"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	got := parseMeminfo(b)
	want := MemoryStats{
		TotalKB:     16384000,
		AvailKB:     12345678,
		SwapTotalKB: 2048000,
		SwapFreeKB:  2000000,
	}
	if got != want {
		t.Fatalf("parseMeminfo: got %+v want %+v", got, want)
	}
}

func TestParseCPUinfo(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("testdata", "proc", "cpuinfo"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	got := parseCPUinfo(b)
	if got.Threads != 4 {
		t.Errorf("Threads: got %d want 4", got.Threads)
	}
	if got.Sockets != 1 {
		t.Errorf("Sockets: got %d want 1", got.Sockets)
	}
	if got.Cores != 2 {
		t.Errorf("Cores: got %d want 2", got.Cores)
	}
	if !strings.Contains(got.Model, "Xeon") {
		t.Errorf("Model: got %q, expected Xeon", got.Model)
	}
}

func TestParseUptime(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("testdata", "proc", "uptime"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	d, ok := parseUptime(b)
	if !ok {
		t.Fatalf("parseUptime: ok=false")
	}
	want := time.Duration(123456.78 * float64(time.Second))
	// allow 1ms slack
	if diff := d - want; diff < -time.Millisecond || diff > time.Millisecond {
		t.Fatalf("parseUptime: got %v want %v", d, want)
	}
}

func TestParseLoadAvg(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("testdata", "proc", "loadavg"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	la, ok := parseLoadAvg(b)
	if !ok {
		t.Fatalf("ok=false")
	}
	want := [3]float64{0.15, 0.30, 0.45}
	if la != want {
		t.Fatalf("got %v want %v", la, want)
	}
}

func TestParseOSReleasePretty(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("testdata", "etc", "os-release"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	got := parseOSReleasePretty(b)
	want := "Debian GNU/Linux 12 (bookworm)"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestParseZpoolVersion(t *testing.T) {
	b := []byte("zfs-2.2.4-1\nzfs-kmod-2.2.4-1\n")
	if got := parseZpoolVersion(b); got != "zfs-2.2.4-1" {
		t.Fatalf("got %q want %q", got, "zfs-2.2.4-1")
	}
	if got := parseZpoolVersion(nil); got != "" {
		t.Fatalf("empty input: got %q", got)
	}
}

func TestParseTimesyncdNTP(t *testing.T) {
	b := []byte(`# comment
[Time]
NTP=time1.example time2.example
FallbackNTP=
`)
	got := parseTimesyncdNTP(b)
	want := []string{"time1.example", "time2.example"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}

	// Section gating: NTP= outside [Time] is ignored.
	b2 := []byte("[Other]\nNTP=foo\n")
	if got := parseTimesyncdNTP(b2); got != nil {
		t.Fatalf("want nil, got %v", got)
	}
}

// ---------- GetInfo ----------

func TestGetInfo(t *testing.T) {
	m, fr := newTestManager(t)
	fr.resp["/fake/uname -r"] = fakeResp{out: []byte("6.1.0-test\n")}
	fr.resp["/fake/uname -m"] = fakeResp{out: []byte("x86_64\n")}
	fr.resp["/fake/zpool version"] = fakeResp{out: []byte("zfs-2.2.4-1\nzfs-kmod-2.2.4-1\n")}

	info, err := m.GetInfo(context.Background())
	if err != nil {
		t.Fatalf("GetInfo: %v", err)
	}
	if info.Hostname != "novanas-test" {
		t.Errorf("Hostname: got %q", info.Hostname)
	}
	if !strings.Contains(info.OSPretty, "Debian") {
		t.Errorf("OSPretty: got %q", info.OSPretty)
	}
	if info.KernelRelease != "6.1.0-test" {
		t.Errorf("KernelRelease: got %q", info.KernelRelease)
	}
	if info.Architecture != "x86_64" {
		t.Errorf("Architecture: got %q", info.Architecture)
	}
	if info.ZFSVersion != "zfs-2.2.4-1" {
		t.Errorf("ZFSVersion: got %q", info.ZFSVersion)
	}
	if info.Memory.TotalKB != 16384000 {
		t.Errorf("Memory.TotalKB: got %d", info.Memory.TotalKB)
	}
	if info.CPU.Threads != 4 || info.CPU.Cores != 2 || info.CPU.Sockets != 1 {
		t.Errorf("CPU: got %+v", info.CPU)
	}
	if info.LoadAvg != [3]float64{0.15, 0.30, 0.45} {
		t.Errorf("LoadAvg: got %v", info.LoadAvg)
	}
	if time.Duration(info.Uptime) <= 0 {
		t.Errorf("Uptime not parsed: %v", info.Uptime)
	}
}

// ---------- GetTimeConfig ----------

func TestGetTimeConfig(t *testing.T) {
	m, fr := newTestManager(t)
	// Stage timesyncd.conf in our temp etc.
	systemdDir := filepath.Join(m.EtcRoot, "systemd")
	if err := os.MkdirAll(systemdDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	confPath := filepath.Join(systemdDir, "timesyncd.conf")
	if err := os.WriteFile(confPath, []byte("[Time]\nNTP=a.example b.example\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(confPath); _ = os.Remove(systemdDir) })

	fr.resp["/fake/timedatectl show --no-pager"] = fakeResp{out: []byte("Timezone=Europe/Brussels\nNTP=yes\nNTPSynchronized=yes\nLocalRTC=no\n")}

	tc, err := m.GetTimeConfig(context.Background())
	if err != nil {
		t.Fatalf("GetTimeConfig: %v", err)
	}
	if tc.Timezone != "Europe/Brussels" {
		t.Errorf("Timezone: got %q", tc.Timezone)
	}
	if !tc.NTP {
		t.Errorf("NTP: got false")
	}
	if !tc.Synchronized {
		t.Errorf("Synchronized: got false")
	}
	want := []string{"a.example", "b.example"}
	if !reflect.DeepEqual(tc.NTPServers, want) {
		t.Errorf("NTPServers: got %v want %v", tc.NTPServers, want)
	}
}

// ---------- Hostname validation ----------

func TestValidateHostname(t *testing.T) {
	good := []string{
		"a", "host", "node-1", "abcdefghij0123456789",
		strings.Repeat("a", 63),
	}
	for _, n := range good {
		if err := validateHostname(n); err != nil {
			t.Errorf("validateHostname(%q): unexpected error %v", n, err)
		}
	}
	bad := map[string]string{
		"":             "empty",
		strings.Repeat("a", 64): "too long",
		"-bad":           "leading hyphen",
		"bad-":           "trailing hyphen",
		"has space":      "space",
		"has.dot":        "dot",
		"has_underscore": "underscore",
		"unicode-é":      "non-ascii",
		"with\x00nul":    "NUL",
	}
	for n, why := range bad {
		if err := validateHostname(n); err == nil {
			t.Errorf("validateHostname(%q) [%s]: want error, got nil", n, why)
		}
	}
}

func TestSetHostnameArgv(t *testing.T) {
	m, fr := newTestManager(t)
	if err := m.SetHostname(context.Background(), "novanas-1"); err != nil {
		t.Fatalf("SetHostname: %v", err)
	}
	if len(fr.calls) != 1 {
		t.Fatalf("calls: %+v", fr.calls)
	}
	c := fr.calls[0]
	if c.Bin != "/fake/hostnamectl" {
		t.Errorf("Bin: got %q", c.Bin)
	}
	if !reflect.DeepEqual(c.Args, []string{"set-hostname", "novanas-1"}) {
		t.Errorf("Args: got %v", c.Args)
	}

	// Validation rejects bad name without invoking runner.
	fr.calls = nil
	if err := m.SetHostname(context.Background(), "-bad"); err == nil {
		t.Fatal("want error")
	}
	if len(fr.calls) != 0 {
		t.Errorf("runner called on validation failure: %+v", fr.calls)
	}
}

// ---------- Timezone validation + SetTimezone ----------

func TestSetTimezoneArgv(t *testing.T) {
	m, fr := newTestManager(t)
	if err := m.SetTimezone(context.Background(), "Europe/Brussels"); err != nil {
		t.Fatalf("SetTimezone: %v", err)
	}
	if len(fr.calls) != 1 {
		t.Fatalf("calls: %+v", fr.calls)
	}
	c := fr.calls[0]
	if c.Bin != "/fake/timedatectl" {
		t.Errorf("Bin: got %q", c.Bin)
	}
	if !reflect.DeepEqual(c.Args, []string{"set-timezone", "Europe/Brussels"}) {
		t.Errorf("Args: got %v", c.Args)
	}
}

func TestSetTimezoneRejectsUnknown(t *testing.T) {
	m, fr := newTestManager(t)
	bad := []string{
		"",
		"/etc/passwd",
		"-flag",
		"../etc",
		"with\x00nul",
		"Mars/Olympus_Mons", // not in zoneinfo fixture
	}
	for _, tz := range bad {
		fr.calls = nil
		if err := m.SetTimezone(context.Background(), tz); err == nil {
			t.Errorf("SetTimezone(%q): want error", tz)
		}
		if len(fr.calls) != 0 {
			t.Errorf("SetTimezone(%q): runner called: %+v", tz, fr.calls)
		}
	}
}

// ---------- SetNTP ----------

func TestSetNTPEnableWithServers(t *testing.T) {
	m, fr := newTestManager(t)
	// Need a writable etc/systemd dir; clean afterward.
	t.Cleanup(func() {
		_ = os.Remove(filepath.Join(m.EtcRoot, "systemd", "timesyncd.conf"))
		_ = os.Remove(filepath.Join(m.EtcRoot, "systemd"))
	})
	servers := []string{"time1.example", "time2.example"}
	if err := m.SetNTP(context.Background(), true, servers); err != nil {
		t.Fatalf("SetNTP: %v", err)
	}

	// Verify file content.
	b, err := os.ReadFile(filepath.Join(m.EtcRoot, "systemd", "timesyncd.conf"))
	if err != nil {
		t.Fatalf("read written conf: %v", err)
	}
	if !strings.Contains(string(b), "NTP=time1.example time2.example") {
		t.Errorf("conf missing NTP line: %s", b)
	}
	if !strings.Contains(string(b), "[Time]") {
		t.Errorf("conf missing [Time]: %s", b)
	}

	// Verify argv shape: timedatectl set-ntp true, then systemctl restart.
	if len(fr.calls) != 2 {
		t.Fatalf("calls: %+v", fr.calls)
	}
	if fr.calls[0].Bin != "/fake/timedatectl" ||
		!reflect.DeepEqual(fr.calls[0].Args, []string{"set-ntp", "true"}) {
		t.Errorf("call[0]: %+v", fr.calls[0])
	}
	if fr.calls[1].Bin != "/fake/systemctl" ||
		!reflect.DeepEqual(fr.calls[1].Args, []string{"restart", "systemd-timesyncd"}) {
		t.Errorf("call[1]: %+v", fr.calls[1])
	}
}

func TestSetNTPDisableNoServers(t *testing.T) {
	m, fr := newTestManager(t)
	if err := m.SetNTP(context.Background(), false, nil); err != nil {
		t.Fatalf("SetNTP: %v", err)
	}
	if len(fr.calls) != 1 {
		t.Fatalf("calls: %+v", fr.calls)
	}
	if !reflect.DeepEqual(fr.calls[0].Args, []string{"set-ntp", "false"}) {
		t.Errorf("Args: %v", fr.calls[0].Args)
	}
}

func TestSetNTPRejectsBadServer(t *testing.T) {
	m, fr := newTestManager(t)
	if err := m.SetNTP(context.Background(), true, []string{"good", "bad server"}); err == nil {
		t.Fatal("want error")
	}
	if len(fr.calls) != 0 {
		t.Errorf("runner called on validation failure: %+v", fr.calls)
	}
}

// ---------- Reboot / Shutdown / Cancel ----------

func TestRebootImmediate(t *testing.T) {
	m, fr := newTestManager(t)
	if err := m.Reboot(context.Background(), 0); err != nil {
		t.Fatalf("Reboot: %v", err)
	}
	if len(fr.calls) != 1 {
		t.Fatalf("calls: %+v", fr.calls)
	}
	if fr.calls[0].Bin != "/fake/systemctl" ||
		!reflect.DeepEqual(fr.calls[0].Args, []string{"reboot", "--no-block"}) {
		t.Errorf("call: %+v", fr.calls[0])
	}
}

func TestRebootDelayed(t *testing.T) {
	m, fr := newTestManager(t)
	// 90s should round up to 2 minutes.
	if err := m.Reboot(context.Background(), 90); err != nil {
		t.Fatalf("Reboot: %v", err)
	}
	if len(fr.calls) != 1 {
		t.Fatalf("calls: %+v", fr.calls)
	}
	if fr.calls[0].Bin != "/fake/shutdown" ||
		!reflect.DeepEqual(fr.calls[0].Args, []string{"-r", "+2"}) {
		t.Errorf("call: %+v", fr.calls[0])
	}
}

func TestShutdownImmediate(t *testing.T) {
	m, fr := newTestManager(t)
	if err := m.Shutdown(context.Background(), 0); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if len(fr.calls) != 1 {
		t.Fatalf("calls: %+v", fr.calls)
	}
	if fr.calls[0].Bin != "/fake/systemctl" ||
		!reflect.DeepEqual(fr.calls[0].Args, []string{"poweroff", "--no-block"}) {
		t.Errorf("call: %+v", fr.calls[0])
	}
}

func TestShutdownDelayed(t *testing.T) {
	m, fr := newTestManager(t)
	if err := m.Shutdown(context.Background(), 60); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if fr.calls[0].Bin != "/fake/shutdown" ||
		!reflect.DeepEqual(fr.calls[0].Args, []string{"-h", "+1"}) {
		t.Errorf("call: %+v", fr.calls[0])
	}
}

func TestCancel(t *testing.T) {
	m, fr := newTestManager(t)
	if err := m.Cancel(context.Background()); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if len(fr.calls) != 1 {
		t.Fatalf("calls: %+v", fr.calls)
	}
	if fr.calls[0].Bin != "/fake/shutdown" ||
		!reflect.DeepEqual(fr.calls[0].Args, []string{"-c"}) {
		t.Errorf("call: %+v", fr.calls[0])
	}
}

func TestMinutesFromSeconds(t *testing.T) {
	cases := map[int]int{
		-5: 0, 0: 0, 1: 1, 59: 1, 60: 1, 61: 2, 90: 2, 120: 2, 121: 3,
	}
	for in, want := range cases {
		if got := minutesFromSeconds(in); got != want {
			t.Errorf("minutesFromSeconds(%d): got %d want %d", in, got, want)
		}
	}
}
