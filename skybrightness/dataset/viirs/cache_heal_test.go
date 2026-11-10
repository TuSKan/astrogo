package viirs_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/skybrightness/dataset/viirs"
	"github.com/TuSKan/astrogo/time"
)

// healYear is the composite every test here opens.
const healYear = viirs.EarliestYear

// errorPage is what a server answering with an error page under a 200 puts
// where an archive or its TIFF should be.
const errorPage = "<html>503 Service Unavailable</html>"

var (
	// archiveFile is the archive's name at the source and in the cache.
	archiveFile = fmt.Sprintf("viirs_%d_raw.zip", healYear)

	// tiffFile is the TIFF entry in the archive. The composite is extracted
	// beside the archive, at tiffKey.
	tiffFile = fmt.Sprintf("viirs_%d_raw.tif", healYear)
)

// tiffKey is where the composite in the archive cached at archiveKey is
// extracted: the archive's own directory, the endpoint's prefix.
func tiffKey(archiveKey string) string { return path.Join(path.Dir(archiveKey), tiffFile) }

// pixels are the composite's four samples, west to east and north to south on
// a 2 by 2 whole-sky grid.
var pixels = [4]float64{1, 2, 3, 4}

// compositeTIFF is a 2 by 2 float64 GeoTIFF covering the whole sky in
// 180-degree pixels, holding px: the smallest file the composite reader
// decodes.
func compositeTIFF(px [4]float64) []byte {
	type entry struct {
		tag, typ     uint16
		count, value uint32
	}

	const (
		typeShort  = 3
		typeLong   = 4
		typeDouble = 12
	)

	scale := []float64{180, 90, 0}
	tie := []float64{0, 0, 0, -180, 90, 0}

	entries := []entry{
		{256, typeLong, 1, 2},     // ImageWidth
		{257, typeLong, 1, 2},     // ImageLength
		{258, typeShort, 1, 64},   // BitsPerSample
		{259, typeShort, 1, 1},    // Compression: none
		{273, typeLong, 1, 0},     // StripOffsets, set below
		{277, typeShort, 1, 1},    // SamplesPerPixel
		{278, typeLong, 1, 2},     // RowsPerStrip
		{279, typeLong, 1, 32},    // StripByteCounts
		{339, typeShort, 1, 3},    // SampleFormat: IEEE float
		{33550, typeDouble, 3, 0}, // ModelPixelScale, set below
		{33922, typeDouble, 6, 0}, // ModelTiepoint, set below
	}

	ifdEnd := 8 + 2 + 12*len(entries) + 4
	scaleAt := ifdEnd
	tieAt := scaleAt + 8*len(scale)
	dataAt := tieAt + 8*len(tie)

	entries[4].value = uint32(dataAt)
	entries[9].value = uint32(scaleAt)
	entries[10].value = uint32(tieAt)

	b := make([]byte, dataAt+8*len(px))
	copy(b, "II")
	binary.LittleEndian.PutUint16(b[2:], 42)
	binary.LittleEndian.PutUint32(b[4:], 8)
	binary.LittleEndian.PutUint16(b[8:], uint16(len(entries)))

	for i, e := range entries {
		o := 10 + 12*i
		binary.LittleEndian.PutUint16(b[o:], e.tag)
		binary.LittleEndian.PutUint16(b[o+2:], e.typ)
		binary.LittleEndian.PutUint32(b[o+4:], e.count)
		binary.LittleEndian.PutUint32(b[o+8:], e.value)
	}

	putDoubles := func(at int, v []float64) {
		for i, x := range v {
			binary.LittleEndian.PutUint64(b[at+8*i:], math.Float64bits(x))
		}
	}

	putDoubles(scaleAt, scale)
	putDoubles(tieAt, tie)
	putDoubles(dataAt, px[:])

	return b
}

// archive zips one entry named tiffFile holding tiff.
func archive(t *testing.T, tiff []byte) []byte {
	t.Helper()

	var b bytes.Buffer

	zw := zip.NewWriter(&b)

	w, err := zw.Create(tiffFile)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := w.Write(tiff); err != nil {
		t.Fatal(err)
	}

	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	return b.Bytes()
}

