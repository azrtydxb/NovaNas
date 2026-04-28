package snapshot

import "testing"

func TestBuildCreateArgs(t *testing.T) {
	args, err := buildCreateArgs("tank/home", "daily-1", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(args) != 2 || args[0] != "snapshot" || args[1] != "tank/home@daily-1" {
		t.Errorf("args=%v", args)
	}
}

func TestBuildCreateArgs_Recursive(t *testing.T) {
	args, _ := buildCreateArgs("tank", "daily-1", true)
	if len(args) != 3 || args[1] != "-r" {
		t.Errorf("args=%v", args)
	}
}

func TestBuildCreateArgs_BadDataset(t *testing.T) {
	if _, err := buildCreateArgs("", "x", false); err == nil {
		t.Error("expected error")
	}
}

func TestBuildCreateArgs_BadShortName(t *testing.T) {
	if _, err := buildCreateArgs("tank", "bad@name", false); err == nil {
		t.Error("expected error")
	}
}
