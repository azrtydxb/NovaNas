// Package wizard models the multi-step installer flow.
package wizard

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/azrtydxb/novanas/installer/internal/disks"
)

// Step is a single wizard page.
type Step interface {
	Init() tea.Cmd
	Update(msg tea.Msg) (Step, tea.Cmd)
	View() string
	IsComplete() bool
	Result() any
}

// State is the running collection of user choices. Every step reads prior
// state and writes its own slice.
type State struct {
	Language     string
	Keyboard     string
	Timezone     string
	Disks        []disks.Disk // chosen 1 or 2
	Mirror       bool
	Iface        string
	Hostname     string
	DHCP         bool
	StaticAddr   string // CIDR
	StaticGW     string
	StaticDNS    []string
	Confirmed    bool
	InstallDone  bool
}

// Languages is the v1 catalog. Only "en" is actually shipped; the rest are
// recorded for future localization.
var Languages = []struct{ Code, Label string }{
	{"en", "English"},
	{"fr", "Français"},
	{"de", "Deutsch"},
	{"es", "Español"},
	{"it", "Italiano"},
	{"nl", "Nederlands"},
	{"ja", "日本語"},
	{"zh", "中文 (简体)"},
}

// Keyboards maps language codes to typical layouts.
var Keyboards = map[string][]string{
	"en": {"us", "gb", "us-intl"},
	"fr": {"fr", "fr-oss", "be"},
	"de": {"de", "de-nodeadkeys", "ch"},
	"es": {"es", "latam"},
	"it": {"it"},
	"nl": {"us-intl", "nl"},
	"ja": {"jp"},
	"zh": {"cn", "us"},
}

// Timezones is a short, high-traffic IANA subset; the step supports filtering
// to keep it usable in a curses screen. A full /usr/share/zoneinfo scan is a
// TODO once we have the live ISO to inspect.
var Timezones = []string{
	"UTC",
	"Europe/London",
	"Europe/Paris",
	"Europe/Berlin",
	"Europe/Madrid",
	"Europe/Amsterdam",
	"Europe/Rome",
	"Europe/Stockholm",
	"Europe/Zurich",
	"America/New_York",
	"America/Chicago",
	"America/Denver",
	"America/Los_Angeles",
	"America/Toronto",
	"America/Mexico_City",
	"America/Sao_Paulo",
	"Asia/Tokyo",
	"Asia/Shanghai",
	"Asia/Hong_Kong",
	"Asia/Singapore",
	"Asia/Seoul",
	"Asia/Kolkata",
	"Asia/Dubai",
	"Australia/Sydney",
	"Australia/Melbourne",
	"Pacific/Auckland",
}
