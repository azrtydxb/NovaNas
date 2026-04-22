package chunk

import (
	"bytes"
	"crypto/rand"
	"testing"

	"github.com/azrtydxb/novanas/storage/internal/crypto"
)

func randDK(t *testing.T) []byte {
	t.Helper()
	k := make([]byte, crypto.ChunkKeySize)
	if _, err := rand.Read(k); err != nil {
		t.Fatal(err)
	}
	return k
}

func TestSplitDataEncrypted_NilDKFallsBack(t *testing.T) {
	data := []byte("hello")
	got, err := SplitDataEncrypted(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Encrypted {
		t.Error("nil dk should fall back to unencrypted SplitData")
	}
}

func TestSplitDataEncrypted_Convergence(t *testing.T) {
	dk := randDK(t)
	data := make([]byte, ChunkSize+1024)
	for i := range data {
		data[i] = byte(i)
	}
	a, err := SplitDataEncrypted(data, dk)
	if err != nil {
		t.Fatal(err)
	}
	b, err := SplitDataEncrypted(data, dk)
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != len(b) || len(a) != 2 {
		t.Fatalf("chunk count: a=%d b=%d", len(a), len(b))
	}
	for i := range a {
		if a[i].ID != b[i].ID {
			t.Errorf("chunk %d: ids differ across calls (convergence broken)", i)
		}
		if !a[i].Encrypted {
			t.Error("expected Encrypted=true")
		}
	}
}

func TestSplitDataEncrypted_RoundTrip(t *testing.T) {
	dk := randDK(t)
	data := make([]byte, 3*ChunkSize/2)
	for i := range data {
		data[i] = byte(i * 7)
	}
	chunks, err := SplitDataEncrypted(data, dk)
	if err != nil {
		t.Fatal(err)
	}
	var reassembled []byte
	for _, c := range chunks {
		pt, err := DecryptChunkData(c, dk)
		if err != nil {
			t.Fatalf("decrypt: %v", err)
		}
		reassembled = append(reassembled, pt...)
	}
	if !bytes.Equal(reassembled, data) {
		t.Error("roundtrip mismatch")
	}
}

func TestSplitDataEncrypted_DifferentDKBreaksDedup(t *testing.T) {
	data := []byte("same plaintext")
	a, err := SplitDataEncrypted(data, randDK(t))
	if err != nil {
		t.Fatal(err)
	}
	b, err := SplitDataEncrypted(data, randDK(t))
	if err != nil {
		t.Fatal(err)
	}
	if a[0].ID == b[0].ID {
		t.Error("different DKs must not produce the same chunk id")
	}
}

func TestOpenChunk_SealEncryptedRoundtrip(t *testing.T) {
	dk := randDK(t)
	o, err := NewOpenChunk("pool", 128)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("wal entries go here")
	if err := o.Append(0, payload); err != nil {
		t.Fatal(err)
	}
	sealed, err := o.SealEncrypted(dk)
	if err != nil {
		t.Fatal(err)
	}
	if !sealed.Encrypted {
		t.Error("expected Encrypted=true")
	}
	// Ciphertext must not equal plaintext.
	if bytes.Equal(sealed.Data, payload) {
		t.Error("ciphertext should not equal plaintext")
	}
	got, err := DecryptChunkData(sealed, dk)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Error("roundtrip mismatch")
	}
}

func TestOpenChunkRegistry_SealEncrypted(t *testing.T) {
	dk := randDK(t)
	reg := NewOpenChunkRegistry()
	c, err := reg.Open("p", 128)
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.Append(c.ID(), 0, []byte("abc")); err != nil {
		t.Fatal(err)
	}
	sealed, err := reg.SealEncrypted(c.ID(), dk)
	if err != nil {
		t.Fatal(err)
	}
	if !sealed.Encrypted {
		t.Error("expected encrypted seal")
	}
	// Registry should have evicted it.
	if _, err := reg.Get(c.ID()); err == nil {
		t.Error("expected NotFound after seal")
	}
}
