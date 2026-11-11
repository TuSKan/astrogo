package starlight

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
)

// between finds each source_id range a query asks about: one per pixel in a
// Fetch query, one covering a whole chunk in a BuildFromGaia or RunChunk one.
var between = regexp.MustCompile(`source_id BETWEEN (\d+) AND (\d+)`)

// fakeArchive stands in for the Gaia TAP service: it answers each query with
// one CSV row per pixel the query asks for, read from the query's own source_id
// ranges, with every band column set to flux(pixel). refuse, when it returns
// true for a call number, answers that call with a 400 instead.
//
// The cache is pointed at a directory of the test's own, so nothing another
// run left there can answer for the archive.
func fakeArchive(
	t *testing.T, order int, bands []string,
	flux func(pixel int64) string, refuse func(call int32) bool,
) *atomic.Int32 {
	t.Helper()

	t.Cleanup(remote.Capture().Restore)
	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))

	divisor := sourceIDDivisor(order)

	var calls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if refuse != nil && refuse(n) {
			http.Error(w, "refused", http.StatusBadRequest)

			return
		}

		_ = r.ParseForm()

		var b strings.Builder

		b.WriteString("hpx,n,ncolour")

		for _, band := range bands {
			b.WriteString("," + strings.ToLower(columnName(band)))
		}

		b.WriteString("\n")

		for _, m := range between.FindAllStringSubmatch(r.Form.Get("QUERY"), -1) {
			lo, _ := strconv.ParseInt(m[1], 10, 64)
			hi, _ := strconv.ParseInt(m[2], 10, 64)

			for pixel := lo / divisor; pixel <= hi/divisor; pixel++ {
				fmt.Fprintf(&b, "%d,10,10", pixel)

				for range bands {
					b.WriteString("," + flux(pixel))
				}

				b.WriteString("\n")
			}
		}

		_, _ = w.Write([]byte(b.String()))
	}))
	t.Cleanup(srv.Close)

	if err := remote.SetURL(remote.GaiaTAP, srv.URL); err != nil {
		t.Fatalf("SetURL: %v", err)
	}

	return &calls
}

// band is a Gaia band with no color term, so the flux the archive answers is
// the flux the map holds, scaled.
func band(name string) GaiaBand { return GaiaBand{Name: name, FluxToRadiance: 1} }

// A multi-band build resumes every band, not just the first (#1077).
//
// The first band's partial map is seeded complete, as a finished single-band
// build leaves it. Before #1077 the build read only that file, skipped every
// chunk it held, made no query and returned the second band all zero, with no
// error.
func TestAMultiBandBuildResumesEveryBand(t *testing.T) {
	const order = 1

	npix := pixelsPerOrder(order)

	calls := fakeArchive(t, order, []string{"V", "B"}, func(int64) string { return "5" }, nil)

	build := GaiaBuild{Order: order, Chunk: int(npix), Bands: []GaiaBand{band("V"), band("B")}, Endpoint: remote.GaiaTAP}

	fsys, prefix, err := remote.CacheDir(context.Background(), remote.GaiaTAP)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}

	seed := make([]float64, npix)
	for i := range seed {
		seed[i] = 1e-9
	}

	if err := writeCache(context.Background(), fsys, path.Join(prefix, build.cacheKeyFor("V")), "V", seed, nil); err != nil {
		t.Fatalf("seed the V cache: %v", err)
	}

	m, _, err := BuildFromGaia(context.Background(), build)
	if err != nil {
		t.Fatalf("BuildFromGaia: %v", err)
	}

	if calls.Load() == 0 {
		t.Error("the build made no query, though only one of its two bands was cached")
	}

	for pixel := range npix {
		if v, err := m.Pixel("B", pixel); err != nil || !(v > 0) {
			t.Fatalf("band B pixel %d is %v (%v); the chunk was skipped on band V's word", pixel, v, err)
		}
	}

	// And the second band now has a partial map of its own, so the next
	// resume skips the chunk for the right reason.
	b := make([]float64, npix)
	if err := readCache(context.Background(), fsys, path.Join(prefix, build.cacheKeyFor("B")), b, nil); err != nil {
		t.Fatalf("the build left no cache for band B: %v", err)
	}

	if b[0] <= 0 {
		t.Error("band B's cache holds no value for pixel 0")
	}
}

// A NaN or infinite flux in an archive answer is no flux (#1078).
//
// The map's own readers refuse non-finite values since #1030; before #1078
// this third reader let NaN and +Inf straight into the map, an infinite one
// being a direction of the sky infinitely bright.
func TestAGaiaAnswerCannotPutANonFiniteFluxInTheMap(t *testing.T) {
	const order = 1

	bad := map[int64]string{0: "NaN", 1: "Infinity", 2: "+Inf", 3: "1e400"}

	fakeArchive(t, order, []string{"V"}, func(pixel int64) string {
		if s, ok := bad[pixel]; ok {
			return s
		}

		return "5"
	}, nil)

	m, _, err := RunChunk(context.Background(),
		GaiaBuild{Order: order, Bands: []GaiaBand{band("V")}, Endpoint: remote.GaiaTAP}, 0, 4)
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}

	for pixel, s := range bad {
		v, err := m.Pixel("V", pixel)
		if err != nil {
			t.Fatalf("Pixel(%d): %v", pixel, err)
		}

		if math.IsNaN(v) || math.IsInf(v, 0) || v != 0 {
			t.Errorf("a flux of %q put %v in pixel %d; it is no flux", s, v, pixel)
		}
	}

	if v, err := m.Pixel("V", 4); err != nil || !(v > 0) || math.IsInf(v, 0) {
		t.Errorf("the control pixel holds %v (%v)", v, err)
	}
}

