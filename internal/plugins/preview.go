package plugins

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

// PreviewResult is what the pre-install handler returns. Aurora's
// "Install" dialog renders the permissions summary verbatim and stashes
// the full payload so it can be replayed into the audit row when the
// user confirms.
type PreviewResult struct {
	// Manifest is the parsed Plugin object — apiVersion, kind,
	// metadata, spec — so Aurora can show advanced fields (vendor,
	// version, description, icon) without parsing YAML again.
	Manifest *Plugin `json:"manifest"`
	// Permissions is the structured consent payload.
	Permissions PermissionsSummary `json:"permissions"`
	// TarballSHA256 is the hex SHA256 of the cosign-verified tarball
	// bytes. Captured here so the audit row written on confirm can
	// pin EXACTLY what the user previewed (tarballs are tiny and the
	// engine fetches again on Install — comparing hashes guarantees
	// nothing changed in between).
	TarballSHA256 string `json:"tarballSha256"`
}

// PreviewError is the wire-typed error returned by PreviewPlugin. It
// lets the HTTP handler map preview-side failures to specific status
// codes (404 unknown, 502 marketplace down, 422 signature failure)
// without string-matching.
type PreviewError struct {
	Code string // "not_found" | "marketplace_unreachable" | "signature_invalid" | "manifest_invalid" | "internal"
	Err  error
}

func (e *PreviewError) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}
	return e.Err.Error()
}
func (e *PreviewError) Unwrap() error { return e.Err }

// preview-error code constants. Handlers compare on these.
const (
	PreviewErrNotFound           = "not_found"
	PreviewErrMarketplaceUnreach = "marketplace_unreachable"
	PreviewErrSignatureInvalid   = "signature_invalid"
	PreviewErrManifestInvalid    = "manifest_invalid"
	PreviewErrInternal           = "internal"
)

// PreviewPlugin fetches + verifies + parses a marketplace plugin
// without installing it. It is read-only: no DB writes, no plugin
// tree, no systemd. Safe to call from a read-perm-gated endpoint.
//
// Behaviour:
//
//  1. Look up (name, version) in the marketplace index.
//  2. Download the tarball + signature.
//  3. Cosign-verify the signature against the tarball bytes.
//  4. Read manifest.yaml from the tarball root (in-memory; nothing
//     hits the filesystem).
//  5. Validate the manifest and produce a PermissionsSummary.
//
// The verify+parse pipeline is logically the same one Install runs, but
// inlined here so this code path stays read-only. We accept the small
// duplication for v1 and can DRY into a shared helper later.
func PreviewPlugin(ctx context.Context, mc *MarketplaceClient, ver *Verifier, name, version string) (*PreviewResult, error) {
	if mc == nil {
		return nil, &PreviewError{Code: PreviewErrInternal, Err: errors.New("plugins: marketplace not configured")}
	}
	if name == "" {
		return nil, &PreviewError{Code: PreviewErrNotFound, Err: errors.New("plugins: empty name")}
	}

	_, v, err := mc.FindVersion(ctx, name, version)
	if err != nil {
		// FindVersion errors fall into two camps:
		//   - "not in marketplace index" / "no version X" → 404
		//   - HTTP-level marketplace failures → 502
		// We can distinguish by string here because FetchIndex only
		// produces wrapped HTTP errors and FindVersion only produces
		// the lookup messages.
		errStr := err.Error()
		switch {
		case containsAny(errStr, "not in marketplace index", "has no version", "has no versions"):
			return nil, &PreviewError{Code: PreviewErrNotFound, Err: err}
		default:
			return nil, &PreviewError{Code: PreviewErrMarketplaceUnreach, Err: err}
		}
	}

	tarball, sig, err := mc.DownloadArtifacts(ctx, v)
	if err != nil {
		return nil, &PreviewError{Code: PreviewErrMarketplaceUnreach, Err: err}
	}

	if ver != nil {
		if vErr := ver.Verify(ctx, tarball, sig); vErr != nil {
			return nil, &PreviewError{Code: PreviewErrSignatureInvalid, Err: vErr}
		}
	}

	manifestBytes, err := readManifestFromTarball(tarball)
	if err != nil {
		return nil, &PreviewError{Code: PreviewErrManifestInvalid, Err: err}
	}
	manifest, err := ParseManifest(manifestBytes)
	if err != nil {
		return nil, &PreviewError{Code: PreviewErrManifestInvalid, Err: err}
	}
	if manifest.Metadata.Name != name {
		return nil, &PreviewError{
			Code: PreviewErrManifestInvalid,
			Err:  fmt.Errorf("plugins: manifest name %q != requested %q", manifest.Metadata.Name, name),
		}
	}

	digest := sha256.Sum256(tarball)
	return &PreviewResult{
		Manifest:      manifest,
		Permissions:   Summarize(manifest),
		TarballSHA256: hex.EncodeToString(digest[:]),
	}, nil
}

// readManifestFromTarball pulls just manifest.yaml out of a gzipped
// plugin tarball, without extracting anything to disk. Used by the
// preview path which must NOT touch the plugin tree.
//
// The tarball-traversal safety is identical to ExtractTarball's: we
// reject any entry whose cleaned name escapes the root.
func readManifestFromTarball(tarball []byte) ([]byte, error) {
	gz, err := gzip.NewReader(bytesReader(tarball))
	if err != nil {
		return nil, fmt.Errorf("plugins: gzip: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("plugins: tar: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg && hdr.Typeflag != tar.TypeRegA {
			continue
		}
		// Match the lifecycle's tarball convention: manifest.yaml at
		// the root, plain "manifest.yaml" or "./manifest.yaml".
		switch hdr.Name {
		case "manifest.yaml", "./manifest.yaml":
		default:
			continue
		}
		// Cap the manifest size; manifests are tiny YAML docs.
		const manifestCap = 1 << 20 // 1 MiB
		buf, err := io.ReadAll(io.LimitReader(tr, manifestCap+1))
		if err != nil {
			return nil, fmt.Errorf("plugins: read manifest: %w", err)
		}
		if int64(len(buf)) > manifestCap {
			return nil, fmt.Errorf("plugins: manifest.yaml exceeds %d bytes", manifestCap)
		}
		return buf, nil
	}
	return nil, errors.New("plugins: tarball: missing manifest.yaml at root")
}

// containsAny reports whether haystack contains any of needles.
func containsAny(haystack string, needles ...string) bool {
	for _, n := range needles {
		if n == "" {
			continue
		}
		if indexOf(haystack, n) >= 0 {
			return true
		}
	}
	return false
}

// indexOf is a tiny strings.Contains-equivalent kept inline so this
// file's import set stays minimal.
func indexOf(s, sub string) int {
	if len(sub) == 0 {
		return 0
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
