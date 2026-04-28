package jobs

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func TestEncodeTask(t *testing.T) {
	id := uuid.New()
	payload, err := json.Marshal(PoolDestroyPayload{Name: "tank"})
	if err != nil {
		t.Fatal(err)
	}
	body, err := encodeTaskBody(id, payload)
	if err != nil {
		t.Fatal(err)
	}
	var got TaskBody
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.JobID != id.String() {
		t.Errorf("jobID=%q", got.JobID)
	}
	if string(got.Payload) != `{"name":"tank"}` {
		t.Errorf("payload=%q", got.Payload)
	}
}
