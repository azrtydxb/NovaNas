package jobs

import (
	"encoding/json"
	"testing"
)

func TestPoolCreatePayload_Roundtrip(t *testing.T) {
	in := PoolCreatePayload{Name: "tank"}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out PoolCreatePayload
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if out.Name != "tank" {
		t.Errorf("got %+v", out)
	}
}

func TestKindString(t *testing.T) {
	if KindPoolCreate != "pool.create" {
		t.Errorf("KindPoolCreate=%q", KindPoolCreate)
	}
}
