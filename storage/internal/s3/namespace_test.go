package s3

import (
	"context"
	"testing"
)

// fakeNSChunkStore implements NamespacedChunkStore with a per-namespace map
// so tests can assert SSE-C writes went into the "ssec" partition.
type fakeNSChunkStore struct {
	// ns -> id -> data
	byNS map[string]map[string][]byte
	// counter for deterministic IDs
	n int
}

func newFakeNSChunkStore() *fakeNSChunkStore {
	return &fakeNSChunkStore{byNS: map[string]map[string][]byte{}}
}

func (f *fakeNSChunkStore) put(ns string, data []byte) string {
	f.n++
	if f.byNS[ns] == nil {
		f.byNS[ns] = map[string][]byte{}
	}
	id := ns + "-" + string(rune('a'+f.n))
	f.byNS[ns][id] = append([]byte(nil), data...)
	return id
}

func (f *fakeNSChunkStore) PutChunkData(_ context.Context, data []byte) (string, error) {
	return f.put("default", data), nil
}
func (f *fakeNSChunkStore) GetChunkData(_ context.Context, id string) ([]byte, error) {
	if d, ok := f.byNS["default"][id]; ok {
		return d, nil
	}
	return nil, context.Canceled
}
func (f *fakeNSChunkStore) DeleteChunkData(_ context.Context, _ string) error { return nil }
func (f *fakeNSChunkStore) PutChunkDataNS(_ context.Context, ns string, data []byte) (string, error) {
	return f.put(ns, data), nil
}
func (f *fakeNSChunkStore) GetChunkDataNS(_ context.Context, ns, id string) ([]byte, error) {
	if d, ok := f.byNS[ns][id]; ok {
		return d, nil
	}
	return nil, context.Canceled
}

func TestPutChunkInNamespace_DefaultGoesToDedupNamespace(t *testing.T) {
	cs := newFakeNSChunkStore()
	id, err := putChunkInNamespace(context.Background(), cs, "default", []byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cs.byNS["default"][id]; !ok {
		t.Fatalf("default put did not land in default namespace: %+v", cs.byNS)
	}
	if len(cs.byNS["ssec"]) != 0 {
		t.Fatalf("default put leaked into ssec namespace: %+v", cs.byNS["ssec"])
	}
}

func TestPutChunkInNamespace_SSECGoesToSegregatedNamespace(t *testing.T) {
	cs := newFakeNSChunkStore()
	id, err := putChunkInNamespace(context.Background(), cs, "ssec", []byte("ct-a"))
	if err != nil {
		t.Fatal(err)
	}
	// Returned chunk ID must carry the namespace prefix so later reads
	// route correctly.
	if got := id[:5]; got != "ssec:" {
		t.Fatalf("SSE-C chunk ID should be prefixed with ssec:, got %q", id)
	}
	// Underlying store must see it in the ssec partition, not default.
	rawID := id[5:]
	if _, ok := cs.byNS["ssec"][rawID]; !ok {
		t.Fatalf("ssec put did not land in ssec namespace: %+v", cs.byNS)
	}
	if len(cs.byNS["default"]) != 0 {
		t.Fatalf("ssec put leaked into default namespace: %+v", cs.byNS["default"])
	}
}

func TestGetChunkFromNamespace_RoundTripsBothNamespaces(t *testing.T) {
	cs := newFakeNSChunkStore()
	ctx := context.Background()

	defID, _ := putChunkInNamespace(ctx, cs, "default", []byte("plain"))
	ssecID, _ := putChunkInNamespace(ctx, cs, "ssec", []byte("cipher"))

	gotDef, err := getChunkFromNamespace(ctx, cs, defID)
	if err != nil || string(gotDef) != "plain" {
		t.Fatalf("default read: got %q err %v", string(gotDef), err)
	}
	gotSSEC, err := getChunkFromNamespace(ctx, cs, ssecID)
	if err != nil || string(gotSSEC) != "cipher" {
		t.Fatalf("ssec read: got %q err %v", string(gotSSEC), err)
	}
}

func TestPutChunkInNamespace_FallbackStoreGetsPrefix(t *testing.T) {
	// memChunkStore (defined in bucket_test.go) is not namespaced.
	// Verify putChunkInNamespace still returns a prefixed ID so reads
	// can route correctly even on backends that ignore the namespace.
	cs := newMemChunkStore()
	id, err := putChunkInNamespace(context.Background(), cs, "ssec", []byte("x"))
	if err != nil {
		t.Fatal(err)
	}
	if id[:5] != "ssec:" {
		t.Fatalf("fallback should still prefix ssec:, got %q", id)
	}
	// And round-trip.
	got, err := getChunkFromNamespace(context.Background(), cs, id)
	if err != nil || string(got) != "x" {
		t.Fatalf("fallback round-trip: got %q err %v", string(got), err)
	}
}
