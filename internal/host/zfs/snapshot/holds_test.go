package snapshot

import (
	"reflect"
	"testing"
)

func TestBuildHoldArgs(t *testing.T) {
	cases := []struct {
		name      string
		snap, tag string
		recursive bool
		want      []string
	}{
		{"plain", "tank/a@s1", "keep", false, []string{"hold", "keep", "tank/a@s1"}},
		{"recursive", "tank/a@s1", "keep", true, []string{"hold", "-r", "keep", "tank/a@s1"}},
		{"tag-with-dot", "tank/a@s1", "v1.2.3", false, []string{"hold", "v1.2.3", "tank/a@s1"}},
		{"tag-with-dash-underscore", "tank/a@s1", "my_tag-1", false, []string{"hold", "my_tag-1", "tank/a@s1"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := buildHoldArgs(c.snap, c.tag, c.recursive)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("got %v want %v", got, c.want)
			}
		})
	}
}

func TestBuildHoldArgs_Reject(t *testing.T) {
	cases := []struct {
		name      string
		snap, tag string
	}{
		{"bad-snap", "tank/a", "tag"},
		{"empty-tag", "tank/a@s", ""},
		{"leading-dash-tag", "tank/a@s", "-evil"},
		{"bad-tag-char", "tank/a@s", "bad tag"},
		{"bad-tag-slash", "tank/a@s", "a/b"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := buildHoldArgs(c.snap, c.tag, false); err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestBuildReleaseArgs(t *testing.T) {
	cases := []struct {
		name      string
		snap, tag string
		recursive bool
		want      []string
	}{
		{"plain", "tank/a@s1", "keep", false, []string{"release", "keep", "tank/a@s1"}},
		{"recursive", "tank/a@s1", "keep", true, []string{"release", "-r", "keep", "tank/a@s1"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := buildReleaseArgs(c.snap, c.tag, c.recursive)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("got %v want %v", got, c.want)
			}
		})
	}
}

func TestBuildReleaseArgs_Reject(t *testing.T) {
	if _, err := buildReleaseArgs("tank/a@s", "-bad", false); err == nil {
		t.Error("expected error for leading-dash tag")
	}
	if _, err := buildReleaseArgs("notasnap", "tag", false); err == nil {
		t.Error("expected error for bad snapshot")
	}
}

func TestBuildHoldsArgs(t *testing.T) {
	got, err := buildHoldsArgs("tank/a@s1")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"holds", "-H", "-p", "tank/a@s1"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestBuildHoldsArgs_Reject(t *testing.T) {
	if _, err := buildHoldsArgs("tank/a"); err == nil {
		t.Error("expected error: missing @")
	}
}

func TestParseHolds(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []Hold
	}{
		{
			"single",
			"tank/a@s1\tkeep\t1700000000\n",
			[]Hold{{Snapshot: "tank/a@s1", Tag: "keep", CreationUnix: 1700000000}},
		},
		{
			"multiple",
			"tank/a@s1\tkeep\t1700000000\ntank/a@s1\tother\t1700000100\n",
			[]Hold{
				{Snapshot: "tank/a@s1", Tag: "keep", CreationUnix: 1700000000},
				{Snapshot: "tank/a@s1", Tag: "other", CreationUnix: 1700000100},
			},
		},
		{"empty", "", nil},
		{"blank-skipped", "\ntank/a@s\ttag\t1\n", []Hold{{Snapshot: "tank/a@s", Tag: "tag", CreationUnix: 1}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseHolds([]byte(c.in))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("got %v want %v", got, c.want)
			}
		})
	}
}

func TestParseHolds_Errors(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"too-few", "tank/a@s\ttag\n"},
		{"too-many", "tank/a@s\ttag\t1\textra\n"},
		{"bad-timestamp", "tank/a@s\ttag\tnotanumber\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := parseHolds([]byte(c.in)); err == nil {
				t.Error("expected error")
			}
		})
	}
}
