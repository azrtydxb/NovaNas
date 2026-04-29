package smart

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/novanas/nova-nas/internal/host/exec"
)

// fakeRunner returns canned bytes/err. It records the args of each call.
type fakeRunner struct {
	out  []byte
	err  error
	args [][]string
}

func (f *fakeRunner) run(_ context.Context, _ string, args ...string) ([]byte, error) {
	cp := append([]string(nil), args...)
	f.args = append(f.args, cp)
	return f.out, f.err
}

func newManager(r *fakeRunner) *Manager {
	return &Manager{SmartctlBin: "/usr/sbin/smartctl", Runner: r.run}
}

// ---------- fixtures ----------

// Healthy SATA HDD. ata_smart_attributes.table has Reallocated_Sector_Ct
// healthy, no when_failed.
const sataHDDJSON = `{
  "device": {"name": "/dev/sda", "type": "sat"},
  "model_name": "WDC WD40EFRX-68N32N0",
  "serial_number": "WD-WCC7K1234567",
  "firmware_version": "82.00A82",
  "user_capacity": {"bytes": 4000787030016},
  "smart_status": {"passed": true},
  "temperature": {"current": 34},
  "power_on_time": {"hours": 12345},
  "power_cycle_count": 42,
  "ata_smart_attributes": {
    "table": [
      {"id": 5, "name": "Reallocated_Sector_Ct", "value": 200, "worst": 200, "thresh": 140, "when_failed": "", "raw": {"value": 0, "string": "0"}},
      {"id": 9, "name": "Power_On_Hours", "value": 80, "worst": 80, "thresh": 0, "when_failed": "", "raw": {"value": 12345, "string": "12345"}},
      {"id": 194, "name": "Temperature_Celsius", "value": 120, "worst": 105, "thresh": 0, "when_failed": "", "raw": {"value": 34, "string": "34 (Min/Max 20/45)"}}
    ]
  },
  "ata_smart_self_test_log": {
    "standard": {
      "table": [
        {"type": {"string": "Short offline"}, "status": {"string": "Completed without error", "passed": true}, "lifetime_hours": 12340, "lba_of_first_error": 0}
      ]
    }
  }
}`

// SATA SSD with Reallocated_Sector_Ct FAILING_NOW.
const sataSSDFailingJSON = `{
  "device": {"name": "/dev/sdb", "type": "sat"},
  "model_name": "Samsung SSD 870 EVO 1TB",
  "serial_number": "S6PWNX0R123456",
  "firmware_version": "SVT01B6Q",
  "user_capacity": {"bytes": 1000204886016},
  "smart_status": {"passed": true},
  "temperature": {"current": 41},
  "power_on_time": {"hours": 8760},
  "power_cycle_count": 100,
  "ata_smart_attributes": {
    "table": [
      {"id": 5, "name": "Reallocated_Sector_Ct", "value": 50, "worst": 50, "thresh": 100, "when_failed": "FAILING_NOW", "raw": {"value": 250, "string": "250"}},
      {"id": 9, "name": "Power_On_Hours", "value": 95, "worst": 95, "thresh": 0, "when_failed": "", "raw": {"value": 8760, "string": "8760"}},
      {"id": 187, "name": "Reported_Uncorrect", "value": 99, "worst": 99, "thresh": 0, "when_failed": "Past", "raw": {"value": 1, "string": "1"}}
    ]
  }
}`

