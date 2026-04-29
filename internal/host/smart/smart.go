// Package smart wraps smartctl(8) to expose per-disk SMART health data.
//
// All shell-out goes through exec.Runner so tests can stub it. Output is
// parsed exclusively from smartctl's JSON mode (-j), which has been stable
// since smartctl 7.0. The legacy text-parse path is intentionally not
// supported.
//
// Exit-code handling follows smartctl(8): the exit code is a bitmask.
//
//	bit 0 - command line did not parse
//	bit 1 - device open failed / SMART/ATA command to disk failed
//	bit 2 - SMART/ATA returned a checksum error or "FAILING" status
//	bit 3 - disk is or has been used outside of design parameters
//	bit 4 - SMART status check returned "DISK OK" but had threshold-exceeded
//	        attributes in the past
//	bit 5 - error log contains records of errors
//	bit 6 - self-test log contains records of errors
//	bit 7 - device is in low-power mode (only set when --nocheck used)
//
// Bits 0-2 are surfaced as Go errors. Bits 3-7 mean "the run produced a
// usable JSON document, but there are warnings"; we still parse the
// document, set HasErrors=true, and return Health with no error.
package smart

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/novanas/nova-nas/internal/host/exec"
)

// Health is the per-disk SMART summary returned by Manager.Get.
type Health struct {
	DeviceName    string      `json:"deviceName"`
	Model         string      `json:"model"`
	SerialNumber  string      `json:"serialNumber"`
	Firmware      string      `json:"firmware"`
	CapacityBytes int64       `json:"capacityBytes"`
	OverallPassed bool        `json:"overallPassed"`
	Temperature   *int        `json:"temperatureC,omitempty"`
	PowerOnHours  *int        `json:"powerOnHours,omitempty"`
	PowerCycles   *int        `json:"powerCycles,omitempty"`
	Attributes    []Attribute `json:"attributes,omitempty"`
	LastTest      *SelfTest   `json:"lastTest,omitempty"`
	HasErrors     bool        `json:"hasErrors"`
	ErrorSummary  []string    `json:"errorSummary,omitempty"`
}

// Attribute is one row of the ATA SMART attribute table. NVMe devices
// don't have an ATA-style table; for those we synthesize Attribute rows
// from the nvme_smart_health_information_log fields that have a defined
// "critical" or "warning" semantic, so HasErrors stays meaningful across
// transports.
type Attribute struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	Value      int    `json:"value"`
	Worst      int    `json:"worst"`
	Threshold  int    `json:"threshold"`
	RawValue   string `json:"rawValue"`
	WhenFailed string `json:"whenFailed,omitempty"`
}

// SelfTest is the most recent self-test entry.
type SelfTest struct {
	Type            string `json:"type"`
	Status          string `json:"status"`
	LifetimeHrs     int    `json:"lifetimeHours"`
	LBAOfFirstError int64  `json:"lbaOfFirstError,omitempty"`
}

// Manager wraps smartctl invocations.
type Manager struct {
	SmartctlBin string // default /usr/sbin/smartctl
	Runner      exec.Runner
}

func (m *Manager) bin() string {
	if m.SmartctlBin == "" {
		return "/usr/sbin/smartctl"
	}
	return m.SmartctlBin
}

func (m *Manager) run(ctx context.Context, args ...string) ([]byte, error) {
	r := m.Runner
	if r == nil {
		r = exec.Run
	}
	return r(ctx, m.bin(), args...)
}

// validateDevicePath enforces an absolute /dev/<name> path with no shell
// metacharacters or path traversal. Smartctl is invoked via exec without a
// shell, but operators sometimes wire user input straight through; the
// stricter check costs nothing.
func validateDevicePath(p string) error {
	if p == "" {
		return fmt.Errorf("device path required")
	}
	if !strings.HasPrefix(p, "/dev/") {
		return fmt.Errorf("device path must start with /dev/: %q", p)
	}
	if strings.HasPrefix(p, "-") {
		return fmt.Errorf("device path cannot start with '-': %q", p)
	}
	if strings.Contains(p, "\x00") {
		return fmt.Errorf("device path contains NUL")
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return fmt.Errorf("device path must not contain '..': %q", p)
		}
	}
	for _, r := range p {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("device path contains control character: %q", p)
		}
		switch r {
		case ' ', '\t', ';', '|', '&', '$', '`', '\\', '"', '\'', '<', '>', '*', '?', '(', ')', '{', '}', '[', ']':
			return fmt.Errorf("device path contains forbidden character %q: %q", r, p)
		}
	}
	return nil
}

