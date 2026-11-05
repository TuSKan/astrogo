package cams

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"math"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scigolib/hdf5"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// aodWhen is the hour every test here asks about; its key is where the
// fixture is served and cached.
var aodWhen = time.GoDate(2026, time.March, 20, 5, 0, 0, 0, time.LocationUTC)

// aodErrorPage is what a server answering with an error page under a 200 puts
// where the NetCDF file should be.
const aodErrorPage = "<html>503 Service Unavailable</html>"

// aodFixture is a NetCDF-4 file holding AODVariable at value in every cell, on
// synthFixture's 3 by 4 grid: the smallest file AOD550 reads end to end.
func aodFixture(tb testing.TB, value float64) []byte {
	tb.Helper()

	path := filepath.Join(tb.TempDir(), "aod.nc")

	fw, err := hdf5.CreateForWrite(path, hdf5.CreateTruncate)
	if err != nil {
		tb.Fatalf("CreateForWrite: %v", err)
	}

	writeDimScale(tb, fw, "/longitude", []float32{0, 90, 180, 270}, 0, "degrees_east", "longitude")
	writeDimScale(tb, fw, "/latitude", []float32{90, 0, -90}, 1, "degrees_north", "latitude")
	writeLevelDim(tb, fw, []int32{1}, 2)
	writeTimeDim(tb, fw, []int32{1078200}, 3)

	// aod550: (time, latitude, longitude) = (1, 3, 4), no level axis.
	aod, err := fw.CreateDataset("/"+AODVariable, hdf5.Float64, []uint64{1, 3, 4})
	if err != nil {
		tb.Fatalf("CreateDataset %s: %v", AODVariable, err)
	}

	mustWriteAttr(tb, aod, "_Netcdf4Coordinates", []int32{3, 1, 0})
	mustWriteAttr(tb, aod, "_FillValue", fillValue)
	mustWriteAttr(tb, aod, "units", "~")
	mustWriteAttr(tb, aod, "long_name", "Total Aerosol Optical Depth at 550nm (test)")

	data := make([]float64, 3*4)
	for i := range data {
		data[i] = value
	}

	if err := aod.Write(data); err != nil {
		tb.Fatalf("Write %s: %v", AODVariable, err)
	}

	if err := aod.Close(); err != nil {
		tb.Fatalf("Close %s writer: %v", AODVariable, err)
	}

	if err := fw.Close(); err != nil {
		tb.Fatalf("Close FileWriter: %v", err)
	}

	dir, err := remote.OpenFS(context.Background(), testutil.FileURL(tb, filepath.Dir(path)))
	if err != nil {
		tb.Fatalf("open fixture dir: %v", err)
	}

	b, err := fs.ReadFile(dir, "aod.nc")
	if err != nil {
		tb.Fatalf("read fixture: %v", err)
	}

	return b
}

// camsCacheAndSource points remote.CopernicusEODATA's cache and source at
// fresh directories and consents to its downloads, for the rest of t. It
// returns the cache with the key aodWhen's file caches under, and the source.
func camsCacheAndSource(t *testing.T) (cache remote.FS, key string, src remote.FS) {
	t.Helper()

	t.Cleanup(remote.Capture(remote.CopernicusEODATA).Restore)

	ctx := context.Background()

	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))

	srcURL := testutil.FileURL(t, t.TempDir())
	if err := remote.SetURL(remote.CopernicusEODATA, srcURL); err != nil {
		t.Fatalf("SetURL: %v", err)
	}

	src, err := remote.OpenFS(ctx, srcURL)
	if err != nil {
		t.Fatalf("open source: %v", err)
	}

	cache, prefix, err := remote.CacheDir(ctx, remote.CopernicusEODATA)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}

	remote.EnableDownloads(0, remote.CopernicusEODATA)

	return cache, prefix + AODKey(aodWhen), src
}

func putAOD(t *testing.T, fsys remote.FS, key string, b []byte) {
	t.Helper()

	if err := remote.WriteFile(context.Background(), fsys, key, bytes.NewReader(b)); err != nil {
		t.Fatalf("write %s: %v", key, err)
	}
}