// archiveSource is an HTTP source serving the archive, whose content and ETag
// a test can change, counting the requests that fetch its body.
type archiveSource struct {
	mu        sync.Mutex
	body      []byte
	etag      string
	downloads atomic.Int32
}

func (a *archiveSource) serve(body []byte, etag string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.body, a.etag = body, etag
}

// viirsCacheAndSource points remote.VIIRSAnnual at an archiveSource serving
// body and its cache at a fresh directory, and consents to its downloads, for
// the rest of t. It returns the data directory, the archive's cache key, and
// the source.
func viirsCacheAndSource(t *testing.T, body []byte) (cache remote.FS, archiveKey string, src *archiveSource) {
	t.Helper()

	t.Cleanup(remote.Capture(remote.VIIRSAnnual).Restore)

	src = &archiveSource{}
	src.serve(body, "v1")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimPrefix(r.URL.Path, "/") != archiveFile {
			http.NotFound(w, r)

			return
		}

		src.mu.Lock()
		body, etag := src.body, src.etag
		src.mu.Unlock()

		w.Header().Set("ETag", `"`+etag+`"`)

		// The backend's one-byte ranged probe is metadata, like its HEAD.
		if r.Method == http.MethodGet && r.Header.Get("Range") != "bytes=0-0" {
			src.downloads.Add(1)
		}

		http.ServeContent(w, r, archiveFile, time.GoTime{}, bytes.NewReader(body))
	}))
	t.Cleanup(srv.Close)

	ctx := context.Background()

	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))

	if err := remote.SetURL(remote.VIIRSAnnual, srv.URL+"/"); err != nil {
		t.Fatalf("SetURL: %v", err)
	}

	cache, prefix, err := remote.CacheDir(ctx, remote.VIIRSAnnual)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}

	remote.EnableDownloads(0, remote.VIIRSAnnual)

	return cache, prefix + archiveFile, src
}

func put(t *testing.T, fsys remote.FS, key string, b []byte) {
	t.Helper()

	if err := remote.WriteFile(context.Background(), fsys, key, bytes.NewReader(b)); err != nil {
		t.Fatalf("write %s: %v", key, err)
	}
}

func isCached(t *testing.T, fsys remote.FS, key string) bool {
	t.Helper()

	_, err := fs.Stat(fsys, key)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Stat %s: %v", key, err)
	}

	return err == nil
}

// open opens healYear's composite and reads one sample of it, closing it again.
func open(t *testing.T) error {
	t.Helper()

	return openReading(t, pixels[0])
}

// openReading is open, wanting the north-west pixel to read want.
func openReading(t *testing.T, want float64) error {
	t.Helper()

	r, closer, err := viirs.Open(context.Background(), healYear)
	if err != nil {
		return fmt.Errorf("viirs.Open: %w", err)
	}

	defer func() { _ = closer.Close() }()

	got, err := r.RadianceAt(angle.Deg(-90), angle.Deg(45))
	if err != nil {
		t.Fatalf("RadianceAt: %v", err)
	}

	// At the pixel's center the neighbors' weights are zero, but only up to the
	// rounding of the sample position, which FMA can change on arm64.
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("RadianceAt the north-west pixel's center = %g, want %g", got, want)
	}

	return nil
}

// The composite end to end: downloaded, extracted, decoded and sampled.
func TestOpenReadsAServedComposite(t *testing.T) {
	_, _, src := viirsCacheAndSource(t, archive(t, compositeTIFF(pixels)))

	if err := open(t); err != nil {
		t.Fatalf("Open: %v", err)
	}

	if n := src.downloads.Load(); n != 1 {
		t.Errorf("%d downloads, want 1", n)
	}
}

// A cached archive damaged where it sits, while the source still serves the
// copy it was fetched from, is replaced in the call that finds it (#1014).
func TestOpenReplacesACorruptCachedArchive(t *testing.T) {
	cache, archiveKey, src := viirsCacheAndSource(t, archive(t, compositeTIFF(pixels)))

	if err := open(t); err != nil {
		t.Fatalf("first Open: %v", err)
	}

	if err := remote.RemoveFile(context.Background(), cache, tiffKey(archiveKey)); err != nil {
		t.Fatalf("remove the extracted TIFF: %v", err)
	}

	put(t, cache, archiveKey, []byte(errorPage))

	if err := open(t); err != nil {
		t.Fatalf("Open over a corrupt cached archive: %v", err)
	}

	if n := src.downloads.Load(); n != 2 {
		t.Errorf("%d downloads, want 2", n)
	}
}

