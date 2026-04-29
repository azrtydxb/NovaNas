// Package tpm wraps a TPM 2.0 device for sealing and unsealing short
// byte strings. It is intended for protecting a small data-encryption
// key (DEK) at rest: the DEK is sealed under the TPM bound to one or
// more PCRs (default PCR 7, the secure-boot state), and the sealed
// blob is safe to store on disk. As long as the same TPM is present
// and the boot state is unchanged, Unseal returns the original bytes.
//
// The package does not implement any larger key-management protocol;
// callers should generate and rotate the DEK themselves and persist
// the sealed blob alongside whatever it protects.
//
// TPM operations are not concurrent-safe at the kernel level, so
// every Sealer method acquires an internal mutex. Sealing is slow
// (~100-300 ms on commodity hardware) — fine for a once-per-boot
// unseal but inappropriate for hot paths.
//
// Operators must run NovaNAS as a user with read/write access to
// /dev/tpmrm0 (the kernel resource manager) — typically root or a
// member of the "tss" group.
package tpm

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"

	"github.com/google/go-tpm/tpm2"
	"github.com/google/go-tpm/tpm2/transport"
	"github.com/google/go-tpm/tpm2/transport/linuxtpm"
)

// ErrNoTPM is returned by New when no usable TPM 2.0 device is present.
var ErrNoTPM = errors.New("no TPM 2.0 device available")

// ErrPCRMismatch is returned by Unseal when the current PCR values do
// not match those the blob was sealed against. This is the expected
// outcome when the boot chain has changed (firmware update, new
// bootloader, secure-boot toggle, etc.) and is therefore reported
// distinctly from generic TPM errors.
var ErrPCRMismatch = errors.New("tpm: PCR state changed since seal (policy authorization failed)")

// Default device paths probed by New.
const (
	defaultDevicePathRM  = "/dev/tpmrm0"
	defaultDevicePathRaw = "/dev/tpm0"
)

// defaultOwnerHandle is the conventional persistent SRK handle under
// the owner hierarchy on most Linux distributions (also used by
// tpm2-tools' tpm2_createprimary -c). When unset, the Sealer creates
// a transient primary key per operation.
const defaultOwnerHandle uint32 = 0x81000001

// openFunc opens a TPM transport. Overridden in tests to swap in the
// in-process simulator.
type openFunc func() (transport.TPMCloser, error)

// Sealer wraps TPM 2.0 to seal/unseal small byte strings (typically a
// data-encryption-key that protects larger secret blobs at rest).
type Sealer struct {
	// DevicePath is the path to the TPM device. Defaults to
	// /dev/tpmrm0 (resource manager) and falls back to /dev/tpm0.
	DevicePath string

	// PCRs are the PCR indexes (in the SHA-256 bank) the sealed blob
	// is bound to. Defaults to {7} — the UEFI secure-boot state.
	PCRs []int

	// OwnerHandle, if non-zero, names a persistent SRK under the
	// owner hierarchy (TPM2_RH_OWNER) to use as the parent of sealed
	// objects. When zero we create a transient primary per operation.
	OwnerHandle uint32

	log  *slog.Logger
	mu   sync.Mutex
	open openFunc
}

// New creates a Sealer with sensible defaults. It probes the TPM at
// construction time and returns ErrNoTPM if no device is available.
// The returned Sealer does not hold an open device handle; each
// operation opens and closes the device.
func New(log *slog.Logger) (*Sealer, error) {
	if log == nil {
		log = slog.Default()
	}
	path, err := probeTPM()
	if err != nil {
		return nil, err
	}
	s := &Sealer{
		DevicePath:  path,
		PCRs:        []int{7},
		OwnerHandle: defaultOwnerHandle,
		log:         log,
	}
	s.open = func() (transport.TPMCloser, error) {
		return linuxtpm.Open(s.DevicePath)
	}
	log.Info("tpm: initialized", "device", path)
	return s, nil
}

// probeTPM checks for an accessible TPM device. It prefers the
// resource manager (/dev/tpmrm0) which serializes access between
// processes; the raw /dev/tpm0 is a fallback for systems without the
// in-kernel resource manager.
func probeTPM() (string, error) {
	for _, p := range []string{defaultDevicePathRM, defaultDevicePathRaw} {
		fi, err := os.Stat(p)
		if err != nil {
			continue
		}
		if fi.Mode()&os.ModeDevice == 0 {
			continue
		}
		return p, nil
	}
	return "", ErrNoTPM
}

// Close releases any TPM resources held by the Sealer. The current
// implementation opens the device per call, so this is a no-op kept
// for API symmetry and forward compatibility.
func (s *Sealer) Close() error {
	return nil
}

// pcrSelection builds a SHA-256 PCR selection over s.PCRs.
func (s *Sealer) pcrSelection() tpm2.TPMLPCRSelection {
	return tpm2.TPMLPCRSelection{
		PCRSelections: []tpm2.TPMSPCRSelection{{
			Hash:      tpm2.TPMAlgSHA256,
			PCRSelect: tpm2.PCClientCompatible.PCRs(uintsFromInts(s.PCRs)...),
		}},
	}
}