func validateTestType(t string) error {
	switch t {
	case "short", "long", "conveyance", "abort":
		return nil
	}
	return fmt.Errorf("invalid test type %q (want short|long|conveyance|abort)", t)
}

// classifyExit splits a smartctl error into (fatal, nonFatalBits, output-was-produced).
// If err is not a *HostError, it's treated as fatal.
func classifyExit(err error) (fatal bool, nonFatalBits int, isHost bool) {
	if err == nil {
		return false, 0, false
	}
	var he *exec.HostError
	if !errors.As(err, &he) {
		return true, 0, false
	}
	code := he.ExitCode
	// bits 0,1,2 are fatal
	fatalMask := (1 << 0) | (1 << 1) | (1 << 2)
	if code&fatalMask != 0 {
		return true, code &^ fatalMask, true
	}
	return false, code, true
}

// Get returns the parsed SMART health for devicePath.
func (m *Manager) Get(ctx context.Context, devicePath string) (*Health, error) {
	if err := validateDevicePath(devicePath); err != nil {
		return nil, err
	}
	out, err := m.run(ctx, "-a", "-j", devicePath)
	fatal, nonFatalBits, isHost := classifyExit(err)
	if fatal {
		// bit 2 (SMART status FAILING) we still want to surface as parsed
		// Health, not a Go error. Re-check: classifyExit grouped 0,1,2 as
		// fatal — pull bit 2 out and treat it as warning-only when the
		// JSON document is present.
		if isHost {
			var he *exec.HostError
			_ = errors.As(err, &he)
			if he.ExitCode&((1<<0)|(1<<1)) == 0 && he.ExitCode&(1<<2) != 0 && len(out) > 0 {
				return parseSmartctlJSON(devicePath, out, he.ExitCode)
			}
		}
		return nil, err
	}
	return parseSmartctlJSON(devicePath, out, nonFatalBits)
}

// RunSelfTest issues a SMART self-test of the named type.
func (m *Manager) RunSelfTest(ctx context.Context, devicePath, testType string) error {
	if err := validateDevicePath(devicePath); err != nil {
		return err
	}
	if err := validateTestType(testType); err != nil {
		return err
	}
	_, err := m.run(ctx, "-t", testType, devicePath)
	if fatal, _, _ := classifyExit(err); fatal {
		return err
	}
	return nil
}

// EnableSMART turns on SMART for drives that ship with it disabled.
func (m *Manager) EnableSMART(ctx context.Context, devicePath string) error {
	if err := validateDevicePath(devicePath); err != nil {
		return err
	}
	_, err := m.run(ctx, "--smart=on", devicePath)
	if fatal, _, _ := classifyExit(err); fatal {
		return err
	}
	return nil
}

// ---------- parsing ----------

