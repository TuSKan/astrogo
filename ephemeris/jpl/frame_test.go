package jpl_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/jpl"
	"github.com/TuSKan/astrogo/ephemeris/jpl/spk"
	"github.com/TuSKan/astrogo/remote"
)

// A segment is usable only in NAIF frame 1, J2000, the frame every chain
// this provider sums assumes; a kernel holding any other is refused whole
// (#813).

// kernelInFrame is a minimal DAF/SPK file — the 1024-byte file record and one
// summary record — holding a single segment for target 499 about 4 in the
// given NAIF frame. Adding a kernel reads only its summaries, so no segment
// data is needed.
func kernelInFrame(frame int32) []byte {
	const record = spk.RecordSize

	buf := make([]byte, 2*record)
	le := binary.LittleEndian

	// File record: ND = 2, NI = 6 (the SPK shape), the first summary record
	// at record 2, little-endian.
	le.PutUint32(buf[8:12], 2)
	le.PutUint32(buf[12:16], 6)
	le.PutUint32(buf[76:80], 2)
	copy(buf[88:96], "LTL-IEEE")

	// Summary record: no next record, one summary.
	sum := buf[record:]
	le.PutUint64(sum[0:8], math.Float64bits(0))
	le.PutUint64(sum[16:24], math.Float64bits(1))
	le.PutUint64(sum[24:32], math.Float64bits(-1e8)) // start ET
	le.PutUint64(sum[32:40], math.Float64bits(1e8))  // end ET

	for i, v := range []int32{499, 4, frame, 2, 1000, 2000} { // target, center, frame, type, start, end
		le.PutUint32(sum[40+4*i:], uint32(v))
	}

	return buf
}

type bytesKernel struct{ *bytes.Reader }

func (bytesKernel) Close() error { return nil }

// TestKernelInAnotherFrameIsRefused: ECLIPJ2000 and B1950 segments are
// refused rather than chained unrotated — ECLIPJ2000 would put every vector
// 23.4° off — and the provider is left without the kernel. J2000 is the
// control: the same file in frame 1 is accepted.
func TestKernelInAnotherFrameIsRefused(t *testing.T) {
	for _, c := range []struct {
		name  string
		frame int32
		want  error
	}{
		{"J2000", 1, nil},
		{"B1950", 2, jpl.ErrUnsupportedFrame},
		{"ECLIPJ2000", 17, jpl.ErrUnsupportedFrame},
	} {
		r, err := spk.NewReader(bytesKernel{bytes.NewReader(kernelInFrame(c.frame))})
		if err != nil {
			t.Fatalf("%s: the fixture: %v", c.name, err)
		}

		p := &jpl.Provider{}

		err = p.AddKernel(r)
		if !errors.Is(err, c.want) {
			t.Errorf("%s: AddKernel err = %v, want %v", c.name, err, c.want)
		}

		if loaded := len(p.LoadedKernels()); (c.want == nil) != (loaded == 1) {
			t.Errorf("%s: %d kernels loaded after AddKernel", c.name, loaded)
		}
	}
}

// TestOpenRefusesAKernelInAnotherFrame: the same refusal through Open, which
// opens the kernel itself and so must close what it could not add.
func TestOpenRefusesAKernelInAnotherFrame(t *testing.T) {
	ctx := context.Background()

	fsys, err := remote.OpenFS(ctx, "mem://jpl-frame-813")
	if err != nil {
		t.Fatalf("mem: %v", err)
	}

	if err := remote.WriteFile(ctx, fsys, "ecliptic.bsp", bytes.NewReader(kernelInFrame(17))); err != nil {
		t.Fatalf("write: %v", err)
	}

	if _, err := jpl.Open(ctx, fsys, "absent.tls", "ecliptic.bsp"); !errors.Is(err, jpl.ErrUnsupportedFrame) {
		t.Errorf("Open err = %v, want ErrUnsupportedFrame", err)
	}
}