// A damaged extracted TIFF is extracted again from the cached archive, which
// is not downloaded again: at a gigabyte, that is the difference that matters.
func TestOpenReextractsACorruptTIFFWithoutDownloading(t *testing.T) {
	cache, archiveKey, src := viirsCacheAndSource(t, archive(t, compositeTIFF(pixels)))

	if err := open(t); err != nil {
		t.Fatalf("first Open: %v", err)
	}

	put(t, cache, tiffKey(archiveKey), []byte(errorPage))

	if err := open(t); err != nil {
		t.Fatalf("Open over a corrupt extracted TIFF: %v", err)
	}

	if n := src.downloads.Load(); n != 1 {
		t.Errorf("%d downloads, want 1: the archive was downloaded again", n)
	}
}

// A source serving something other than an archive is downloaded once and
// never cached, and the next call is refused without downloading it again.
func TestOpenDownloadsACorruptArchiveOnce(t *testing.T) {
	cache, archiveKey, src := viirsCacheAndSource(t, []byte(errorPage))

	if err := open(t); err == nil || errors.Is(err, remote.ErrRejectedSource) {
		t.Fatalf("first Open: err %v, want the archive's error", err)
	}

	if isCached(t, cache, archiveKey) {
		t.Error("the corrupt download was cached")
	}

	if err := open(t); !errors.Is(err, remote.ErrRejectedSource) {
		t.Fatalf("second Open, source unchanged: err %v, want remote.ErrRejectedSource", err)
	}

	if n := src.downloads.Load(); n != 1 {
		t.Errorf("%d downloads, want 1", n)
	}

	src.serve(archive(t, compositeTIFF(pixels)), "v2")

	if err := open(t); err != nil {
		t.Fatalf("Open after the source changed: %v", err)
	}
}

// A TIFF with no record of the archive it came from, over an archive whose TIFF
// does not decode: the first is replaced by an extraction, the archive is
// refused, and no TIFF of either is left behind.
func TestOpenLeavesNoTIFFFromARefusedArchive(t *testing.T) {
	cache, archiveKey, _ := viirsCacheAndSource(t, archive(t, []byte(errorPage)))
	put(t, cache, tiffKey(archiveKey), []byte(errorPage))

	if err := open(t); err == nil || errors.Is(err, remote.ErrRejectedSource) {
		t.Fatalf("Open: err %v, want the decode error", err)
	}

	if isCached(t, cache, archiveKey) {
		t.Error("the archive was cached")
	}

	if isCached(t, cache, tiffKey(archiveKey)) {
		t.Error("a TIFF that did not decode was left in the cache")
	}
}

// An archive whose TIFF does not decode is refused with it, since the TIFF was
// just extracted from it: neither is left in the cache, and the archive is not
// downloaded again while the source still serves it.
func TestOpenRefusesAnArchiveWhoseTIFFDoesNotDecode(t *testing.T) {
	cache, archiveKey, src := viirsCacheAndSource(t, archive(t, []byte(errorPage)))

	if err := open(t); err == nil || errors.Is(err, remote.ErrRejectedSource) {
		t.Fatalf("first Open: err %v, want the decode error", err)
	}

	if isCached(t, cache, archiveKey) {
		t.Error("the archive was cached")
	}

	if isCached(t, cache, tiffKey(archiveKey)) {
		t.Error("the TIFF that did not decode was left in the cache")
	}

	if err := open(t); !errors.Is(err, remote.ErrRejectedSource) {
		t.Fatalf("second Open, source unchanged: err %v, want remote.ErrRejectedSource", err)
	}

	if n := src.downloads.Load(); n != 1 {
		t.Errorf("%d downloads, want 1", n)
	}
}

