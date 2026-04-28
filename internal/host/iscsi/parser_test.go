package iscsi

import (
	"reflect"
	"testing"
)

func TestParseTargetList_Basic(t *testing.T) {
	out := []byte(`o- iscsi ............................................. [Targets: 2]
  o- iqn.2020-01.io.example:foo .................. [TPGs: 1]
  o- iqn.2020-01.io.example:bar .................. [TPGs: 1]
`)
	got, err := parseTargetList(out)
	if err != nil {
		t.Fatal(err)
	}
	want := []Target{
		{IQN: "iqn.2020-01.io.example:foo"},
		{IQN: "iqn.2020-01.io.example:bar"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got=%v want=%v", got, want)
	}
}

func TestParseTargetList_Empty(t *testing.T) {
	out := []byte(`o- iscsi ............................................. [Targets: 0]
`)
	got, err := parseTargetList(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func TestParseTargetDetail_Full(t *testing.T) {
	out := []byte(`o- iqn.2020-01.io.example:foo ................. [TPGs: 1]
  o- tpg1 ......................................... [...]
    o- acls ..................................... [ACLs: 1]
    | o- iqn.2020-01.io.client:bar ............ [Mapped LUNs: 1]
    o- luns ..................................... [LUNs: 1]
    | o- lun0 ............... [block/zvol1 (/dev/zvol/tank/vol1)]
    o- portals .................................. [Portals: 2]
      o- 10.0.0.1:3260 ........................... [OK]
      o- 10.0.0.2:3260 ........................... [OK]
`)
	d, err := parseTargetDetail(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.ACLs) != 1 || d.ACLs[0].InitiatorIQN != "iqn.2020-01.io.client:bar" {
		t.Errorf("acls=%v", d.ACLs)
	}
	if len(d.LUNs) != 1 || d.LUNs[0].ID != 0 ||
		d.LUNs[0].Backstore != "zvol1" || d.LUNs[0].Zvol != "/dev/zvol/tank/vol1" {
		t.Errorf("luns=%+v", d.LUNs)
	}
	if len(d.Portals) != 2 {
		t.Fatalf("expected 2 portals, got %v", d.Portals)
	}
	if d.Portals[0] != (Portal{IP: "10.0.0.1", Port: 3260, Transport: "tcp"}) {
		t.Errorf("portals[0]=%v", d.Portals[0])
	}
}

func TestParseTargetDetail_NoACLs(t *testing.T) {
	out := []byte(`o- iqn.2020-01.io.example:foo ................. [TPGs: 1]
  o- tpg1 ......................................... [...]
    o- acls ..................................... [ACLs: 0]
    o- luns ..................................... [LUNs: 0]
    o- portals .................................. [Portals: 1]
      o- 10.0.0.1:3260 ........................... [OK]
`)
	d, err := parseTargetDetail(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.ACLs) != 0 || len(d.LUNs) != 0 {
		t.Errorf("expected empty acls/luns, got acls=%v luns=%v", d.ACLs, d.LUNs)
	}
	if len(d.Portals) != 1 {
		t.Errorf("expected 1 portal, got %v", d.Portals)
	}
}

func TestParseLUNLine_BackstoreOnly(t *testing.T) {
	// Sometimes the bracketed annotation is just `[block/<name>]` without
	// the dev path.
	lun, ok := parseLUNLine("lun3", "    | o- lun3 ............... [block/myblock]")
	if !ok {
		t.Fatal("expected ok")
	}
	if lun.ID != 3 || lun.Backstore != "myblock" {
		t.Errorf("got %+v", lun)
	}
}

func TestParsePortalName_IPv6(t *testing.T) {
	p, ok := parsePortalName("[::1]:3260")
	if !ok {
		t.Fatal("expected ok")
	}
	if p.IP != "::1" || p.Port != 3260 {
		t.Errorf("got %+v", p)
	}
}

func TestParsePortalName_BadFormat(t *testing.T) {
	for _, name := range []string{"10.0.0.1", "no-port:", ":3260"} {
		if _, ok := parsePortalName(name); ok {
			t.Errorf("expected !ok for %q", name)
		}
	}
}

func TestParseTreeLine_Skips(t *testing.T) {
	for _, line := range []string{
		"",
		"some banner",
		"   ",
	} {
		if _, _, ok := parseTreeLine(line); ok {
			t.Errorf("expected !ok for %q", line)
		}
	}
}
