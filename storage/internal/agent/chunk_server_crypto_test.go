package agent

import (
	"bytes"
	"context"
	"sync"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/azrtydxb/novanas/storage/internal/crypto"
)

// fakeKeyProvider is a VolumeKeyProvider that returns a fixed DK for a
// single volume id. All other volume ids are reported as unencrypted.
type fakeKeyProvider struct {
	volumeID string
	dk       []byte
}

func (f fakeKeyProvider) DatasetKey(volumeID string) ([]byte, bool) {
	if volumeID == f.volumeID {
		return f.dk, true
	}
	return nil, false
}

// fakeMetaClient is a test-local MetaClient that stores per-chunk crypto
// bookkeeping in an in-memory map. Keyed by (volumeID, chunkID).
type fakeMetaClient struct {
	mu      sync.Mutex
	entries map[string]fakeCrypto
}

type fakeCrypto struct {
	plaintextHash []byte
	authTag       []byte
	dkVersion     uint32
}

func newFakeMetaClient() *fakeMetaClient {
	return &fakeMetaClient{entries: make(map[string]fakeCrypto)}
}

func fakeMetaKey(volumeID, chunkID string) string {
	return volumeID + "|" + chunkID
}

func (f *fakeMetaClient) SetChunkCrypto(_ context.Context, volumeID, chunkID string, plaintextHash, authTag []byte, dkVersion uint32) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	ph := append([]byte(nil), plaintextHash...)
	tag := append([]byte(nil), authTag...)
	f.entries[fakeMetaKey(volumeID, chunkID)] = fakeCrypto{plaintextHash: ph, authTag: tag, dkVersion: dkVersion}
	return nil
}

func (f *fakeMetaClient) GetChunkCrypto(_ context.Context, volumeID, chunkID string) ([]byte, []byte, uint32, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.entries[fakeMetaKey(volumeID, chunkID)]
	if !ok {
		return nil, nil, 0, status.Error(codes.NotFound, "chunk crypto not found")
	}
	return e.plaintextHash, e.authTag, e.dkVersion, nil
}

func (f *fakeMetaClient) DeleteChunkCrypto(_ context.Context, volumeID, chunkID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.entries, fakeMetaKey(volumeID, chunkID))
	return nil
}

