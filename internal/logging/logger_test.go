package logging

import (
	"bytes"
	"strings"
	"testing"
)

func TestNew_RespectsLevel(t *testing.T) {
	var buf bytes.Buffer
	logger, err := New("warn", &buf)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	logger.Info("info-msg")
	logger.Warn("warn-msg")
	out := buf.String()
	if strings.Contains(out, "info-msg") {
		t.Errorf("info should be suppressed; got: %s", out)
	}
	if !strings.Contains(out, "warn-msg") {
		t.Errorf("warn missing; got: %s", out)
	}
}

func TestNew_RejectsUnknownLevel(t *testing.T) {
	if _, err := New("nope", nil); err == nil {
		t.Fatal("expected error for unknown level")
	}
}
