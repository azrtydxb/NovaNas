package crypto

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"testing"
)

func mustKey(t *testing.T) []byte {
	t.Helper()
	k := make([]byte, ChunkKeySize)
	if _, err := rand.Read(k); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return k
}

// TestConvergence: same plaintext + same DK -> identical ciphertext +
// identical chunk id. This is the core dedup property.
func TestConvergence(t *testing.T) {
	dk := mustKey(t)
	plaintext := []byte("convergent encryption test plaintext")

	a, err := EncryptChunk(dk, plaintext)
	if err != nil {
		t.Fatalf("encrypt a: %v", err)
	}
	b, err := EncryptChunk(dk, plaintext)
	if err != nil {
		t.Fatalf("encrypt b: %v", err)
	}

	if !bytes.Equal(a.Ciphertext, b.Ciphertext) {
		t.Error("same (dk, plaintext) should yield identical ciphertext")
	}
	if a.ChunkID != b.ChunkID {
		t.Error("same (dk, plaintext) should yield identical chunk id")
	}
	if a.PlaintextHash != b.PlaintextHash {
		t.Error("plaintext hash should be deterministic")
	}
}

// TestDedupBreakageAcrossDK: different DKs with same plaintext must
// produce different ciphertext / chunk ids — i.e. no cross-volume
// dedup leakage.
func TestDedupBreakageAcrossDK(t *testing.T) {
	dk1 := mustKey(t)
	dk2 := mustKey(t)
	plaintext := []byte("the same plaintext on two volumes")

	a, err := EncryptChunk(dk1, plaintext)
	if err != nil {
		t.Fatal(err)
	}
	b, err := EncryptChunk(dk2, plaintext)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(a.Ciphertext, b.Ciphertext) {
		t.Error("different DKs must produce different ciphertext for same plaintext")
	}
	if a.ChunkID == b.ChunkID {
		t.Error("different DKs must produce different chunk ids")
	}
}

// TestRoundtrip: encrypt -> decrypt returns the original plaintext.
func TestRoundtrip(t *testing.T) {
	dk := mustKey(t)
	plaintext := []byte("roundtrip payload: " + string(bytes.Repeat([]byte{0xAB}, 4096)))

	enc, err := EncryptChunk(dk, plaintext)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecryptChunk(dk, enc.Ciphertext, enc.AuthTag, enc.PlaintextHash[:])
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Error("decrypted plaintext differs from input")
	}
}

// TestTamperDetection: mutating ciphertext or tag causes decrypt to
// fail authentication.
func TestTamperDetection(t *testing.T) {
	dk := mustKey(t)
	plaintext := []byte("tamper me")
	enc, err := EncryptChunk(dk, plaintext)
	if err != nil {
		t.Fatal(err)
	}

	// Flip one bit in ciphertext.
	ct := append([]byte{}, enc.Ciphertext...)
	ct[0] ^= 0x01
	if _, err := DecryptChunk(dk, ct, enc.AuthTag, enc.PlaintextHash[:]); err == nil {
		t.Error("tampered ciphertext should fail decrypt")
	}

	// Flip a bit in the tag.
	tag := enc.AuthTag
	tag[0] ^= 0x01
	if _, err := DecryptChunk(dk, enc.Ciphertext, tag, enc.PlaintextHash[:]); err == nil {
		t.Error("tampered auth tag should fail decrypt")
	}

	// Wrong DK should fail.
	otherDK := mustKey(t)
	if _, err := DecryptChunk(otherDK, enc.Ciphertext, enc.AuthTag, enc.PlaintextHash[:]); err == nil {
		t.Error("wrong DK should fail decrypt")
	}
}

// TestDomainSeparation: the key and IV derivations must differ. If
// they were identical we would have key-IV reuse (catastrophic for
// AES-GCM).
func TestDomainSeparation(t *testing.T) {
	dk := mustKey(t)
	hash := sha256.Sum256([]byte("any plaintext"))
	key, iv := DeriveChunkKey(dk, hash[:])

	// The IV must not equal the first 12 bytes of the key.
	if bytes.Equal(key[:ChunkIVSize], iv[:]) {
		t.Error("IV must not equal first 12 bytes of key (domain separation failed)")
	}

	// Two different plaintext hashes must produce two different keys
	// and two different IVs.
	hash2 := sha256.Sum256([]byte("a different plaintext"))
	key2, iv2 := DeriveChunkKey(dk, hash2[:])
	if bytes.Equal(key[:], key2[:]) {
		t.Error("different plaintext hashes should yield different keys")
	}
	if bytes.Equal(iv[:], iv2[:]) {
		t.Error("different plaintext hashes should yield different IVs")
	}
}

