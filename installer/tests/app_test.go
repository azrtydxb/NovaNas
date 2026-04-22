package tests

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/azrtydxb/novanas/installer/internal/app"
	"github.com/azrtydxb/novanas/installer/internal/disks"
)

// fakeScanner always returns one fake disk so the DisksStep has something
// selectable without hitting real lsblk.
func fakeScanner() *disks.Scanner {
	return &disks.Scanner{
		Exec: func(name string, args ...string) ([]byte, error) {
			return []byte(lsblkFixture), nil
		},
	}
}

func TestAppBuildsAndRendersFirstStep(t *testing.T) {
	m := app.New(app.Options{
		BundlePath:  "/tmp/fake.raucb",
		SkipNetwork: true,
		DryRun:      true,
		DiskScanner: fakeScanner(),
	})
	v := m.View()
	if v == "" {
		t.Fatal("empty view")
	}
	// First step is language; should show English.
	if !contains(v, "English") {
		t.Errorf("language step should include 'English'; view:\n%s", v)
	}
}

func TestAppAdvancesThroughLanguage(t *testing.T) {
	m := app.New(app.Options{
		BundlePath:  "/tmp/fake.raucb",
		SkipNetwork: true,
		DryRun:      true,
		DiskScanner: fakeScanner(),
	})

	// Pick language (English is idx 0 — Enter).
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	// Pick first keyboard layout.
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	st := m.State()
	if st.Language != "en" {
		t.Errorf("expected language=en, got %q", st.Language)
	}
	if st.Keyboard == "" {
		t.Errorf("expected keyboard to be set")
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
