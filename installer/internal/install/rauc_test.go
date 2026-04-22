package install

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.mozilla.org/pkcs7"
)

// makeTestCert issues a self-signed ECDSA-P256 cert + key for RAUC bundle
// signing tests.
func makeTestCert(t *testing.T) (*x509.Certificate, *ecdsa.PrivateKey, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "novanas-rauc-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageCodeSigning},
		IsCA:         true,
		BasicConstraintsValid: true,
	}
	derBytes, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	cert, err := x509.ParseCertificate(derBytes)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	return cert, key, pemBytes
}

// writeBundle constructs a minimum-viable RAUC classic bundle: a fake
// payload (size > 1 MiB so the stat check passes) followed by a detached
// CMS signature and a big-endian uint32 trailer with the signature size.
func writeBundle(t *testing.T, path string, payload, sig []byte) {
	t.Helper()
	var buf bytes.Buffer
	buf.Write(payload)
	buf.Write(sig)
	var sz [4]byte
	binary.BigEndian.PutUint32(sz[:], uint32(len(sig)))
	buf.Write(sz[:])
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("write bundle: %v", err)
	}
}

func TestRAUCExtractor_VerifyInProcess(t *testing.T) {
	cert, key, pemCert := makeTestCert(t)

	// Payload large enough to pass the > 1 MiB sanity check.
	payload := bytes.Repeat([]byte{0xA5}, 2*1024*1024)

	signed, err := pkcs7.NewSignedData(payload)
	if err != nil {
		t.Fatalf("new signed data: %v", err)
	}
	signed.SetDigestAlgorithm(pkcs7.OIDDigestAlgorithmSHA256)
	if err := signed.AddSigner(cert, key, pkcs7.SignerInfoConfig{}); err != nil {
		t.Fatalf("add signer: %v", err)
	}
	signed.Detach()
	sig, err := signed.Finish()
	if err != nil {
		t.Fatalf("finish: %v", err)
	}

	tmp := t.TempDir()
	keyringPath := filepath.Join(tmp, "keyring.pem")
	if err := os.WriteFile(keyringPath, pemCert, 0o600); err != nil {
		t.Fatalf("write keyring: %v", err)
	}
	bundlePath := filepath.Join(tmp, "bundle.raucb")
	writeBundle(t, bundlePath, payload, sig)

	ex := &RAUCExtractor{
		KeyringPath: keyringPath,
		// Exec is wired but shouldn't be invoked because in-process path wins.
		Exec: func(name string, args ...string) error {
			t.Fatalf("unexpected shellout: %s %v", name, args)
			return nil
		},
	}
	if err := ex.Verify(bundlePath); err != nil {
		t.Fatalf("verify: %v", err)
	}

	// Tamper: flip a byte in the payload and rewrite the bundle.
	payload2 := append([]byte(nil), payload...)
	payload2[1234] ^= 0xFF
	writeBundle(t, bundlePath, payload2, sig)

	ex.Exec = func(name string, args ...string) error {
		// Fallback will be invoked on tamper; simulate rauc verify failing.
		return &execErr{s: "rauc: tampered"}
	}
	if err := ex.Verify(bundlePath); err == nil {
		t.Fatalf("expected verify to fail on tampered bundle")
	}
}

type execErr struct{ s string }

func (e *execErr) Error() string { return e.s }
