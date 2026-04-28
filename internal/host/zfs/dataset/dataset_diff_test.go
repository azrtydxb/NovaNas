package dataset

import (
	"reflect"
	"strings"
	"testing"
)

func TestBuildDiffArgs(t *testing.T) {
	cases := []struct {
		name     string
		from, to string
		want     []string
	}{
		{"snapshot-only", "tank/a@s1", "", []string{"diff", "-H", "tank/a@s1"}},
		{"snapshot-to-dataset", "tank/a@s1", "tank/a", []string{"diff", "-H", "tank/a@s1", "tank/a"}},
		{"snapshot-to-snapshot", "tank/a@s1", "tank/a@s2", []string{"diff", "-H", "tank/a@s1", "tank/a@s2"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := buildDiffArgs(c.from, c.to)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("got %v want %v", got, c.want)
			}
		})
	}
}

func TestBuildDiffArgs_RejectBadFrom(t *testing.T) {
	cases := []struct {
		name string
		from string
	}{
		{"missing-at", "tank/a"},
		{"empty", ""},
		{"leading-dash", "-evil@s"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := buildDiffArgs(c.from, ""); err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestBuildDiffArgs_RejectBadTo(t *testing.T) {
	if _, err := buildDiffArgs("tank/a@s", "bad name"); err == nil {
		t.Error("expected error for invalid to target")
	}
}

func TestParseDiff(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []DatasetDiffEntry
	}{
		{
			"added",
			"+\t/tank/a/file\n",
			[]DatasetDiffEntry{{Change: "+", Path: "/tank/a/file"}},
		},
		{
			"removed",
			"-\t/tank/a/gone\n",
			[]DatasetDiffEntry{{Change: "-", Path: "/tank/a/gone"}},
		},
		{
			"modified",
			"M\t/tank/a/touched\n",
			[]DatasetDiffEntry{{Change: "M", Path: "/tank/a/touched"}},
		},
		{
			"renamed",
			"R\t/tank/a/old\t/tank/a/new\n",
			[]DatasetDiffEntry{{Change: "R", Path: "/tank/a/old", NewPath: "/tank/a/new"}},
		},
		{
			"mixed",
			"+\t/tank/a/x\nM\t/tank/a/y\nR\t/tank/a/old\t/tank/a/new\n-\t/tank/a/z\n",
			[]DatasetDiffEntry{
				{Change: "+", Path: "/tank/a/x"},
				{Change: "M", Path: "/tank/a/y"},
				{Change: "R", Path: "/tank/a/old", NewPath: "/tank/a/new"},
				{Change: "-", Path: "/tank/a/z"},
			},
		},
		{
			"empty",
			"",
			nil,
		},
		{
			"blank-line-skipped",
			"+\t/a\n\n",
			[]DatasetDiffEntry{{Change: "+", Path: "/a"}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseDiff([]byte(c.in))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("got %v want %v", got, c.want)
			}
		})
	}
}

func TestParseDiff_Errors(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		wantSub string
	}{
		{"single-field", "lonely\n", "bad line"},
		{"unknown-code", "X\t/path\n", "unknown change code"},
		{"rename-missing-newpath", "R\t/old\n", "R expects 3 fields"},
		{"plain-with-extra-field", "+\t/a\textra\n", "+ expects 2 fields"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := parseDiff([]byte(c.in))
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), c.wantSub) {
				t.Errorf("error %q missing %q", err.Error(), c.wantSub)
			}
		})
	}
}
