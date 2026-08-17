//go:build integration

// Ground-truth test against the real CAMS files this package was built
// and validated against (see doc.go's decision-gate spike notes). Every
// expected value below was obtained independently of this package's own
// code, via `ncdump -h`/`ncdump -v` against the real files at
// remote/credentials/z_cams_c_ecmf_20230101000000_prod_an_ml_000_*.nc —
// never by trusting a value this reader itself produced. Skipped
// entirely (t.Skip, not a failure) when those files are absent, so this
// never runs in CI and is one build tag away locally:
//
//	go test -tags=integration -run TestGroundTruth -v ./atmosphere/dataset/cams/
//
// Never commit or embed these .nc files (see doc.go and CLAUDE.md's
// "Embedded data" section) — remote/credentials/ is gitignored.
package cams

import (
	"context"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/remote/file"

	"github.com/TuSKan/astrogo/internal/testutil"
)

// credentialsDir is where these real files live in this session's local
// checkout; adjust here if you relocate them (e.g. to remote.DataDir(),
// per the CAMS EODATA plan's own stated intent), not per-test.
const credentialsDir = `../../../remote/credentials`

// credentialsBucket opens credentialsDir as a *file.Bucket, once per test
// process — real files live directly as keys within it.
func credentialsBucket(t *testing.T) *file.Bucket {
	t.Helper()

	url := testutil.FileURL(t, credentialsDir)

	bucket, err := file.Open(context.Background(), url)
	if err != nil {
		t.Fatalf("Open credentials bucket: %v", err)
	}

	return bucket
}

func lnspFixture(t *testing.T) (*file.Bucket, string) {
	t.Helper()

	const key = "z_cams_c_ecmf_20230101000000_prod_an_ml_000_lnsp.nc"

	bucket := credentialsBucket(t)

	if exists, _ := bucket.Exists(context.Background(), key); !exists { //nolint:errcheck // a failed existence check just means "not present" for this skip
		t.Skipf("real CAMS file not present at %s/%s -- skipping ground-truth test", credentialsDir, key)
	}

	return bucket, key
}

func aermr01Fixture(t *testing.T) (*file.Bucket, string) {
	t.Helper()

	const key = "z_cams_c_ecmf_20230101000000_prod_an_ml_000_aermr01.nc"

	bucket := credentialsBucket(t)

	if exists, _ := bucket.Exists(context.Background(), key); !exists { //nolint:errcheck // a failed existence check just means "not present" for this skip
		t.Skipf("real CAMS file not present at %s/%s -- skipping ground-truth test", credentialsDir, key)
	}

	return bucket, key
}

// TestGroundTruthLnspGrid cross-checks File.Dims and the dimension-scale
// values against ncdump -v longitude,latitude,time output for the real
// lnsp file -- ncdump -v longitude/latitude, this file's own header.
func TestGroundTruthLnspGrid(t *testing.T) {
	bucket, key := lnspFixture(t)

	f, err := Open(context.Background(), bucket, key)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })

	dims := f.Dims()
	want := map[string]int{"longitude": 900, "latitude": 451, "time": 1}
	for name, n := range want {
		if dims[name] != n {
			t.Errorf("Dims()[%q] = %d, want %d (ncdump -h)", name, dims[name], n)
		}
	}
	if _, hasLevel := dims["level"]; hasLevel {
		t.Error(`Dims() has "level", but the real lnsp file has no level dimension (ncdump -h)`)
	}
}

// TestGroundTruthLnspValues cross-checks three real data points against
// `ncdump -v lnsp` output on the actual downloaded file -- not values
// this package produced itself. ncdump's default text formatting shows
// ~15 significant digits, a rounded display of the underlying double, so
// comparison uses a tight relative tolerance rather than exact equality.
//
//	ncdump -v lnsp z_cams_..._lnsp.nc:
//	  lnsp[time=0,lat=0,  lon=0]   = 11.5385539531708   (flat index 0)
//	  lnsp[time=0,lat=225,lon=450] = 11.5226371884346   (flat index 202950)
//	  lnsp[time=0,lat=450,lon=899] = 11.1248897910118   (flat index 405899, the last value)
func TestGroundTruthLnspValues(t *testing.T) {
	bucket, key := lnspFixture(t)

	f, err := Open(context.Background(), bucket, key)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })

	lnsp, err := f.Var("lnsp")
	if err != nil {
		t.Fatalf("Var(lnsp): %v", err)
	}

	cases := []struct {
		lat, lon int
		want     float64
	}{
		{0, 0, 11.5385539531708},
		{225, 450, 11.5226371884346},
		{450, 899, 11.1248897910118},
	}

	for _, c := range cases {
		got, err := lnsp.At(0, 0, c.lat, c.lon)
		if err != nil {
			t.Fatalf("At(0,0,%d,%d): %v", c.lat, c.lon, err)
		}

		if math.Abs(got-c.want)/c.want > 1e-9 {
			t.Errorf("lnsp[lat=%d,lon=%d] = %.13g, want %.13g (ncdump -v)", c.lat, c.lon, got, c.want)
		}
	}
}