// NVMe drive — no ata_smart_attributes; nvme_smart_health_information_log.
const nvmeJSON = `{
  "device": {"name": "/dev/nvme0", "type": "nvme"},
  "model_name": "Samsung SSD 980 PRO 2TB",
  "serial_number": "S6XXNSAR123456",
  "firmware_version": "5B2QGXA7",
  "nvme_total_capacity": 2000398934016,
  "user_capacity": {"bytes": 0},
  "smart_status": {"passed": true},
  "temperature": {"current": 38},
  "power_cycle_count": 75,
  "nvme_smart_health_information_log": {
    "critical_warning": 0,
    "temperature": 38,
    "available_spare": 100,
    "available_spare_threshold": 10,
    "percentage_used": 3,
    "power_cycles": 75,
    "power_on_hours": 4321,
    "unsafe_shutdowns": 7,
    "media_errors": 0,
    "num_err_log_entries": 0
  },
  "nvme_self_test_log": {
    "table": [
      {"self_test_code": {"string": "Short"}, "self_test_result": {"string": "Completed without error"}, "power_on_hours": 4300, "lba_of_first_error": 0}
    ]
  }
}`

// NVMe drive that reports critical_warning != 0 and media_errors > 0.
const nvmeFailingJSON = `{
  "device": {"name": "/dev/nvme1", "type": "nvme"},
  "model_name": "Generic NVMe",
  "serial_number": "NVMEFAIL0001",
  "firmware_version": "1.0",
  "nvme_total_capacity": 500107862016,
  "user_capacity": {"bytes": 0},
  "smart_status": {"passed": false},
  "temperature": {"current": 72},
  "nvme_smart_health_information_log": {
    "critical_warning": 4,
    "temperature": 72,
    "available_spare": 5,
    "available_spare_threshold": 10,
    "percentage_used": 105,
    "power_cycles": 1000,
    "power_on_hours": 50000,
    "unsafe_shutdowns": 12,
    "media_errors": 7,
    "num_err_log_entries": 3
  }
}`

// ---------- tests ----------

func TestGet_HealthySATA(t *testing.T) {
	r := &fakeRunner{out: []byte(sataHDDJSON)}
	m := newManager(r)
	h, err := m.Get(context.Background(), "/dev/sda")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h.Model != "WDC WD40EFRX-68N32N0" {
		t.Errorf("Model=%q", h.Model)
	}
	if h.SerialNumber != "WD-WCC7K1234567" {
		t.Errorf("Serial=%q", h.SerialNumber)
	}
	if h.Firmware != "82.00A82" {
		t.Errorf("Firmware=%q", h.Firmware)
	}
	if h.CapacityBytes != 4000787030016 {
		t.Errorf("Capacity=%d", h.CapacityBytes)
	}
	if !h.OverallPassed {
		t.Errorf("OverallPassed=false, want true")
	}
	if h.Temperature == nil || *h.Temperature != 34 {
		t.Errorf("Temperature=%v", h.Temperature)
	}
	if h.PowerOnHours == nil || *h.PowerOnHours != 12345 {
		t.Errorf("PowerOnHours=%v", h.PowerOnHours)
	}
	if h.PowerCycles == nil || *h.PowerCycles != 42 {
		t.Errorf("PowerCycles=%v", h.PowerCycles)
	}
	if len(h.Attributes) != 3 {
		t.Fatalf("attrs=%d, want 3", len(h.Attributes))
	}
	if h.HasErrors {
		t.Errorf("HasErrors=true, want false (errors=%v)", h.ErrorSummary)
	}
	if h.LastTest == nil || h.LastTest.Status != "Completed without error" {
		t.Errorf("LastTest=%+v", h.LastTest)
	}
	// args check: -a -j /dev/sda
	if len(r.args) != 1 || len(r.args[0]) != 3 ||
		r.args[0][0] != "-a" || r.args[0][1] != "-j" || r.args[0][2] != "/dev/sda" {
		t.Errorf("args=%v", r.args)
	}
}

func TestGet_FailingAttribute(t *testing.T) {
	r := &fakeRunner{out: []byte(sataSSDFailingJSON)}
	m := newManager(r)
	h, err := m.Get(context.Background(), "/dev/sdb")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !h.HasErrors {
		t.Fatalf("HasErrors=false, want true")
	}
	// Both FAILING_NOW and Past should appear in the summary.
	joined := strings.Join(h.ErrorSummary, "|")
	if !strings.Contains(joined, "Reallocated_Sector_Ct") || !strings.Contains(joined, "FAILING_NOW") {
		t.Errorf("ErrorSummary missing FAILING_NOW entry: %v", h.ErrorSummary)
	}
	if !strings.Contains(joined, "Reported_Uncorrect") || !strings.Contains(joined, "Past") {
		t.Errorf("ErrorSummary missing Past entry: %v", h.ErrorSummary)
	}
}

