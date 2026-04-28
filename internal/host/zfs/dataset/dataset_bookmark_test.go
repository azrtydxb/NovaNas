package dataset

import (
	"reflect"
	"testing"
)

func TestValidateBookmarkName_Positive(t *testing.T) {
	cases := []string{
		"tank#bm1",
		"tank/home#daily-2024-01-01",
		"tank/a/b#snap_v1",
		"tank/a#a",
	}
	for _, s := range cases {
		t.Run(s, func(t *testing.T) {
			if err := validateBookmarkName(s); err != nil {
				t.Errorf("unexpected error for %q: %v", s, err)
			}
		})
	}
}

func TestValidateBookmarkName_Negative(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"no-hash", "tank/home"},
		{"empty", ""},
		{"leading-dash", "-tank#bm"},
		{"two-hashes", "tank#a#b"},
		{"empty-short", "tank#"},
		{"empty-dataset", "#bm"},
		{"bad-char-in-short", "tank#bad name"},
		{"dot-in-short", "tank#bad.name"},
		{"bad-dataset", "bad@ds#bm"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := validateBookmarkName(c.in); err == nil {
				t.Errorf("expected error for %q", c.in)
			}
		})
	}
}

func TestBuildBookmarkArgs(t *testing.T) {
	got, err := buildBookmarkArgs("tank/a@snap1", "tank/a#bm1")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"bookmark", "tank/a@snap1", "tank/a#bm1"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestBuildBookmarkArgs_RejectBadSnapshot(t *testing.T) {
	if _, err := buildBookmarkArgs("tank/a", "tank/a#bm"); err == nil {
		t.Error("expected error: missing @ in snapshot")
	}
}

func TestBuildBookmarkArgs_RejectBadBookmark(t *testing.T) {
	if _, err := buildBookmarkArgs("tank/a@s", "tank/a"); err == nil {
		t.Error("expected error: missing # in bookmark")
	}
}

func TestBuildListBookmarksArgs(t *testing.T) {
	cases := []struct {
		name string
		root string
		want []string
	}{
		{"no-root", "", []string{"list", "-H", "-p", "-t", "bookmark", "-o", "name,creation"}},
		{"root", "tank/home", []string{"list", "-H", "-p", "-t", "bookmark", "-o", "name,creation", "-r", "tank/home"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := buildListBookmarksArgs(c.root)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("got %v want %v", got, c.want)
			}
		})
	}
}

func TestBuildListBookmarksArgs_RejectBadRoot(t *testing.T) {
	if _, err := buildListBookmarksArgs("bad@root"); err == nil {
		t.Error("expected error")
	}
}

func TestBuildDestroyBookmarkArgs(t *testing.T) {
	got, err := buildDestroyBookmarkArgs("tank/a#bm1")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"destroy", "tank/a#bm1"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestBuildDestroyBookmarkArgs_RejectBad(t *testing.T) {
	if _, err := buildDestroyBookmarkArgs("tank/a"); err == nil {
		t.Error("expected error: missing #")
	}
}

func TestParseBookmarkList(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []Bookmark
	}{
		{
			"single",
			"tank/home#bm1\t1700000000\n",
			[]Bookmark{{Name: "tank/home#bm1", CreationUnix: 1700000000}},
		},
		{
			"multiple",
			"tank/a#bm1\t1700000000\ntank/b#bm2\t1700000123\n",
			[]Bookmark{
				{Name: "tank/a#bm1", CreationUnix: 1700000000},
				{Name: "tank/b#bm2", CreationUnix: 1700000123},
			},
		},
		{"empty", "", nil},
		{"blank-skipped", "\ntank/a#bm\t1\n", []Bookmark{{Name: "tank/a#bm", CreationUnix: 1}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseBookmarkList([]byte(c.in))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("got %v want %v", got, c.want)
			}
		})
	}
}

func TestParseBookmarkList_Errors(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"missing-creation", "tank#bm\n"},
		{"too-many-fields", "tank#bm\t1\t2\n"},
		{"bad-creation", "tank#bm\tnotanumber\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := parseBookmarkList([]byte(c.in)); err == nil {
				t.Error("expected error")
			}
		})
	}
}
