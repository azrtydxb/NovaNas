package tpm

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// Wire format for sealed blobs persisted to disk:
//
//   4  bytes  magic     = "TPMS"
//   2  bytes  version   = 1 (big-endian)
//   2  bytes  pcrCount  = N (big-endian, must be <= 24)
//   N  bytes  pcrIdxs   (one uint8 per PCR, value 0..23)
//   2  bytes  pubLen    = M (big-endian)
//   M  bytes  pub       = TPM2B_PUBLIC bytes (already includes its
//                          own length prefix)
//   2  bytes  privLen   = K (big-endian)
//   K  bytes  priv      = TPM2B_PRIVATE bytes
//
// The redundant outer length prefixes make malformed input cheap to
// reject without parsing the TPM-internal structures, and let us
// upgrade the format in the future without touching go-tpm.

const (
	wireMagic   = "TPMS"
	wireVersion = 1
	maxPCRs     = 24
)

// Limits for the outer length-prefixed fields. TPM2B_PUBLIC and
// TPM2B_PRIVATE for a sealed keyedHash object are well under 1 KiB
// in practice; we cap each at 4 KiB to be safe against pathological
// inputs without rejecting legitimate ones.
const maxBlobField = 4096

// ErrInvalidBlob is returned for any unmarshalling failure: bad magic,
// length overruns, version mismatch, etc. Specific causes are wrapped.
var ErrInvalidBlob = errors.New("tpm: invalid sealed blob")

func marshalSealedBlob(pcrs []uint8, pub, priv []byte) ([]byte, error) {
	if len(pcrs) > maxPCRs {
		return nil, fmt.Errorf("%w: too many PCRs (%d)", ErrInvalidBlob, len(pcrs))
	}
	if len(pub) > maxBlobField {
		return nil, fmt.Errorf("%w: pub too large (%d)", ErrInvalidBlob, len(pub))
	}
	if len(priv) > maxBlobField {
		return nil, fmt.Errorf("%w: priv too large (%d)", ErrInvalidBlob, len(priv))
	}
	for _, p := range pcrs {
		if p >= maxPCRs {
			return nil, fmt.Errorf("%w: PCR index out of range: %d", ErrInvalidBlob, p)
		}
	}

	out := make([]byte, 0, 4+2+2+len(pcrs)+2+len(pub)+2+len(priv))
	out = append(out, wireMagic...)
	out = binary.BigEndian.AppendUint16(out, wireVersion)
	out = binary.BigEndian.AppendUint16(out, uint16(len(pcrs)))
	out = append(out, pcrs...)
	out = binary.BigEndian.AppendUint16(out, uint16(len(pub)))
	out = append(out, pub...)
	out = binary.BigEndian.AppendUint16(out, uint16(len(priv)))
	out = append(out, priv...)
	return out, nil
}

func unmarshalSealedBlob(b []byte) (pcrs []uint8, pub, priv []byte, err error) {
	r := wireReader{b: b}
	magic, err := r.take(4)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("%w: short header", ErrInvalidBlob)
	}
	if string(magic) != wireMagic {
		return nil, nil, nil, fmt.Errorf("%w: bad magic %q", ErrInvalidBlob, magic)
	}
	ver, err := r.takeU16()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("%w: short version", ErrInvalidBlob)
	}
	if ver != wireVersion {
		return nil, nil, nil, fmt.Errorf("%w: unsupported version %d", ErrInvalidBlob, ver)
	}
	pcrCount, err := r.takeU16()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("%w: short pcr-count", ErrInvalidBlob)
	}
	if pcrCount > maxPCRs {
		return nil, nil, nil, fmt.Errorf("%w: pcr-count too large (%d)", ErrInvalidBlob, pcrCount)
	}
	pcrBytes, err := r.take(int(pcrCount))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("%w: short pcr list", ErrInvalidBlob)
	}
	pcrs = make([]uint8, len(pcrBytes))
	copy(pcrs, pcrBytes)
	for _, p := range pcrs {
		if p >= maxPCRs {
			return nil, nil, nil, fmt.Errorf("%w: PCR index out of range: %d", ErrInvalidBlob, p)
		}
	}

	pubLen, err := r.takeU16()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("%w: short pub-len", ErrInvalidBlob)
	}
	if int(pubLen) > maxBlobField {
		return nil, nil, nil, fmt.Errorf("%w: pub-len too large (%d)", ErrInvalidBlob, pubLen)
	}
	pubBytes, err := r.take(int(pubLen))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("%w: short pub", ErrInvalidBlob)
	}
	pub = make([]byte, len(pubBytes))
	copy(pub, pubBytes)

	privLen, err := r.takeU16()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("%w: short priv-len", ErrInvalidBlob)
	}
	if int(privLen) > maxBlobField {
		return nil, nil, nil, fmt.Errorf("%w: priv-len too large (%d)", ErrInvalidBlob, privLen)
	}
	privBytes, err := r.take(int(privLen))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("%w: short priv", ErrInvalidBlob)
	}
	priv = make([]byte, len(privBytes))
	copy(priv, privBytes)

	if r.remaining() != 0 {
		return nil, nil, nil, fmt.Errorf("%w: %d trailing bytes", ErrInvalidBlob, r.remaining())
	}
	return pcrs, pub, priv, nil
}

// wireReader is a tiny bounded reader. It returns io.ErrUnexpectedEOF
// (well, a sentinel) when asked for more bytes than remain.
type wireReader struct {
	b   []byte
	pos int
}

var errShort = errors.New("short")

func (r *wireReader) take(n int) ([]byte, error) {
	if n < 0 || r.pos+n > len(r.b) {
		return nil, errShort
	}
	out := r.b[r.pos : r.pos+n]
	r.pos += n
	return out, nil
}

func (r *wireReader) takeU16() (uint16, error) {
	b, err := r.take(2)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(b), nil
}

func (r *wireReader) remaining() int { return len(r.b) - r.pos }
