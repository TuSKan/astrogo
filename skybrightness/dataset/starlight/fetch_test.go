package starlight

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
)

func fetchSpec() GaiaBuild {
	return GaiaBuild{Order: 8, FainterThan: 6, Bands: []GaiaBand{GaiaJohnsonV()}}
}

// A scattered set of directions must become one query with one BETWEEN each,
// not a span covering everything between them. Asking for two pixels at
// opposite ends of the sky as a single range would aggregate the entire
// catalogue.
func TestADQLForPixelsIsADisjunction(t *testing.T) {
	t.Parallel()

	adql, err := fetchSpec().adqlForPixels([]int64{0, 400000, 786431})
	if err != nil {
		t.Fatalf("adqlForPixels: %v", err)
	}

	if n := strings.Count(adql, "source_id BETWEEN"); n != 3 {
		t.Errorf("got %d ranges, want one per pixel:\n%s", n, adql)
	}

	if n := strings.Count(adql, " OR "); n != 2 {
		t.Errorf("got %d disjunctions for 3 pixels, want 2:\n%s", n, adql)
	}

	// The last pixel's range must end at the last source_id it can hold.
	if !strings.Contains(adql, "GROUP BY hpx") {
		t.Errorf("missing the grouping:\n%s", adql)
	}

	if _, err := fetchSpec().adqlForPixels(nil); !errors.Is(err, ErrFetchSpec) {
		t.Errorf("no pixels: err = %v, want ErrFetchSpec", err)
	}
}

// The cut is the largest lever in the map, so it has to reach the query.
func TestMagnitudeCutReachesTheQuery(t *testing.T) {
	t.Parallel()

	adql, err := fetchSpec().ADQL(0, 9)
	if err != nil {
		t.Fatalf("ADQL: %v", err)
	}

	if !strings.Contains(adql, "phot_g_mean_mag > 6") {
		t.Errorf("the cut must appear in the query:\n%s", adql)
	}

	// NoMagnitudeCut emits no predicate at all rather than a permissive one,
	// because any predicate on phot_g_mean_mag also drops the sources that
	// have no G magnitude.
	every := fetchSpec()
	every.FainterThan = NoMagnitudeCut

	adql, err = every.ADQL(0, 9)
	if err != nil {
		t.Fatalf("ADQL: %v", err)
	}

	if strings.Contains(adql, "phot_g_mean_mag") {
		t.Errorf("NoMagnitudeCut must emit no predicate:\n%s", adql)
	}
}

// The zero value is rejected rather than read as either extreme. A silent
// G > 0 quietly discards the brightest sources; a silent "no cut" hands back a
// map whose brightest pixels are single stars. Both look like a working map,
// which is why neither is a default.
func TestMagnitudeCutIsRequired(t *testing.T) {
	t.Parallel()

	unset := GaiaBuild{Order: 8, Bands: []GaiaBand{GaiaJohnsonV()}}

	if _, err := unset.ADQL(0, 9); !errors.Is(err, ErrGaiaCut) {
		t.Errorf("unset cut: err = %v, want ErrGaiaCut", err)
	}
}

// Two cuts are two datasets. Sharing a cache between them would blend a
// background map with a total-light map, and afterwards neither is
// recoverable — so the cut, the band and the order are all in the key.
func TestCacheKeySeparatesDatasets(t *testing.T) {
	t.Parallel()

	base := fetchSpec()

	other := base
	other.FainterThan = 10

	coarse := base
	coarse.Order = 6

	everything := base
	everything.FainterThan = NoMagnitudeCut

	keys := map[string]string{
		"G>6":     base.cacheKey(),
		"G>10":    other.cacheKey(),
		"order 6": coarse.cacheKey(),
		"all":     everything.cacheKey(),
	}

	seen := map[string]string{}

	for name, key := range keys {
		if prior, dup := seen[key]; dup {
			t.Errorf("%s and %s share the cache key %q", name, prior, key)
		}

		seen[key] = name
	}

	if !strings.Contains(keys["G>6"], "o8") || !strings.Contains(keys["G>6"], "V") {
		t.Errorf("the key must name the order and band: %q", keys["G>6"])
	}
}

