package plugins

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"os/exec"
)

// DefaultTrustKeyPath is where the cosign public key lives by default.
// Operators can override with MARKETPLACE_TRUST_KEY_PATH.
const DefaultTrustKeyPath = "/etc/nova-nas/trust/marketplace.pub"

// Verifier validates signed marketplace tarballs against a cosign
// public key. Two implementations are wired:
//
//   - The default (Go-native) path: parses a PEM-encoded ECDSA/RSA/Ed25519
//     public key and verifies a base64-encoded raw signature against
//     SHA256(tarball). This matches cosign's "blob, --output-signature"
//     mode and avoids pulling in the heavy sigstore Go deps.
//   - When CosignBin is set and the cosign binary is on PATH, the
//     verifier shells out to `cosign verify-blob`. This is the path
//     operators use when they want full cosign semantics (rekor,
//     transparency, etc.).
//
// The decision rationale: at v1 the marketplace publishes a static
// detached signature alongside each tarball. Native PEM verification
// is sufficient and keeps the binary lean. Operators who want rekor
// can opt in by setting MARKETPLACE_COSIGN_BIN.
type Verifier struct {
	PublicKeyPath string
	CosignBin     string // optional; when set, shell out
}

// NewVerifier constructs a Verifier from path. Empty path falls back
// to DefaultTrustKeyPath.
func NewVerifier(path string) *Verifier {
	if path == "" {
		path = DefaultTrustKeyPath
	}
	return &Verifier{PublicKeyPath: path}
}

// Verify checks that signature is a valid cosign signature of tarball
// produced by the configured key. Returns nil on success; a wrapped
// error otherwise. The bytes are NEVER trusted on a non-nil error.
func (v *Verifier) Verify(ctx context.Context, tarball, signature []byte) error {
	if v == nil || v.PublicKeyPath == "" {
		return errors.New("plugins: verifier: no public key configured")
	}
	if len(tarball) == 0 {
		return errors.New("plugins: verifier: empty tarball")
	}
	if len(signature) == 0 {
		return errors.New("plugins: verifier: empty signature")
	}
	if v.CosignBin != "" {
		return v.verifyWithCosign(ctx, tarball, signature)
	}
	return v.verifyNative(tarball, signature)
}

func (v *Verifier) verifyNative(tarball, signature []byte) error {
	keyBytes, err := os.ReadFile(v.PublicKeyPath)
	if err != nil {
		return fmt.Errorf("plugins: read trust key: %w", err)
	}
	block, _ := pem.Decode(keyBytes)
	if block == nil {
		return errors.New("plugins: trust key: not PEM-encoded")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("plugins: trust key parse: %w", err)
	}
	// cosign emits sigs as base64. Accept both raw and base64 forms so
	// operators can paste either into a fixture.
	sig := signature
	if decoded, derr := base64.StdEncoding.DecodeString(string(signature)); derr == nil {
		sig = decoded
	}
	digest := sha256.Sum256(tarball)
	switch k := pub.(type) {
	case *ecdsa.PublicKey:
		if !ecdsa.VerifyASN1(k, digest[:], sig) {
			return errors.New("plugins: signature: ecdsa verify failed")
		}
		return nil
	case *rsa.PublicKey:
		if err := rsa.VerifyPKCS1v15(k, crypto.SHA256, digest[:], sig); err != nil {
			return fmt.Errorf("plugins: signature: rsa: %w", err)
		}
		return nil
	case ed25519.PublicKey:
		if !ed25519.Verify(k, tarball, sig) {
			return errors.New("plugins: signature: ed25519 verify failed")
		}
		return nil
	default:
		return fmt.Errorf("plugins: unsupported key type %T", pub)
	}
}

func (v *Verifier) verifyWithCosign(ctx context.Context, tarball, signature []byte) error {
	tmp, err := os.CreateTemp("", "novanas-pkg-*.tar.gz")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(tarball); err != nil {
		_ = tmp.Close()
		return err
	}
	_ = tmp.Close()

	sigTmp, err := os.CreateTemp("", "novanas-pkg-*.sig")
	if err != nil {
		return err
	}
	defer os.Remove(sigTmp.Name())
	if _, err := sigTmp.Write(signature); err != nil {
		_ = sigTmp.Close()
		return err
	}
	_ = sigTmp.Close()

	cmd := exec.CommandContext(ctx, v.CosignBin,
		"verify-blob",
		"--key", v.PublicKeyPath,
		"--signature", sigTmp.Name(),
		tmp.Name(),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("plugins: cosign verify-blob: %w (%s)", err, string(out))
	}
	return nil
}