// The cached archive and the TIFF extracted from it both damaged: the TIFF no
// longer matches the archive's record, extracting it again finds the archive
// unreadable, and the archive is replaced in the same call.
func TestOpenReplacesADamagedArchiveAndTIFF(t *testing.T) {
	cache, archiveKey, src := viirsCacheAndSource(t, archive(t, compositeTIFF(pixels)))

	if err := open(t); err != nil {
		t.Fatalf("first Open: %v", err)
	}

	put(t, cache, tiffKey(archiveKey), []byte(errorPage))
	put(t, cache, archiveKey, []byte(errorPage))

	if err := open(t); err != nil {
		t.Fatalf("Open over a damaged archive and TIFF: %v", err)
	}

	if n := src.downloads.Load(); n != 2 {
		t.Errorf("%d downloads, want 2", n)
	}
}

// A year reprocessed upstream is read from the new archive, not from the
// composite extracted from the old one (#1015). Before, the new archive was
// downloaded, a gigabyte in practice, and the old pixels served.
func TestOpenReadsAReprocessedYear(t *testing.T) {
	_, _, src := viirsCacheAndSource(t, archive(t, compositeTIFF(pixels)))

	if err := open(t); err != nil {
		t.Fatalf("first Open: %v", err)
	}

	reprocessed := [4]float64{10, 20, 30, 40}
	src.serve(archive(t, compositeTIFF(reprocessed)), "v2")

	if err := openReading(t, reprocessed[0]); err != nil {
		t.Fatalf("Open after upstream reprocessed the year: %v", err)
	}

	if n := src.downloads.Load(); n != 2 {
		t.Errorf("%d downloads, want 2", n)
	}
}

// The composite is reused while the archive it came from is the one cached: a
// second Open does not extract it again, a cost of hundreds of megabytes per
// call at full size. Extracted again, it would be written anew and its
// modification time would move.
func TestOpenReusesTheCompositeOfTheCachedArchive(t *testing.T) {
	cache, archiveKey, _ := viirsCacheAndSource(t, archive(t, compositeTIFF(pixels)))

	if err := open(t); err != nil {
		t.Fatalf("first Open: %v", err)
	}

	before, err := fs.Stat(cache, tiffKey(archiveKey))
	if err != nil {
		t.Fatalf("Stat the composite: %v", err)
	}

	if err := open(t); err != nil {
		t.Fatalf("second Open: %v", err)
	}

	after, err := fs.Stat(cache, tiffKey(archiveKey))
	if err != nil {
		t.Fatalf("Stat the composite again: %v", err)
	}

	if !after.ModTime().Equal(before.ModTime()) {
		t.Errorf("the composite was extracted again for an unchanged archive: modified %v, then %v",
			before.ModTime(), after.ModTime())
	}
}

// The composite is extracted beside its archive, under the endpoint's prefix,
// with the record of the archive copy it came from. It used to be written at
// the root of the data directory (#1015).
func TestOpenExtractsBesideTheArchive(t *testing.T) {
	cache, archiveKey, _ := viirsCacheAndSource(t, archive(t, compositeTIFF(pixels)))

	if err := open(t); err != nil {
		t.Fatalf("Open: %v", err)
	}

	if !isCached(t, cache, tiffKey(archiveKey)) {
		t.Errorf("no composite at %s, beside the archive", tiffKey(archiveKey))
	}

	if !isCached(t, cache, tiffKey(archiveKey)+".archive") {
		t.Errorf("no record of the archive beside %s", tiffKey(archiveKey))
	}

	if isCached(t, cache, tiffFile) {
		t.Errorf("a composite was written at the root of the data directory, %s", tiffFile)
	}
}

// A composite an earlier version left at the root of the data directory is
// removed once the composite is extracted where it belongs: nothing reads it
// any more, and at full size it is hundreds of megabytes.
func TestOpenRemovesTheCompositeOlderVersionsLeftAtTheRoot(t *testing.T) {
	cache, _, _ := viirsCacheAndSource(t, archive(t, compositeTIFF(pixels)))
	put(t, cache, tiffFile, compositeTIFF(pixels))

	if err := open(t); err != nil {
		t.Fatalf("Open: %v", err)
	}

	if isCached(t, cache, tiffFile) {
		t.Errorf("the composite older versions left at the root, %s, is still there", tiffFile)
	}
}
