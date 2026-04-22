package agent

import (
	"bytes"
	"testing"

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

// TestChunkServer_EncryptDecryptRoundtrip exercises maybeEncrypt /
// maybeDecrypt on a ChunkServer with a fake key provider, covering the
// three cases required by the A6 spec:
//  1. encrypted put + get for a volume with a DK
//  2. pass-through for a volume without a DK
//  3. pass-through when the server has no key provider at all
func TestChunkServer_EncryptDecryptRoundtrip(t *testing.T) {
	dk := bytes.Repeat([]byte{0x42}, crypto.ChunkKeySize)
	plaintext := []byte("the quick brown fox jumps over the lazy dog")

	t.Run("encrypted roundtrip", func(t *testing.T) {
		srv := &ChunkServer{plaintextHashes: map[string][]byte{}, authTags: map[string][crypto.AuthTagSize]byte{}}
		srv.WithVolumeKeys(fakeKeyProvider{volumeID: "vol-a", dk: dk})

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

		// Record chunk id -> plaintext hash so the decrypt helper can
		// find it (in production this comes from the metadata store).
		chunkID := "cid-1"
		srv.plaintextHashes[chunkID] = ph

		got, err := srv.maybeDecrypt("vol-a", chunkID, ct, tag)
		if err != nil {
			t.Fatalf("decrypt error: %v", err)
		}
		if !bytes.Equal(got, plaintext) {
			t.Fatalf("roundtrip mismatch: got %q want %q", got, plaintext)
		}
	})

	t.Run("unknown volume falls through unencrypted", func(t *testing.T) {
		srv := &ChunkServer{plaintextHashes: map[string][]byte{}, authTags: map[string][crypto.AuthTagSize]byte{}}
		srv.WithVolumeKeys(fakeKeyProvider{volumeID: "vol-a", dk: dk})

		ct, _, _, encrypted := srv.maybeEncrypt("vol-other", plaintext)
		if encrypted {
			t.Fatalf("expected encrypted=false for unknown volume")
		}
		if !bytes.Equal(ct, plaintext) {
			t.Fatalf("expected pass-through plaintext")
		}
	})

	t.Run("put-get simulation persists hash and tag", func(t *testing.T) {
		// Simulates the PutChunk write path by invoking maybeEncrypt,
		// storing the resulting (hash, tag) in the server's indices, and
		// then exercising the GetChunk read path via maybeDecrypt. This
		// is the closest we can get to an end-to-end roundtrip without a
		// real dataplane backend.
		srv := &ChunkServer{
			plaintextHashes: map[string][]byte{},
			authTags:        map[string][crypto.AuthTagSize]byte{},
		}
		srv.WithVolumeKeys(fakeKeyProvider{volumeID: "vol-x", dk: dk})

		for i, pt := range [][]byte{
			plaintext,
			bytes.Repeat([]byte{0x01}, 1024),
			[]byte(""), // empty — but ChunkServer rejects empty data at the RPC layer;
			// here we just drive the helpers to prove hash/tag bookkeeping is stable.
		} {
			if len(pt) == 0 {
				continue
			}
			cid := "chunk-" + string(rune('a'+i))
			ct, ph, tag, encrypted := srv.maybeEncrypt("vol-x", pt)
			if !encrypted {
				t.Fatalf("expected encrypted=true for chunk %d", i)
			}
			srv.plaintextHashes[cid] = ph
			srv.authTags[cid] = tag

			// Read back via the same code path GetChunk uses.
			storedTag := srv.authTags[cid]
			got, err := srv.maybeDecrypt("vol-x", cid, ct, storedTag)
			if err != nil {
				t.Fatalf("decrypt chunk %d: %v", i, err)
			}
			if !bytes.Equal(got, pt) {
				t.Fatalf("chunk %d roundtrip mismatch", i)
			}
		}
	})

	t.Run("nil key provider is a no-op", func(t *testing.T) {
		srv := &ChunkServer{plaintextHashes: map[string][]byte{}, authTags: map[string][crypto.AuthTagSize]byte{}}

		ct, _, tag, encrypted := srv.maybeEncrypt("any", plaintext)
		if encrypted {
			t.Fatalf("expected encrypted=false with nil keys")
		}
		if !bytes.Equal(ct, plaintext) {
			t.Fatalf("expected pass-through plaintext")
		}
		// Decrypt with nil keys is also a no-op.
		got, err := srv.maybeDecrypt("any", "cid", ct, tag)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !bytes.Equal(got, plaintext) {
			t.Fatalf("expected pass-through on decrypt, got %q", got)
		}
	})
}