// smartctlOutput is the subset of smartctl -j we consume. smartctl
// produces a large document; we cherry-pick.
type smartctlOutput struct {
	Device struct {
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"device"`
	ModelName        string `json:"model_name"`
	ModelFamily      string `json:"model_family"`
	SerialNumber     string `json:"serial_number"`
	FirmwareVersion  string `json:"firmware_version"`
	UserCapacity     struct {
		Bytes int64 `json:"bytes"`
	} `json:"user_capacity"`
	NVMeTotalCapacity *int64 `json:"nvme_total_capacity,omitempty"`

	SmartStatus struct {
		Passed bool `json:"passed"`
	} `json:"smart_status"`

	Temperature struct {
		Current *int `json:"current"`
	} `json:"temperature"`

	PowerOnTime struct {
		Hours *int `json:"hours"`
	} `json:"power_on_time"`
	PowerCycleCount *int `json:"power_cycle_count"`

	ATASmartAttributes struct {
		Table []ataAttr `json:"table"`
	} `json:"ata_smart_attributes"`

	NVMeLog *nvmeLog `json:"nvme_smart_health_information_log,omitempty"`

	ATASelfTestLog *struct {
		Standard struct {
			Table []ataSelfTestEntry `json:"table"`
		} `json:"standard"`
	} `json:"ata_smart_self_test_log,omitempty"`

	NVMeSelfTestLog *struct {
		Table []nvmeSelfTestEntry `json:"table"`
	} `json:"nvme_self_test_log,omitempty"`
}

type ataAttr struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	Value      int    `json:"value"`
	Worst      int    `json:"worst"`
	Thresh     int    `json:"thresh"`
	WhenFailed string `json:"when_failed"`
	Raw        struct {
		Value  int64  `json:"value"`
		String string `json:"string"`
	} `json:"raw"`
}

type nvmeLog struct {
	CriticalWarning             int  `json:"critical_warning"`
	Temperature                 *int `json:"temperature"`
	AvailableSpare              *int `json:"available_spare"`
	AvailableSpareThreshold     *int `json:"available_spare_threshold"`
	PercentageUsed              *int `json:"percentage_used"`
	PowerCycles                 *int `json:"power_cycles"`
	PowerOnHours                *int `json:"power_on_hours"`
	UnsafeShutdowns             *int `json:"unsafe_shutdowns"`
	MediaErrors                 *int `json:"media_errors"`
	NumErrLogEntries            *int `json:"num_err_log_entries"`
}

type ataSelfTestEntry struct {
	Type struct {
		String string `json:"string"`
	} `json:"type"`
	Status struct {
		String string `json:"string"`
		Passed *bool  `json:"passed"`
	} `json:"status"`
	LifetimeHours   int   `json:"lifetime_hours"`
	LBAOfFirstError int64 `json:"lba_of_first_error"`
}

type nvmeSelfTestEntry struct {
	SelfTestCode struct {
		String string `json:"string"`
	} `json:"self_test_code"`
	SelfTestResult struct {
		String string `json:"string"`
	} `json:"self_test_result"`
	PowerOnHours    int   `json:"power_on_hours"`
	LBAOfFirstError int64 `json:"lba_of_first_error"`
}

// parseSmartctlJSON parses the smartctl -j document. nonFatalBits is the
// remaining exit-code bitmask (3..7) so callers can flag warnings even when
// no individual attribute reports failure.
func parseSmartctlJSON(devicePath string, data []byte, nonFatalBits int) (*Health, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("smartctl produced no output for %s", devicePath)
	}
	var doc smartctlOutput
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse smartctl json: %w", err)
	}
	h := &Health{
		DeviceName:    devicePath,
		Model:         firstNonEmpty(doc.ModelName, doc.ModelFamily),
		SerialNumber:  doc.SerialNumber,
		Firmware:      doc.FirmwareVersion,
		CapacityBytes: doc.UserCapacity.Bytes,
		OverallPassed: doc.SmartStatus.Passed,
	}
	if h.CapacityBytes == 0 && doc.NVMeTotalCapacity != nil {
		h.CapacityBytes = *doc.NVMeTotalCapacity
	}
	if doc.Temperature.Current != nil {
		v := *doc.Temperature.Current
		h.Temperature = &v
	}
	if doc.PowerOnTime.Hours != nil {
		v := *doc.PowerOnTime.Hours
		h.PowerOnHours = &v
	}
	if doc.PowerCycleCount != nil {
		v := *doc.PowerCycleCount
		h.PowerCycles = &v
	}

	if strings.EqualFold(doc.Device.Type, "nvme") || doc.NVMeLog != nil {
		populateFromNVMe(h, doc.NVMeLog)
		populateNVMeSelfTest(h, doc.NVMeSelfTestLog)
	} else {
		populateFromATA(h, doc.ATASmartAttributes.Table)
		populateATASelfTest(h, doc.ATASelfTestLog)
	}

	// Compute HasErrors / ErrorSummary.
	if !h.OverallPassed {
		h.HasErrors = true
		h.ErrorSummary = append(h.ErrorSummary, "SMART overall-health self-assessment FAILED")
	}
	for _, a := range h.Attributes {
		if a.WhenFailed != "" {
			h.HasErrors = true
			h.ErrorSummary = append(h.ErrorSummary,
				fmt.Sprintf("%s value %d below threshold %d, %s",
					a.Name, a.Value, a.Threshold, a.WhenFailed))
		}
	}
	// Non-fatal exit bits indicate smartctl-detected warnings even when no
	// individual attribute is FAILING_NOW.
	if nonFatalBits != 0 && !h.HasErrors {
		h.HasErrors = true
		h.ErrorSummary = append(h.ErrorSummary,
			fmt.Sprintf("smartctl reported warnings (exit-code bits 0x%02x)", nonFatalBits))
	}
	return h, nil
}

func populateFromATA(h *Health, table []ataAttr) {
	for _, a := range table {
		raw := a.Raw.String
		if raw == "" {
			raw = fmt.Sprintf("%d", a.Raw.Value)
		}
		h.Attributes = append(h.Attributes, Attribute{
			ID:         a.ID,
			Name:       a.Name,
			Value:      a.Value,
			Worst:      a.Worst,
			Threshold:  a.Thresh,
			RawValue:   raw,
			WhenFailed: a.WhenFailed,
		})
	}
}

// populateFromNVMe synthesizes Attribute rows from the NVMe log so the
// downstream HasErrors logic works uniformly. Only fields with a defined
// "is bad" condition are flagged via WhenFailed.
func populateFromNVMe(h *Health, log *nvmeLog) {
	if log == nil {
		return
	}
	if log.PowerOnHours != nil && h.PowerOnHours == nil {
		v := *log.PowerOnHours
		h.PowerOnHours = &v
	}
	if log.PowerCycles != nil && h.PowerCycles == nil {
		v := *log.PowerCycles
		h.PowerCycles = &v
	}
	if log.Temperature != nil && h.Temperature == nil {
		v := *log.Temperature
		h.Temperature = &v
	}

	add := func(name string, value int, raw string, failed string) {
		h.Attributes = append(h.Attributes, Attribute{
			Name:       name,
			Value:      value,
			RawValue:   raw,
			WhenFailed: failed,
		})
	}
	if log.CriticalWarning != 0 {
		add("critical_warning", log.CriticalWarning,
			fmt.Sprintf("0x%02x", log.CriticalWarning), "FAILING_NOW")
	}
	if log.AvailableSpare != nil && log.AvailableSpareThreshold != nil {
		failed := ""
		if *log.AvailableSpare < *log.AvailableSpareThreshold {
			failed = "FAILING_NOW"
		}
		// Threshold field is meaningful here; reuse it.
		h.Attributes = append(h.Attributes, Attribute{
			Name:       "available_spare",
			Value:      *log.AvailableSpare,
			Threshold:  *log.AvailableSpareThreshold,
			RawValue:   fmt.Sprintf("%d", *log.AvailableSpare),
			WhenFailed: failed,
		})
	}
	if log.PercentageUsed != nil {
		failed := ""
		if *log.PercentageUsed >= 100 {
			failed = "FAILING_NOW"
		}
		add("percentage_used", *log.PercentageUsed,
			fmt.Sprintf("%d", *log.PercentageUsed), failed)
	}
	if log.MediaErrors != nil && *log.MediaErrors > 0 {
		add("media_errors", *log.MediaErrors,
			fmt.Sprintf("%d", *log.MediaErrors), "FAILING_NOW")
	}
	if log.UnsafeShutdowns != nil {
		add("unsafe_shutdowns", *log.UnsafeShutdowns,
			fmt.Sprintf("%d", *log.UnsafeShutdowns), "")
	}
	if log.NumErrLogEntries != nil {
		add("num_err_log_entries", *log.NumErrLogEntries,
			fmt.Sprintf("%d", *log.NumErrLogEntries), "")
	}
}

func populateATASelfTest(h *Health, log *struct {
	Standard struct {
		Table []ataSelfTestEntry `json:"table"`
	} `json:"standard"`
}) {
	if log == nil || len(log.Standard.Table) == 0 {
		return
	}
	e := log.Standard.Table[0]
	h.LastTest = &SelfTest{
		Type:            e.Type.String,
		Status:          e.Status.String,
		LifetimeHrs:     e.LifetimeHours,
		LBAOfFirstError: e.LBAOfFirstError,
	}
}

func populateNVMeSelfTest(h *Health, log *struct {
	Table []nvmeSelfTestEntry `json:"table"`
}) {
	if log == nil || len(log.Table) == 0 {
		return
	}
	e := log.Table[0]
	h.LastTest = &SelfTest{
		Type:            e.SelfTestCode.String,
		Status:          e.SelfTestResult.String,
		LifetimeHrs:     e.PowerOnHours,
		LBAOfFirstError: e.LBAOfFirstError,
	}
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
