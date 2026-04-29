package plugins

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

func writeTestKey(t *testing.T) (priv *ecdsa.PrivateKey, pubPath string) {
	t.Helper()
	p, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(&p.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	pem := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
	dir := t.TempDir()
	path := filepath.Join(dir, "pub.pem")
	if err := os.WriteFile(path, pem, 0o644); err != nil {
		t.Fatal(err)
	}
	return p, path
}

func TestVerifier_Native_GoodSig(t *testing.T) {
	priv, path := writeTestKey(t)
	tarball := []byte("fake tarball contents")
	digest := sha256.Sum256(tarball)
	sig, err := ecdsa.SignASN1(rand.Reader, priv, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	v := NewVerifier(path)
	if err := v.Verify(context.Background(), tarball, []byte(base64.StdEncoding.EncodeToString(sig))); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
}

func TestVerifier_Native_TamperedTarball(t *testing.T) {
	priv, path := writeTestKey(t)
	tarball := []byte("fake tarball contents")
	digest := sha256.Sum256(tarball)
	sig, _ := ecdsa.SignASN1(rand.Reader, priv, digest[:])
	v := NewVerifier(path)
	if err := v.Verify(context.Background(), append(tarball, '!'), sig); err == nil {
		t.Fatal("expected tampering rejection")
	}
}

func TestVerifier_Native_EmptyArgs(t *testing.T) {
	_, path := writeTestKey(t)
	v := NewVerifier(path)
	if err := v.Verify(context.Background(), nil, []byte("x")); err == nil {
		t.Fatal("empty tarball must fail")
	}
	if err := v.Verify(context.Background(), []byte("x"), nil); err == nil {
		t.Fatal("empty sig must fail")
	}
}

func TestVerifier_NoKey(t *testing.T) {
	v := &Verifier{}
	if err := v.Verify(context.Background(), []byte("x"), []byte("y")); err == nil {
		t.Fatal("expected no-key rejection")
	}
}