func aodSite(t *testing.T) *coord.Geodetic {
	t.Helper()

	site, err := coord.NewGeodetic(angle.Deg(-70.4), angle.Deg(-24.6), 2635)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	return site
}

// AOD550 end to end, against a file served as the real ones are: fetched,
// cached, opened, and read at the site's cell.
func TestAOD550ReadsAServedFile(t *testing.T) {
	_, _, src := camsCacheAndSource(t)
	putAOD(t, src, AODKey(aodWhen), aodFixture(t, 0.15))

	got, err := AOD550(context.Background(), aodSite(t), aodWhen)
	if err != nil {
		t.Fatalf("AOD550: %v", err)
	}

	if got != 0.15 {
		t.Errorf("AOD550 = %g, want the file's 0.15", got)
	}
}

// A cached file damaged where it sits, while the source still serves the copy
// it was fetched from, is replaced in the call that finds it (#1014).
func TestAOD550ReplacesACorruptCachedFile(t *testing.T) {
	cache, key, src := camsCacheAndSource(t)
	putAOD(t, src, AODKey(aodWhen), aodFixture(t, 0.15))

	if _, err := AOD550(context.Background(), aodSite(t), aodWhen); err != nil {
		t.Fatalf("first call: %v", err)
	}

	putAOD(t, cache, key, []byte(aodErrorPage))

	got, err := AOD550(context.Background(), aodSite(t), aodWhen)
	if err != nil {
		t.Fatalf("call over a corrupt cached file: %v", err)
	}

	if got != 0.15 {
		t.Errorf("AOD550 = %g, want the source's 0.15", got)
	}
}

// A source serving a file that does not open is downloaded once and never
// cached, and the next call is refused without downloading it again. Neither
// error carries the registration advice: no credential fixes a bad file.
func TestAOD550DownloadsACorruptFileOnce(t *testing.T) {
	cache, key, src := camsCacheAndSource(t)
	putAOD(t, src, AODKey(aodWhen), []byte(aodErrorPage))

	_, err := AOD550(context.Background(), aodSite(t), aodWhen)
	if !errors.Is(err, ErrAOD) || errors.Is(err, remote.ErrRejectedSource) {
		t.Fatalf("first call: err %v, want ErrAOD from opening the file", err)
	}

	if strings.Contains(err.Error(), RegistrationAdvice) {
		t.Errorf("a file that does not open is answered with credential advice: %v", err)
	}

	if _, statErr := fs.Stat(cache, key); !errors.Is(statErr, fs.ErrNotExist) {
		t.Errorf("the corrupt download was cached (Stat: %v)", statErr)
	}

	_, err = AOD550(context.Background(), aodSite(t), aodWhen)
	if !errors.Is(err, remote.ErrRejectedSource) || !errors.Is(err, ErrAOD) {
		t.Fatalf("second call, source unchanged: err %v, want ErrAOD and remote.ErrRejectedSource", err)
	}

	if strings.Contains(err.Error(), RegistrationAdvice) {
		t.Errorf("a refused file is answered with credential advice: %v", err)
	}
}

// A site with no cell is the caller's error, found before the file is touched:
// inside the read, it would have condemned a good cached file.
func TestAOD550KeepsTheFileOverANonFiniteSite(t *testing.T) {
	cache, key, src := camsCacheAndSource(t)
	putAOD(t, src, AODKey(aodWhen), aodFixture(t, 0.15))

	if _, err := AOD550(context.Background(), aodSite(t), aodWhen); err != nil {
		t.Fatalf("first call: %v", err)
	}

	var site coord.Geodetic
	site.FromUnitVector(vector.V3(math.NaN(), math.NaN(), math.NaN()))

	if _, err := AOD550(context.Background(), &site, aodWhen); !errors.Is(err, ErrAOD) {
		t.Fatalf("AOD550 at a NaN site: err %v, want ErrAOD", err)
	}

	if _, err := fs.Stat(cache, key); err != nil {
		t.Errorf("the cached file is gone after a call with a bad site: %v", err)
	}
}