func uintsFromInts(in []int) []uint {
	out := make([]uint, len(in))
	for i, v := range in {
		out[i] = uint(v)
	}
	return out
}

// pcrIndexesU8 returns the PCR indexes as uint8 for the wire format.
func (s *Sealer) pcrIndexesU8() ([]uint8, error) {
	out := make([]uint8, len(s.PCRs))
	for i, v := range s.PCRs {
		if v < 0 || v > 23 {
			return nil, fmt.Errorf("tpm: PCR index out of range: %d", v)
		}
		out[i] = uint8(v)
	}
	return out, nil
}

// trialPolicyDigest runs a trial PolicyPCR session to compute the
// authPolicy that must be baked into the sealed object's public area.
func trialPolicyDigest(t transport.TPM, sel tpm2.TPMLPCRSelection) ([]byte, error) {
	sess, cleanup, err := tpm2.PolicySession(t, tpm2.TPMAlgSHA256, 16, tpm2.Trial())
	if err != nil {
		return nil, fmt.Errorf("tpm: trial PolicySession: %w", err)
	}
	defer cleanup()

	if _, err := (&tpm2.PolicyPCR{
		PolicySession: sess.Handle(),
		Pcrs:          sel,
	}).Execute(t); err != nil {
		return nil, fmt.Errorf("tpm: trial PolicyPCR: %w", err)
	}
	pgd, err := (&tpm2.PolicyGetDigest{PolicySession: sess.Handle()}).Execute(t)
	if err != nil {
		return nil, fmt.Errorf("tpm: PolicyGetDigest: %w", err)
	}
	return pgd.PolicyDigest.Buffer, nil
}

// createPrimarySRK creates a transient ECC-P256 storage root key
// under the owner hierarchy. The same template is used regardless of
// whether OwnerHandle is set — when it is, this is bypassed entirely.
func createPrimarySRK(t transport.TPM) (*tpm2.CreatePrimaryResponse, error) {
	rsp, err := (&tpm2.CreatePrimary{
		PrimaryHandle: tpm2.TPMRHOwner,
		InPublic:      tpm2.New2B(tpm2.ECCSRKTemplate),
	}).Execute(t)
	if err != nil {
		return nil, fmt.Errorf("tpm: CreatePrimary(SRK): %w", err)
	}
	return rsp, nil
}

// withSRK runs fn with a parent (SRK) handle/name. If OwnerHandle is
// set we use the persistent SRK; otherwise we create a transient one
// and flush it on return.
func (s *Sealer) withSRK(t transport.TPM, fn func(parent tpm2.AuthHandle) error) error {
	if s.OwnerHandle != 0 {
		// Trust the persistent SRK exists. We need its Name for
		// HMAC sessions — read it.
		h := tpm2.TPMHandle(s.OwnerHandle)
		rp, err := (&tpm2.ReadPublic{ObjectHandle: h}).Execute(t)
		if err != nil {
			// Fall back to a transient SRK if the persistent one
			// isn't there. Many systems don't pre-provision one.
			s.log.Debug("tpm: persistent SRK not present, creating transient", "handle", fmt.Sprintf("0x%08x", s.OwnerHandle), "err", err)
			return s.withTransientSRK(t, fn)
		}
		return fn(tpm2.AuthHandle{
			Handle: h,
			Name:   rp.Name,
			Auth:   tpm2.PasswordAuth(nil),
		})
	}
	return s.withTransientSRK(t, fn)
}

func (s *Sealer) withTransientSRK(t transport.TPM, fn func(parent tpm2.AuthHandle) error) error {
	srk, err := createPrimarySRK(t)
	if err != nil {
		return err
	}
	defer func() {
		if _, ferr := (&tpm2.FlushContext{FlushHandle: srk.ObjectHandle}).Execute(t); ferr != nil {
			s.log.Warn("tpm: FlushContext(SRK) failed", "err", ferr)
		}
	}()
	return fn(tpm2.AuthHandle{
		Handle: srk.ObjectHandle,
		Name:   srk.Name,
		Auth:   tpm2.PasswordAuth(nil),
	})
}