// A cached pixel must not be asked for again — that is the whole point of the
// cache, and the difference between a four-second call and a free one.
func TestWantedPixelsSkipsWhatIsHeld(t *testing.T) {
	t.Parallel()

	grid, err := coord.NewHEALPix(1 << 8)
	if err != nil {
		t.Fatalf("NewHEALPix: %v", err)
	}

	dirs := []coord.ICRS{
		coord.NewICRS(angle.Deg(10), angle.Deg(20)),
		coord.NewICRS(angle.Deg(200), angle.Deg(-40)),
		coord.NewICRS(angle.Deg(10), angle.Deg(20)), // a repeat
	}

	have := make([]float64, grid.NumPixels())

	// Nothing cached: two distinct pixels, the repeat collapsed.
	want := wantedPixels(grid, dirs, have)
	if len(want) != 2 {
		t.Fatalf("got %d pixels for 2 distinct directions, want 2", len(want))
	}

	if want[0] > want[1] {
		t.Errorf("pixels must ascend for the index: %v", want)
	}

	// Cache the first, and it drops out.
	have[grid.PixelOf(dirs[0].RA(), dirs[0].Dec())] = 1e-9

	if want = wantedPixels(grid, dirs, have); len(want) != 1 {
		t.Errorf("got %d pixels, want 1 after caching the other", len(want))
	}

	// Cache both and nothing is left to ask for, so Fetch makes no request.
	have[grid.PixelOf(dirs[1].RA(), dirs[1].Dec())] = 1e-9

	if want = wantedPixels(grid, dirs, have); len(want) != 0 {
		t.Errorf("got %d pixels, want none when everything is cached", len(want))
	}
}

// The cache tolerates gaps, which is what separates it from Load: a published
// map missing a pixel is malformed, a cache simply has not been asked yet.
func TestCacheRoundTripsSparsely(t *testing.T) {
	t.Parallel()

	values := make([]float64, 3072)
	values[7] = 1.25e-9
	values[100] = 4e-10

	var buf strings.Builder

	for pixel, v := range values {
		if v > 0 {
			fmt.Fprintf(&buf, "%d %.6e\n", pixel, v)
		}
	}

	restored := make([]float64, 3072)
	if err := parseCache(strings.NewReader("# bands: V\n"+buf.String()), restored, nil); err != nil {
		t.Fatalf("parseCache: %v", err)
	}

	for pixel, want := range values {
		if got := restored[pixel]; math.Abs(got-want) > 1e-15 {
			t.Errorf("pixel %d = %v, want %v", pixel, got, want)
		}
	}

	// Malformed lines are skipped rather than failing the whole cache: a
	// truncated write should cost the pixels it lost, not every pixel.
	partial := make([]float64, 3072)
	if err := parseCache(strings.NewReader("# bands: V\n7 1.25e-09\nbroken\n9 not-a-number\n1e9 5\n"), partial, nil); err != nil {
		t.Fatalf("parseCache: %v", err)
	}

	if partial[7] != 1.25e-9 {
		t.Errorf("the good line was lost: %v", partial[7])
	}
}

// A build that dies partway must keep what it finished.
//
// The order-8 build is 787 queries against a shared service. When one was
// throttled at chunk 360, the run discarded 360 chunks of completed work and
// the only way forward was to ask for all of it again, which is the behaviour
// that earns a throttle. Checkpointing means a stumble costs a minute and a
// resume asks only for what is missing.
func TestCacheRoundTripsCounts(t *testing.T) {
	t.Parallel()

	values := make([]float64, 3072)
	counts := make([]int64, 3072)
	values[7], counts[7] = 1.25e-9, 812
	values[100], counts[100] = 4e-10, 45

	var buf strings.Builder

	buf.WriteString("# bands: V\n")

	for pixel, v := range values {
		if v > 0 {
			fmt.Fprintf(&buf, "%d %.6e %d\n", pixel, v, counts[pixel])
		}
	}

	gotValues := make([]float64, 3072)
	gotCounts := make([]int64, 3072)

	if err := parseCache(strings.NewReader(buf.String()), gotValues, gotCounts); err != nil {
		t.Fatalf("parseCache: %v", err)
	}

	// The source total is what proves a whole-sky build tiled the sky, so it
	// has to survive a resume rather than counting only the final run.
	if gotCounts[7] != 812 || gotCounts[100] != 45 {
		t.Errorf("counts = %d, %d, want 812, 45", gotCounts[7], gotCounts[100])
	}

	if gotValues[7] != 1.25e-9 {
		t.Errorf("value = %v, want 1.25e-9", gotValues[7])
	}

	// A cache written before counts existed still loads; it just cannot report
	// the total.
	old := make([]float64, 3072)
	if err := parseCache(strings.NewReader("# bands: V\n7 1.250000e-09\n"), old, make([]int64, 3072)); err != nil {
		t.Fatalf("parseCache without counts: %v", err)
	}

	if old[7] != 1.25e-9 {
		t.Errorf("a two-column cache did not load: %v", old[7])
	}
}

// A chunk already held is not asked for again, which is what makes a resume
// cheap rather than a rerun.
func TestChunkIsCachedSkipsCompletedRanges(t *testing.T) {
	t.Parallel()

	build := fetchSpec()
	values := make([]float64, 100)

	if build.chunkIsCached(values, 0, 9) {
		t.Error("an empty range must not read as cached")
	}

	for i := range 10 {
		values[i] = 1e-9
	}

	if !build.chunkIsCached(values, 0, 9) {
		t.Error("a fully populated range must read as cached")
	}

	// One gap is enough to refetch: a partial chunk is not a chunk.
	values[5] = 0
	if build.chunkIsCached(values, 0, 9) {
		t.Error("a range with a hole must be refetched")
	}
}