// The color-recovery columns are read the same way: a non-finite one adds
// nothing to a pixel accumulate has accepted (#1078).
func TestRecoverColorlessIgnoresANonFiniteColumn(t *testing.T) {
	t.Parallel()

	v := GaiaJohnsonV()
	build := GaiaBuild{Bands: []GaiaBand{v}}
	col := strings.ToLower(columnName(v.Name))

	for name, row := range map[string][3]string{
		"NaN total":    {"NaN", "1", "0.8"},
		"infinite sum": {"+Inf", "1", "0.8"},
		"NaN color":    {"2", "1", "NaN"},
	} {
		rows, err := newSyncRows(strings.NewReader(
			col + "_all," + col + "_col," + col + "_mc\n" + strings.Join(row[:], ",") + "\n"))
		if err != nil {
			t.Fatalf("%s: newSyncRows: %v", name, err)
		}

		if !rows.Next() {
			t.Fatalf("%s: no row", name)
		}

		if got := build.recoverColorless(v, rows); got != 0 {
			t.Errorf("%s: recoverColorless = %v, want 0", name, got)
		}
	}
}

// The cache writer leaves out a value its own reader would refuse (#1078).
//
// parseCache drops a non-finite value since #1030, so a NaN written to the
// cache only made that pixel be fetched again on every call.
func TestWriteCacheLeavesOutNonFiniteValues(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	fsys, err := remote.OpenFS(ctx, testutil.FileURL(t, t.TempDir()))
	if err != nil {
		t.Fatalf("OpenFS: %v", err)
	}

	values := []float64{1e-9, math.NaN(), math.Inf(1), 2e-9, math.Inf(-1)}

	if err := writeCache(ctx, fsys, "cache.txt", "V", values, nil); err != nil {
		t.Fatalf("writeCache: %v", err)
	}

	f, err := remote.Open(ctx, fsys, "cache.txt")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = f.Close() }()

	var body strings.Builder
	if _, err := io.Copy(&body, f); err != nil {
		t.Fatalf("read: %v", err)
	}

	text := body.String()
	if strings.Contains(text, "NaN") || strings.Contains(text, "Inf") {
		t.Errorf("the cache holds a non-finite value:\n%s", text)
	}

	rows := 0

	for line := range strings.SplitSeq(text, "\n") {
		if line != "" && !strings.HasPrefix(line, "#") {
			rows++
		}
	}

	if rows != 2 {
		t.Errorf("the cache holds %d rows; want the 2 finite, positive values:\n%s", rows, text)
	}
}

// Fetch keeps the batches it completed when a later one fails (#1079).
//
// It wrote its cache only after the last batch, so a failure in batch two
// discarded batch one, and a retry asked the archive for it again.
// BuildFromGaia had been fixed for exactly this; Fetch had not.
func TestFetchKeepsCompletedBatchesWhenALaterOneFails(t *testing.T) {
	const order = 5

	npix := pixelsPerOrder(order)

	calls := fakeArchive(t, order, []string{"V"}, func(int64) string { return "5" },
		func(call int32) bool { return call > 1 })

	spec := GaiaBuild{Order: order, Bands: []GaiaBand{band("V")}, Endpoint: remote.GaiaTAP}

	// More directions than one batch holds, spread over the sphere.
	dirs := make([]coord.ICRS, 0, 900)

	for i := range 900 {
		dec := -89.0 + 178.0*float64(i)/899
		dirs = append(dirs, coord.NewICRS(angle.Deg(math.Mod(float64(i)*137.5, 360)), angle.Deg(dec)))
	}

	if _, err := Fetch(context.Background(), spec, dirs...); err == nil {
		t.Fatal("Fetch succeeded although the archive refused its second batch")
	}

	if calls.Load() < 2 {
		t.Fatalf("Fetch made %d queries; the test needs a second batch to fail", calls.Load())
	}

	fsys, prefix, err := remote.CacheDir(context.Background(), remote.GaiaTAP)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}

	cached := make([]float64, npix)
	if err := readCache(context.Background(), fsys, path.Join(prefix, spec.cacheKeyFor("V")), cached, nil); err != nil {
		t.Fatalf("the failed fetch left no cache: %v", err)
	}

	held := 0

	for _, v := range cached {
		if v > 0 {
			held++
		}
	}

	if held != fetchBatch {
		t.Errorf("the cache holds %d pixels after the failed fetch; the completed first batch was %d",
			held, fetchBatch)
	}
}