// TestDetermism: DeriveChunkKey is a pure function of (dk, hash).
func TestDeriveChunkKeyDeterminism(t *testing.T) {
	dk := mustKey(t)
	hash := sha256.Sum256([]byte("x"))
	k1, iv1 := DeriveChunkKey(dk, hash[:])
	k2, iv2 := DeriveChunkKey(dk, hash[:])
	if k1 != k2 || iv1 != iv2 {
		t.Error("DeriveChunkKey must be deterministic")
	}
}

// TestChunkIDStructure: chunk id is SHA-256(ct||tag).
func TestChunkIDStructure(t *testing.T) {
	dk := mustKey(t)
	enc, err := EncryptChunk(dk, []byte("hi"))
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.New()
	h.Write(enc.Ciphertext)
	h.Write(enc.AuthTag[:])
	var expected ChunkID
	copy(expected[:], h.Sum(nil))
	if enc.ChunkID != expected {
		t.Error("chunk id must equal SHA-256(ciphertext||tag)")
	}
}

// TestSSECNonDedup: two identical plaintexts in the SSEC namespace
// must produce different chunk ids (random IV per write).
func TestSSECNonDedup(t *testing.T) {
	k := mustKey(t)
	pt := []byte("identical customer data")

	a, _, err := EncryptChunkSSEC(k, pt)
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := EncryptChunkSSEC(k, pt)
	if err != nil {
		t.Fatal(err)
	}
	if a.ChunkID == b.ChunkID {
		t.Error("SSEC namespace must not dedup identical plaintexts")
	}
	if bytes.Equal(a.Ciphertext, b.Ciphertext) {
		t.Error("SSEC namespace must not produce identical ciphertexts")
	}
	if a.Namespace != NamespaceSSEC {
		t.Error("namespace should be NamespaceSSEC")
	}
}

// TestSSECRoundtrip: SSEC encrypt -> decrypt returns plaintext.
func TestSSECRoundtrip(t *testing.T) {
	k := mustKey(t)
	pt := []byte("ssec roundtrip")
	enc, iv, err := EncryptChunkSSEC(k, pt)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecryptChunkSSEC(k, enc.Ciphertext, iv, enc.AuthTag)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, pt) {
		t.Error("ssec roundtrip mismatch")
	}
}

func TestKeyCacheLifecycle(t *testing.T) {
	c := NewKeyCache()
	dk, err := GenerateDataKey("vol-1", 1)
	if err != nil {
		t.Fatal(err)
	}
	c.Put("vol-1", dk)

	got, ok := c.Get("vol-1")
	if !ok || got != dk {
		t.Fatal("cache miss")
	}
	raw, err := got.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != ChunkKeySize {
		t.Error("bad key length")
	}
	ZeroBytes(raw)

	if !c.Evict("vol-1") {
		t.Error("expected eviction to return true")
	}
	if _, err := dk.Bytes(); err == nil {
		t.Error("Bytes after Close should fail")
	}
	if _, ok := c.Get("vol-1"); ok {
		t.Error("should be evicted")
	}
}

func TestKeyCachePutReplacesAndZeroises(t *testing.T) {
	c := NewKeyCache()
	a, _ := GenerateDataKey("v", 1)
	b, _ := GenerateDataKey("v", 2)
	c.Put("v", a)
	c.Put("v", b) // should close a
	if _, err := a.Bytes(); err == nil {
		t.Error("replaced DataKey should be closed")
	}
	got, _ := c.Get("v")
	if got != b {
		t.Error("cache should return latest")
	}
	c.Close()
	if c.Len() != 0 {
		t.Error("cache should be empty after Close")
	}
}

func TestDataKeyEqual(t *testing.T) {
	raw := make([]byte, ChunkKeySize)
	for i := range raw {
		raw[i] = byte(i)
	}
	a, _ := NewDataKey("x", 1, raw)
	b, _ := NewDataKey("x", 1, raw)
	if !a.Equal(b) {
		t.Error("identical DKs should compare equal")
	}
	c, _ := GenerateDataKey("y", 1)
	if a.Equal(c) {
		t.Error("different DKs should not compare equal")
	}
}
