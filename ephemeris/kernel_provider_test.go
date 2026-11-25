package ephemeris_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"math"
	"strings"
	"testing"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/ephemeris/jpl"
	"github.com/TuSKan/astrogo/ephemeris/jpl/spk"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
)

// serveKernels points NAIFSPK at a local source holding the given kernels
// under planets/, NAIFLSK at one holding naif0012, and the cache at an empty
// directory, so NewProvider reaches no host.
func serveKernels(t *testing.T, kernels map[string][]byte) {
	t.Helper()

	t.Cleanup(remote.Capture(remote.NAIFSPK, remote.NAIFLSK).Restore)
	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))

	files := map[string][]byte{"lsk/naif0012.tls": []byte(testutil.NAIF0012)}
	for name, b := range kernels {
		files["planets/"+name+".bsp"] = b
	}

	src := testutil.FileURL(t, t.TempDir())

	fsys, err := remote.OpenFS(context.Background(), src)
	if err != nil {
		t.Fatalf("open the source: %v", err)
	}

	for key, b := range files {
		if err := remote.WriteFile(context.Background(), fsys, key, bytes.NewReader(b)); err != nil {
			t.Fatalf("seed %s: %v", key, err)
		}
	}

	for _, id := range []remote.EndpointID{remote.NAIFSPK, remote.NAIFLSK} {
		if err := remote.SetURL(id, src); err != nil {
			t.Fatal(err)
		}
	}
}

// TestNewProviderLoadsAnExtraKernel is the control for the test above: the same
// source, with an extra kernel that can be fetched and added.
func TestNewProviderLoadsAnExtraKernel(t *testing.T) {
	serveKernels(t, map[string][]byte{
		"base":  kernelInFrame(1),
		"extra": kernelInFrame(1),
	})

	p, err := eph.NewProvider(context.Background(), eph.Planets, "base", eph.WithKernel("extra"))
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	jp, ok := p.(*jpl.Provider)
	if !ok {
		t.Fatalf("NewProvider returned a %T, want *jpl.Provider", p)
	}

	if n := len(jp.LoadedKernels()); n != 2 {
		t.Errorf("%d kernels loaded, want the base and the extra", n)
	}
}

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

// TestNewProviderErrorsNameTheEphemerisPackageOnce: NewProvider puts
// "ephemeris: " before what it wraps, and a kernel backend that began its
// errors with it as well once said "ephemeris: ephemeris: new provider".
func TestNewProviderErrorsNameTheEphemerisPackageOnce(t *testing.T) {
	t.Cleanup(remote.Capture().Restore)
	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))
	remote.SetOffline(true)

	_, err := eph.NewProvider(context.Background(), eph.Planets, "de440s")
	if err == nil {
		t.Fatal("an offline kernel fetch returned no error")
	}

	if got := strings.Count(err.Error(), "ephemeris:"); got != 1 {
		t.Errorf("the error names the ephemeris package %d times, want once:\n  %v", got, err)
	}
}