func TestGet_NVMeHealthy(t *testing.T) {
	r := &fakeRunner{out: []byte(nvmeJSON)}
	m := newManager(r)
	h, err := m.Get(context.Background(), "/dev/nvme0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h.CapacityBytes != 2000398934016 {
		t.Errorf("Capacity=%d (expected nvme_total_capacity fallback)", h.CapacityBytes)
	}
	if h.PowerOnHours == nil || *h.PowerOnHours != 4321 {
		t.Errorf("PowerOnHours=%v", h.PowerOnHours)
	}
	if h.HasErrors {
		t.Errorf("HasErrors=true, want false (%v)", h.ErrorSummary)
	}
	// available_spare attr should exist with threshold populated.
	var foundSpare bool
	for _, a := range h.Attributes {
		if a.Name == "available_spare" {
			foundSpare = true
			if a.Value != 100 || a.Threshold != 10 {
				t.Errorf("available_spare value=%d threshold=%d", a.Value, a.Threshold)
			}
		}
	}
	if !foundSpare {
		t.Errorf("available_spare attribute missing")
	}
	if h.LastTest == nil || h.LastTest.Type != "Short" {
		t.Errorf("LastTest=%+v", h.LastTest)
	}
}

func TestGet_NVMeFailing(t *testing.T) {
	r := &fakeRunner{out: []byte(nvmeFailingJSON)}
	m := newManager(r)
	h, err := m.Get(context.Background(), "/dev/nvme1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !h.HasErrors {
		t.Errorf("HasErrors=false, want true")
	}
	if h.OverallPassed {
		t.Errorf("OverallPassed=true, want false")
	}
	joined := strings.Join(h.ErrorSummary, "|")
	for _, want := range []string{"critical_warning", "available_spare", "percentage_used", "media_errors", "FAILED"} {
		if !strings.Contains(joined, want) {
			t.Errorf("ErrorSummary missing %q in %v", want, h.ErrorSummary)
		}
	}
}

func TestGet_ExitBit2_FailingHealth_NotErr(t *testing.T) {
	// smartctl exit bit 2 set: SMART status FAILING. Output JSON is still
	// produced; we want HasErrors+OverallPassed=false, no Go error.
	hostErr := &exec.HostError{
		Bin:      "/usr/sbin/smartctl",
		Args:     []string{"-a", "-j", "/dev/sdc"},
		ExitCode: 1 << 2,
		Stderr:   "",
	}
	// Variant of the failing JSON with smart_status.passed=false.
	failHealthJSON := strings.Replace(sataSSDFailingJSON, `"passed": true`, `"passed": false`, 1)
	r := &fakeRunner{out: []byte(failHealthJSON), err: hostErr}
	m := newManager(r)
	h, err := m.Get(context.Background(), "/dev/sdc")
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if h.OverallPassed {
		t.Errorf("OverallPassed=true, want false")
	}
	if !h.HasErrors {
		t.Errorf("HasErrors=false, want true")
	}
}

func TestGet_ExitBits3to7_NonFatal(t *testing.T) {
	// Bit 5 (error log has records) + bit 6 (self-test log has errors).
	hostErr := &exec.HostError{
		Bin:      "/usr/sbin/smartctl",
		Args:     []string{"-a", "-j", "/dev/sda"},
		ExitCode: (1 << 5) | (1 << 6),
	}
	r := &fakeRunner{out: []byte(sataHDDJSON), err: hostErr}
	m := newManager(r)
	h, err := m.Get(context.Background(), "/dev/sda")
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if !h.HasErrors {
		t.Errorf("HasErrors=false, want true (warnings flagged via exit bits)")
	}
	joined := strings.Join(h.ErrorSummary, "|")
	if !strings.Contains(joined, "warnings") {
		t.Errorf("expected exit-code warning in summary, got %v", h.ErrorSummary)
	}
}

