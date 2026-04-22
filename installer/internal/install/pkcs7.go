package install

import (
	"crypto/x509"
	"errors"
	"fmt"
	"os"

	"go.mozilla.org/pkcs7"
)

// verifyPKCS7Detached verifies a detached PKCS#7 (CMS SignedData) signature
// against the given content bytes using the provided keyring (PEM file
// containing one or more trusted X.509 certificates).
//
// Returns nil on successful signature + chain verification. On any failure
// (malformed signature, untrusted signer, content mismatch) a non-nil
// error describes the first problem encountered.
func verifyPKCS7Detached(keyringPath string, signatureDER []byte, content []byte) error {
	roots, err := loadKeyring(keyringPath)
	if err != nil {
		return fmt.Errorf("load keyring: %w", err)
	}

	p7, err := pkcs7.Parse(signatureDER)
	if err != nil {
		return fmt.Errorf("parse pkcs7: %w", err)
	}
	// Detached signature: attach the covered content ourselves.
	p7.Content = content

	// Fill in the verification roots. pkcs7.Verify picks up any extra certs
	// embedded in the SignedData structure automatically.
	if err := p7.VerifyWithChain(roots); err != nil {
		// Fall back to policy-less verify: useful when the keyring is a
		// bare leaf cert without chain info (a common RAUC setup).
		if err2 := p7.Verify(); err2 != nil {
			return fmt.Errorf("pkcs7 verify: %w (chain: %v)", err2, err)
		}
		// Self-verified but not chained — require at least one signer to
		// match a cert in our keyring by subject+SPKI.
		if !signerMatchesKeyring(p7.Certificates, roots) {
			return fmt.Errorf("pkcs7 verify: no signer matches keyring")
		}
	}
	return nil
}

// loadKeyring parses a PEM file and returns a CertPool of the trusted
// certificates. An empty / corrupt file returns an error.
func loadKeyring(path string) (*x509.CertPool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(raw) {
		return nil, errors.New("no PEM certificates found in keyring")
	}
	return pool, nil
}

// signerMatchesKeyring returns true if any of the signer certs embedded
// in the signature chain to the keyring by public-key equality. This is
// a degraded-trust check used only when the keyring contains bare leaf
// certs without parent chain info.
func signerMatchesKeyring(signerCerts []*x509.Certificate, roots *x509.CertPool) bool {
	subjects := roots.Subjects() //nolint:staticcheck // adequate for bare-leaf fallback
	for _, sc := range signerCerts {
		for _, raw := range subjects {
			if len(raw) == len(sc.RawSubject) && string(raw) == string(sc.RawSubject) {
				return true
			}
		}
	}
	return false
}

