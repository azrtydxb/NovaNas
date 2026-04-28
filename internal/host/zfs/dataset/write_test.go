package dataset

import "testing"

func TestBuildCreateArgs_Filesystem(t *testing.T) {
	spec := CreateSpec{
		Parent: "tank",
		Name:   "home",
		Type:   "filesystem",
		Properties: map[string]string{
			"compression": "lz4",
			"recordsize":  "128K",
		},
	}
	args, err := buildCreateArgs(spec)
	if err != nil {
		t.Fatal(err)
	}
	// "create -o compression=lz4 -o recordsize=128K tank/home"
	if args[0] != "create" {
		t.Errorf("args[0]=%q", args[0])
	}
	if args[len(args)-1] != "tank/home" {
		t.Errorf("last arg=%q", args[len(args)-1])
	}
}

func TestBuildCreateArgs_Volume(t *testing.T) {
	spec := CreateSpec{
		Parent: "tank",
		Name:   "vol1",
		Type:   "volume",
		VolumeSizeBytes: 1 << 30,
		Properties: map[string]string{"compression": "off"},
	}
	args, err := buildCreateArgs(spec)
	if err != nil {
		t.Fatal(err)
	}
	// expect "-V <size>" present
	saw := false
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "-V" && args[i+1] == "1073741824" {
			saw = true
		}
	}
	if !saw {
		t.Errorf("missing -V; args=%v", args)
	}
}

func TestBuildCreateArgs_RejectBadName(t *testing.T) {
	spec := CreateSpec{Parent: "tank", Name: "bad@name", Type: "filesystem"}
	if _, err := buildCreateArgs(spec); err == nil {
		t.Error("expected error")
	}
}
