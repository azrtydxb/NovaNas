package tpm

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestWireRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		pcrs []uint8
		pub  []byte
		priv []byte
	}{
		{"single-pcr", []uint8{7}, []byte("pub"), []byte("priv")},
		{"multi-pcr", []uint8{0, 2, 7}, bytes.Repeat([]byte{0xab}, 200), bytes.Repeat([]byte{0xcd}, 100)},
		{"max-pcrs", seq(0, maxPCRs), []byte{0x01}, []byte{0x02}},
		{"empty-pcrs", []uint8{}, []byte("p"), []byte("k")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, err := marshalSealedBlob(tc.pcrs, tc.pub, tc.priv)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			pcrs, pub, priv, err := unmarshalSealedBlob(b)
			if err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if !bytes.Equal(pcrs, tc.pcrs) {
				t.Errorf("pcrs: got %v want %v", pcrs, tc.pcrs)
			}
			if !bytes.Equal(pub, tc.pub) {
				t.Errorf("pub: got %x want %x", pub, tc.pub)
			}
			if !bytes.Equal(priv, tc.priv) {
				t.Errorf("priv: got %x want %x", priv, tc.priv)
			}
		})
	}
}

func TestWireMalformed(t *testing.T) {
	good, err := marshalSealedBlob([]uint8{7}, []byte("pub"), []byte("priv"))
	if err != nil {
		t.Fatalf("baseline marshal: %v", err)
	}

	cases := []struct {
		name    string
		input   []byte
		wantSub string
	}{
		{"empty", nil, "short header"},
		{"bad-magic", append([]byte("XXXX"), good[4:]...), "bad magic"},
		{"truncated-header", good[:5], "short version"},
		{"truncated-pcr-count", good[:7], "short pcr-count"},
		{"truncated-priv", good[:len(good)-1], "short priv"},
		{"trailing-garbage", append(append([]byte{}, good...), 0xff), "trailing bytes"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, _, err := unmarshalSealedBlob(tc.input)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !errors.Is(err, ErrInvalidBlob) {
				t.Errorf("want ErrInvalidBlob, got %v", err)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("error %q does not mention %q", err, tc.wantSub)
			}
		})
	}
}

func TestWireVersionMismatch(t *testing.T) {
	good, err := marshalSealedBlob([]uint8{7}, []byte("pub"), []byte("priv"))
	if err != nil {
		t.Fatal(err)
	}
	// Bump version field at offset 4..6 to 0x0002.
	bad := append([]byte{}, good...)
	bad[4] = 0x00
	bad[5] = 0x02
	_, _, _, err = unmarshalSealedBlob(bad)
	if err == nil {
		t.Fatal("expected version mismatch error")
	}
	if !strings.Contains(err.Error(), "unsupported version") {
		t.Errorf("error %q does not mention version", err)
	}
}

func TestWireRejectsOversizePCRCount(t *testing.T) {
	// Hand-craft a header that claims 200 PCRs.
	buf := []byte("TPMS")
	buf = append(buf, 0x00, 0x01) // version
	buf = append(buf, 0x00, 0xc8) // pcr-count = 200
	_, _, _, err := unmarshalSealedBlob(buf)
	if err == nil || !strings.Contains(err.Error(), "pcr-count too large") {
		t.Fatalf("want pcr-count error, got %v", err)
	}
}

func TestWireRejectsOversizeFields(t *testing.T) {
	if _, err := marshalSealedBlob([]uint8{7}, bytes.Repeat([]byte{0}, maxBlobField+1), []byte("p")); err == nil {
		t.Errorf("expected oversize pub to be rejected")
	}
	if _, err := marshalSealedBlob([]uint8{7}, []byte("p"), bytes.Repeat([]byte{0}, maxBlobField+1)); err == nil {
		t.Errorf("expected oversize priv to be rejected")
	}
}

func TestWireRejectsOutOfRangePCR(t *testing.T) {
	if _, err := marshalSealedBlob([]uint8{maxPCRs}, []byte("p"), []byte("k")); err == nil {
		t.Errorf("expected out-of-range PCR to be rejected")
	}
}

func seq(start, n int) []uint8 {
	out := make([]uint8, n)
	for i := 0; i < n; i++ {
		out[i] = uint8(start + i)
	}
	return out
}