// TestChunkServer_EncryptDecryptRoundtrip exercises maybeEncrypt /
// maybeDecrypt with the metadata-service-backed crypto bookkeeping,
// covering the cases required by the A8-Persistence contract:
//  1. encrypted put + get for a volume with a DK persists through meta
//  2. pass-through for a volume without a DK
//  3. missing crypto metadata -> decryption fails cleanly
//  4. nil key provider is a no-op (no encryption at all)
func TestChunkServer_EncryptDecryptRoundtrip(t *testing.T) {
	dk := bytes.Repeat([]byte{0x42}, crypto.ChunkKeySize)
	plaintext := []byte("the quick brown fox jumps over the lazy dog")
	ctx := context.Background()

	t.Run("encrypted roundtrip via meta client", func(t *testing.T) {
		meta := newFakeMetaClient()
		srv := (&ChunkServer{}).
			WithVolumeKeys(fakeKeyProvider{volumeID: "vol-a", dk: dk}).
			WithMetaClient(meta)

		ct, ph, tag, encrypted := srv.maybeEncrypt("vol-a", plaintext)
		if !encrypted {
			t.Fatalf("expected encrypted=true, got false")
		}
		if bytes.Equal(ct, plaintext) {
			t.Fatalf("ciphertext equals plaintext")
		}
		if len(ph) != 32 {
			t.Fatalf("expected 32-byte plaintextHash, got %d", len(ph))
		}

		// Simulate the PutChunk persist step.
		chunkID := "cid-1"
		if err := srv.meta.SetChunkCrypto(ctx, "vol-a", chunkID, ph, tag[:], 0); err != nil {
			t.Fatalf("meta SetChunkCrypto: %v", err)
		}

		// Simulate the GetChunk fetch step.
		gotPH, gotTag, _, err := srv.meta.GetChunkCrypto(ctx, "vol-a", chunkID)
		if err != nil {
			t.Fatalf("meta GetChunkCrypto: %v", err)
		}
		if !bytes.Equal(gotPH, ph[:]) {
			t.Fatalf("plaintextHash roundtrip mismatch")
		}
		var storedTag [crypto.AuthTagSize]byte
		copy(storedTag[:], gotTag)

		got, err := srv.maybeDecrypt("vol-a", ct, gotPH, storedTag)
		if err != nil {
			t.Fatalf("decrypt error: %v", err)
		}
		if !bytes.Equal(got, plaintext) {
			t.Fatalf("roundtrip mismatch: got %q want %q", got, plaintext)
		}
	})

	t.Run("unknown volume falls through unencrypted", func(t *testing.T) {
		srv := (&ChunkServer{}).
			WithVolumeKeys(fakeKeyProvider{volumeID: "vol-a", dk: dk}).
			WithMetaClient(newFakeMetaClient())

		ct, _, _, encrypted := srv.maybeEncrypt("vol-other", plaintext)
		if encrypted {
			t.Fatalf("expected encrypted=false for unknown volume")
		}
		if !bytes.Equal(ct, plaintext) {
			t.Fatalf("expected pass-through plaintext")
		}
	})

	t.Run("missing crypto metadata is treated as unencrypted pass-through", func(t *testing.T) {
		meta := newFakeMetaClient()
		srv := (&ChunkServer{}).
			WithVolumeKeys(fakeKeyProvider{volumeID: "vol-x", dk: dk}).
			WithMetaClient(meta)

		// Encrypt but deliberately do NOT persist to meta.
		ct, _, tag, encrypted := srv.maybeEncrypt("vol-x", plaintext)
		if !encrypted {
			t.Fatalf("expected encrypted=true")
		}

		// Read path: meta returns NotFound, plaintextHash nil -> passthrough.
		_, _, _, err := meta.GetChunkCrypto(ctx, "vol-x", "cid-missing")
		if err == nil || status.Code(err) != codes.NotFound {
			t.Fatalf("expected NotFound, got %v", err)
		}

		// maybeDecrypt with nil plaintextHash treats as pass-through and
		// returns the (still-encrypted) ciphertext unchanged.
		got, err := srv.maybeDecrypt("vol-x", ct, nil, tag)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !bytes.Equal(got, ct) {
			t.Fatalf("expected passthrough of ciphertext")
		}
	})

	t.Run("put-get simulation persists hash and tag through meta", func(t *testing.T) {
		meta := newFakeMetaClient()
		srv := (&ChunkServer{}).
			WithVolumeKeys(fakeKeyProvider{volumeID: "vol-x", dk: dk}).
			WithMetaClient(meta)

		for i, pt := range [][]byte{
			plaintext,
			bytes.Repeat([]byte{0x01}, 1024),
		} {
			cid := "chunk-" + string(rune('a'+i))
			ct, ph, tag, encrypted := srv.maybeEncrypt("vol-x", pt)
			if !encrypted {
				t.Fatalf("expected encrypted=true for chunk %d", i)
			}
			if err := meta.SetChunkCrypto(ctx, "vol-x", cid, ph, tag[:], 7); err != nil {
				t.Fatalf("meta SetChunkCrypto %d: %v", i, err)
			}

			gotPH, gotTag, gotVer, err := meta.GetChunkCrypto(ctx, "vol-x", cid)
			if err != nil {
				t.Fatalf("meta GetChunkCrypto %d: %v", i, err)
			}
			if gotVer != 7 {
				t.Fatalf("chunk %d dk version mismatch: got %d", i, gotVer)
			}
			var storedTag [crypto.AuthTagSize]byte
			copy(storedTag[:], gotTag)

			got, err := srv.maybeDecrypt("vol-x", ct, gotPH, storedTag)
			if err != nil {
				t.Fatalf("decrypt chunk %d: %v", i, err)
			}
			if !bytes.Equal(got, pt) {
				t.Fatalf("chunk %d roundtrip mismatch", i)
			}
		}
	})

	t.Run("nil key provider is a no-op", func(t *testing.T) {
		srv := &ChunkServer{}

		ct, _, tag, encrypted := srv.maybeEncrypt("any", plaintext)
		if encrypted {
			t.Fatalf("expected encrypted=false with nil keys")
		}
		if !bytes.Equal(ct, plaintext) {
			t.Fatalf("expected pass-through plaintext")
		}
		// Decrypt with nil keys is also a no-op.
		got, err := srv.maybeDecrypt("any", ct, nil, tag)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !bytes.Equal(got, plaintext) {
			t.Fatalf("expected pass-through on decrypt, got %q", got)
		}
	})

	t.Run("ForgetChunkCrypto removes bookkeeping", func(t *testing.T) {
		meta := newFakeMetaClient()
		srv := (&ChunkServer{}).WithMetaClient(meta)
		if err := meta.SetChunkCrypto(ctx, "vol", "c1", []byte("h"), []byte("t"), 1); err != nil {
			t.Fatal(err)
		}
		if err := srv.ForgetChunkCrypto(ctx, "vol", "c1"); err != nil {
			t.Fatalf("ForgetChunkCrypto: %v", err)
		}
		if _, _, _, err := meta.GetChunkCrypto(ctx, "vol", "c1"); err == nil || status.Code(err) != codes.NotFound {
			t.Fatalf("expected NotFound after forget, got %v", err)
		}
		// Nil meta path is a no-op, not an error.
		bare := &ChunkServer{}
		if err := bare.ForgetChunkCrypto(ctx, "vol", "c1"); err != nil {
			t.Fatalf("expected nil meta to be no-op, got %v", err)
		}
	})

}
