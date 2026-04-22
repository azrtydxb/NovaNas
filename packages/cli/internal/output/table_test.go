package output

import (
	"bytes"
	"strings"
	"testing"
)

func TestTable(t *testing.T) {
	var buf bytes.Buffer
	Table(&buf, []string{"NAME", "STATUS"}, [][]string{
		{"pool-a", "Ready"},
		{"pool-b", "Degraded"},
	})
	out := buf.String()
	for _, want := range []string{"NAME", "STATUS", "pool-a", "Degraded"} {
		if !strings.Contains(out, want) {
			t.Fatalf("table output missing %q:\n%s", want, out)
		}
	}
}