func TestGet_ExitBit0_FatalParseError(t *testing.T) {
	hostErr := &exec.HostError{
		Bin:      "/usr/sbin/smartctl",
		Args:     []string{"-a", "-j", "/dev/sda"},
		ExitCode: 1 << 0,
		Stderr:   "unrecognized option",
	}
	r := &fakeRunner{out: nil, err: hostErr}
	m := newManager(r)
	if _, err := m.Get(context.Background(), "/dev/sda"); err == nil {
		t.Fatalf("expected fatal error from bit 0")
	}
}

func TestGet_ExitBit1_DeviceOpenFailed(t *testing.T) {
	hostErr := &exec.HostError{
		Bin:      "/usr/sbin/smartctl",
		Args:     []string{"-a", "-j", "/dev/sdz"},
		ExitCode: 1 << 1,
		Stderr:   "open failed",
	}
	r := &fakeRunner{out: nil, err: hostErr}
	m := newManager(r)
	_, err := m.Get(context.Background(), "/dev/sdz")
	if err == nil {
		t.Fatalf("expected fatal error from bit 1")
	}
	var he *exec.HostError
	if !errors.As(err, &he) {
		t.Errorf("expected *HostError, got %T", err)
	}
}

func TestValidateDevicePath(t *testing.T) {
	cases := []struct {
		in  string
		ok  bool
	}{
		{"/dev/sda", true},
		{"/dev/nvme0n1", true},
		{"/dev/disk/by-id/wwn-0x123", true},
		{"", false},
		{"sda", false},          // not absolute /dev
		{"/etc/passwd", false},  // wrong prefix
		{"-/dev/sda", false},    // leading dash
		{"/dev/../etc/x", false},
		{"/dev/sda;ls", false},
		{"/dev/sda foo", false},
		{"/dev/sd$a", false},
	}
	for _, c := range cases {
		err := validateDevicePath(c.in)
		if (err == nil) != c.ok {
			t.Errorf("validateDevicePath(%q): err=%v, want ok=%v", c.in, err, c.ok)
		}
	}
}

func TestGet_RejectsBadPath(t *testing.T) {
	m := newManager(&fakeRunner{})
	if _, err := m.Get(context.Background(), "-rf"); err == nil {
		t.Errorf("expected error for leading-dash path")
	}
	if _, err := m.Get(context.Background(), "/etc/passwd"); err == nil {
		t.Errorf("expected error for non-/dev path")
	}
}

func TestRunSelfTest(t *testing.T) {
	r := &fakeRunner{}
	m := newManager(r)
	if err := m.RunSelfTest(context.Background(), "/dev/sda", "short"); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(r.args) != 1 || r.args[0][0] != "-t" || r.args[0][1] != "short" || r.args[0][2] != "/dev/sda" {
		t.Errorf("args=%v", r.args)
	}
	if err := m.RunSelfTest(context.Background(), "/dev/sda", "bogus"); err == nil {
		t.Errorf("expected error for bogus test type")
	}
	if err := m.RunSelfTest(context.Background(), "-rf", "short"); err == nil {
		t.Errorf("expected error for bad device path")
	}
}

func TestEnableSMART(t *testing.T) {
	r := &fakeRunner{}
	m := newManager(r)
	if err := m.EnableSMART(context.Background(), "/dev/sda"); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(r.args) != 1 || r.args[0][0] != "--smart=on" || r.args[0][1] != "/dev/sda" {
		t.Errorf("args=%v", r.args)
	}
}

func TestGet_EmptyOutput(t *testing.T) {
	r := &fakeRunner{out: nil}
	m := newManager(r)
	if _, err := m.Get(context.Background(), "/dev/sda"); err == nil {
		t.Errorf("expected error on empty output")
	}
}