// Seal binds plaintext to the current TPM and PCR state. The returned
// blob is opaque and safe to store on disk; it can only be unsealed by
// the same TPM with PCRs in the state they were in at seal time.
func (s *Sealer) Seal(plaintext []byte) ([]byte, error) {
	if len(plaintext) == 0 {
		return nil, errors.New("tpm: refusing to seal empty plaintext")
	}
	if len(plaintext) > 128 {
		// TPM2_Create's TPM2BSensitiveData has a hard limit; 128
		// bytes is comfortably within spec and well-suited to a DEK.
		return nil, fmt.Errorf("tpm: plaintext too long (%d bytes); seal short keys only", len(plaintext))
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	t, err := s.open()
	if err != nil {
		return nil, fmt.Errorf("tpm: open: %w", err)
	}
	defer t.Close()

	sel := s.pcrSelection()

	authPolicy, err := trialPolicyDigest(t, sel)
	if err != nil {
		return nil, err
	}

	var sealedPub tpm2.TPM2BPublic
	var sealedPriv tpm2.TPM2BPrivate

	err = s.withSRK(t, func(parent tpm2.AuthHandle) error {
		create := tpm2.Create{
			ParentHandle: parent,
			InSensitive: tpm2.TPM2BSensitiveCreate{
				Sensitive: &tpm2.TPMSSensitiveCreate{
					Data: tpm2.NewTPMUSensitiveCreate(&tpm2.TPM2BSensitiveData{
						Buffer: plaintext,
					}),
				},
			},
			InPublic: tpm2.New2B(tpm2.TPMTPublic{
				Type:    tpm2.TPMAlgKeyedHash,
				NameAlg: tpm2.TPMAlgSHA256,
				ObjectAttributes: tpm2.TPMAObject{
					FixedTPM:    true,
					FixedParent: true,
					NoDA:        true,
					// UserWithAuth is intentionally false — auth is
					// only allowed via the PCR policy session.
				},
				AuthPolicy: tpm2.TPM2BDigest{Buffer: authPolicy},
			}),
		}
		rsp, err := create.Execute(t)
		if err != nil {
			return fmt.Errorf("tpm: Create(sealed): %w", err)
		}
		sealedPub = rsp.OutPublic
		sealedPriv = rsp.OutPrivate
		return nil
	})
	if err != nil {
		return nil, err
	}

	pcrIdx, err := s.pcrIndexesU8()
	if err != nil {
		return nil, err
	}
	return marshalSealedBlob(pcrIdx, tpm2.Marshal(&sealedPub), tpm2.Marshal(&sealedPriv))
}

// Unseal reverses Seal. Returns ErrPCRMismatch when the PCR state has
// changed since the blob was sealed.
func (s *Sealer) Unseal(sealed []byte) ([]byte, error) {
	pcrIdx, pubBytes, privBytes, err := unmarshalSealedBlob(sealed)
	if err != nil {
		return nil, err
	}

	pub, err := tpm2.Unmarshal[tpm2.TPM2BPublic](pubBytes)
	if err != nil {
		return nil, fmt.Errorf("tpm: unmarshal public: %w", err)
	}
	priv, err := tpm2.Unmarshal[tpm2.TPM2BPrivate](privBytes)
	if err != nil {
		return nil, fmt.Errorf("tpm: unmarshal private: %w", err)
	}

	// Build the PCR selection from the blob's recorded indexes, not
	// from s.PCRs — the blob is self-describing and a Sealer
	// reconfigured between Seal and Unseal should still work.
	pcrInts := make([]uint, len(pcrIdx))
	for i, v := range pcrIdx {
		pcrInts[i] = uint(v)
	}
	sel := tpm2.TPMLPCRSelection{
		PCRSelections: []tpm2.TPMSPCRSelection{{
			Hash:      tpm2.TPMAlgSHA256,
			PCRSelect: tpm2.PCClientCompatible.PCRs(pcrInts...),
		}},
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	t, err := s.open()
	if err != nil {
		return nil, fmt.Errorf("tpm: open: %w", err)
	}
	defer t.Close()

	var out []byte
	err = s.withSRK(t, func(parent tpm2.AuthHandle) error {
		loadRsp, err := (&tpm2.Load{
			ParentHandle: parent,
			InPrivate:    *priv,
			InPublic:     *pub,
		}).Execute(t)
		if err != nil {
			return fmt.Errorf("tpm: Load(sealed): %w", err)
		}
		defer func() {
			if _, ferr := (&tpm2.FlushContext{FlushHandle: loadRsp.ObjectHandle}).Execute(t); ferr != nil {
				s.log.Warn("tpm: FlushContext(sealed) failed", "err", ferr)
			}
		}()

		sess, cleanup, err := tpm2.PolicySession(t, tpm2.TPMAlgSHA256, 16)
		if err != nil {
			return fmt.Errorf("tpm: PolicySession: %w", err)
		}
		defer cleanup()

		if _, err := (&tpm2.PolicyPCR{
			PolicySession: sess.Handle(),
			Pcrs:          sel,
		}).Execute(t); err != nil {
			return fmt.Errorf("tpm: PolicyPCR: %w", err)
		}

		unsealRsp, err := (&tpm2.Unseal{
			ItemHandle: tpm2.AuthHandle{
				Handle: loadRsp.ObjectHandle,
				Name:   loadRsp.Name,
				Auth:   sess,
			},
		}).Execute(t)
		if err != nil {
			if isPolicyFailure(err) {
				return ErrPCRMismatch
			}
			return fmt.Errorf("tpm: Unseal: %w", err)
		}
		out = make([]byte, len(unsealRsp.OutData.Buffer))
		copy(out, unsealRsp.OutData.Buffer)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// isPolicyFailure reports whether err looks like a TPM_RC_POLICY_FAIL
// (or related) — the specific outcome when PCR values differ from
// those the object's authPolicy was bound to.
func isPolicyFailure(err error) bool {
	return errors.Is(err, tpm2.TPMRCPolicyFail) ||
		errors.Is(err, tpm2.TPMRCPolicy) ||
		errors.Is(err, tpm2.TPMRCValue)
}