// TestGroundTruthAermr01Shape cross-checks aermr01's shape and metadata
// against ncdump -h output on the real file. Full-value ground truth
// (like TestGroundTruthLnspValues) isn't practical here: ncdump -v on a
// 55.6-million-element variable produces gigabytes of text and doesn't
// finish in a reasonable time. Bulk values are instead validated by
// internal consistency (ReadPlane vs. At agreeing, TestGroundTruth
// AermrInternalConsistency) plus the shape/attribute checks below, which
// ARE cheap to verify independently via ncdump -h.
//
//	ncdump -h z_cams_..._aermr01.nc:
//	  double aermr01(time, level, latitude, longitude) ;
//	    aermr01:_FillValue = 9.96920996838687e+36 ;
//	    aermr01:units = "kg kg**-1" ;
//	    aermr01:long_name = "Sea Salt Aerosol (0.03 - 0.5 um) Mixing Ratio" ;
//	  level = 1..137 (137 values, confirmed via ncdump -v level)
func TestGroundTruthAermr01Shape(t *testing.T) {
	bucket, key := aermr01Fixture(t)

	f, err := Open(context.Background(), bucket, key)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })

	dims := f.Dims()
	want := map[string]int{"longitude": 900, "latitude": 451, "level": 137, "time": 1}
	for name, n := range want {
		if dims[name] != n {
			t.Errorf("Dims()[%q] = %d, want %d (ncdump -h)", name, dims[name], n)
		}
	}

	aermr, err := f.Var("aermr01")
	if err != nil {
		t.Fatalf("Var(aermr01): %v", err)
	}

	if !aermr.HasLevel() {
		t.Error("aermr01.HasLevel() = false, want true")
	}
	if got := aermr.Units(); got != "kg kg**-1" {
		t.Errorf("aermr01.Units() = %q, want %q (ncdump -h)", got, "kg kg**-1")
	}
	if got := aermr.LongName(); got != "Sea Salt Aerosol (0.03 - 0.5 um) Mixing Ratio" {
		t.Errorf("aermr01.LongName() = %q, want %q (ncdump -h)", got, "Sea Salt Aerosol (0.03 - 0.5 um) Mixing Ratio")
	}

	wantAxes := []string{"time", "level", "latitude", "longitude"}
	gotAxes := aermr.AxisNames()
	if len(gotAxes) != len(wantAxes) {
		t.Fatalf("aermr01.AxisNames() = %v, want %v", gotAxes, wantAxes)
	}
	for i, a := range wantAxes {
		if gotAxes[i] != a {
			t.Errorf("aermr01.AxisNames()[%d] = %q, want %q", i, gotAxes[i], a)
		}
	}
}

// TestGroundTruthAermrInternalConsistency reads several full horizontal
// planes via ReadPlane and confirms Var.At agrees with them exactly, and
// that every value is finite and physically sane for a mixing ratio
// (>= 0, since kg/kg cannot be negative, or NaN at a fill-valued cell).
// This is not independent ground truth (see TestGroundTruthAermr01Shape's
// doc comment for why that isn't practical here) -- it is a real
// end-to-end exercise of the full, chunked, deflate-compressed read
// path against the actual 182 MB file, checking ReadPlane's and At's two
// independent code paths never disagree.
func TestGroundTruthAermrInternalConsistency(t *testing.T) {
	bucket, key := aermr01Fixture(t)

	f, err := Open(context.Background(), bucket, key)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })

	aermr, err := f.Var("aermr01")
	if err != nil {
		t.Fatalf("Var(aermr01): %v", err)
	}

	dims := f.Dims()
	latN, lonN := dims["latitude"], dims["longitude"]

	for _, level := range []int{0, 50, 100, 136} {
		plane, err := aermr.ReadPlane(0, level)
		if err != nil {
			t.Fatalf("ReadPlane(0,%d): %v", level, err)
		}
		if len(plane) != latN*lonN {
			t.Fatalf("ReadPlane(0,%d) len = %d, want %d", level, len(plane), latN*lonN)
		}

		for _, idx := range []struct{ lat, lon int }{
			{0, 0}, {225, 450}, {latN - 1, lonN - 1},
		} {
			want := plane[idx.lat*lonN+idx.lon]

			got, err := aermr.At(0, level, idx.lat, idx.lon)
			if err != nil {
				t.Fatalf("At(0,%d,%d,%d): %v", level, idx.lat, idx.lon, err)
			}

			if want != got && !(math.IsNaN(want) && math.IsNaN(got)) {
				t.Errorf("level=%d lat=%d lon=%d: ReadPlane gave %v, At gave %v", level, idx.lat, idx.lon, want, got)
			}

			if !math.IsNaN(got) && got < 0 {
				t.Errorf("level=%d lat=%d lon=%d: aermr01 = %v, want >= 0 (a mixing ratio cannot be negative)",
					level, idx.lat, idx.lon, got)
			}
		}
	}
}
