package middleware

import (
	"strings"
	"testing"
)

func TestRedactSecrets(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"password", `{"password":"hunter2"}`},
		{"secret", `{"name":"x","secret":"abc"}`},
		{"token", `{"token":"eyJhbGc"}`},
		{"passphrase", `{"passphrase":"open sesame"}`},
		{"case-insensitive", `{"Password":"hunter2"}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := string(RedactSecrets([]byte(c.in)))
			if !strings.Contains(got, "<redacted>") {
				t.Errorf("in=%q got=%q (no <redacted>)", c.in, got)
			}
			if strings.Contains(got, "hunter2") || strings.Contains(got, "abc") || strings.Contains(got, "eyJhbGc") || strings.Contains(got, "open sesame") {
				t.Errorf("secret leaked: %q", got)
			}
		})
	}
}

func TestRedactSecrets_PreservesNonSecrets(t *testing.T) {
	in := `{"name":"tank","health":"ONLINE"}`
	got := string(RedactSecrets([]byte(in)))
	if got != in {
		t.Errorf("non-secret body modified: %q", got)
	}
}
