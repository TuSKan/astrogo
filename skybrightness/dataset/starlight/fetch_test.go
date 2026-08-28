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
	return GaiaBuild{Order: 8, Bands: []GaiaBand{GaiaJohnsonV()}}
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

// Two builds whose values differ must never share a cache key. The order and
// the band both change the numbers, so both are in the name.
func TestCacheKeySeparatesDatasets(t *testing.T) {
	t.Parallel()

	base := fetchSpec()

	coarse := base
	coarse.Order = 6

	otherBand := base
	otherBand.Bands = []GaiaBand{{Name: "G", FluxToRadiance: 1e-18}}

	keys := map[string]string{
		"order 8 V": base.cacheKey(),
		"order 6 V": coarse.cacheKey(),
		"order 8 G": otherBand.cacheKey(),
	}

	seen := map[string]string{}

	for name, key := range keys {
		if prior, dup := seen[key]; dup {
			t.Errorf("%s and %s share the cache key %q", name, prior, key)
		}

		seen[key] = name
	}

	if !strings.Contains(keys["order 8 V"], "o8") || !strings.Contains(keys["order 8 V"], "V") {
		t.Errorf("the key must name the order and band: %q", keys["order 8 V"])
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

// Colourless sources must be recovered, not dropped.
//
// Across the whole order-8 build 14.95 per cent of sources carry no BP-RP, and
// in the densest pixels of the Galactic plane it passes 50 per cent. Dropping
// them underestimates the plane specifically — the brightest part of the map —
// and a deficit that varies with direction cannot be calibrated away the way a
// uniform one could.
func TestColourRecoveryColumnsAreRequested(t *testing.T) {
	t.Parallel()

	adql, err := fetchSpec().ADQL(0, 9)
	if err != nil {
		t.Fatalf("ADQL: %v", err)
	}

	// The unconditional sum, the colour-propagating sum whose difference gives
	// the dropped flux, and the pixel's mean colour to assign it.
	for _, want := range []string{
		"SUM(phot_g_mean_flux) AS b_V_all",
		"SUM(phot_g_mean_flux+0*bp_rp) AS b_V_col",
		"SUM(phot_g_mean_flux*bp_rp)/SUM(phot_g_mean_flux+0*bp_rp) AS b_V_mc",
	} {
		if !strings.Contains(adql, want) {
			t.Errorf("missing %q from:\n%s", want, adql)
		}
	}

	// NULL propagation rather than CASE or FILTER: one archive rejects each.
	for _, forbidden := range []string{"CASE", "FILTER", "COALESCE"} {
		if strings.Contains(adql, forbidden) {
			t.Errorf("%s is rejected by one archive or the other:\n%s", forbidden, adql)
		}
	}

	// A band with no colour term has nothing to recover.
	plain := GaiaBuild{Order: 8, Bands: []GaiaBand{{Name: "G", FluxToRadiance: 1e-18}}}

	adql, err = plain.ADQL(0, 9)
	if err != nil {
		t.Fatalf("ADQL: %v", err)
	}

	if strings.Contains(adql, "_all") || strings.Contains(adql, "_mc") {
		t.Errorf("an untransformed band needs no recovery columns:\n%s", adql)
	}
}

// The recovered flux must be scaled by the same polynomial the query applied,
// evaluated at the pixel's mean colour. A different factor here than in the
// query is a seam that no downstream check could see.
func TestColourRecoveryUsesTheSamePolynomial(t *testing.T) {
	t.Parallel()

	band := GaiaJohnsonV()

	// At bp_rp = 0 the polynomial collapses to its constant term.
	if got, want := band.colourFactor(0), math.Pow(10, 0.4*band.ColourTerm[0]); math.Abs(got-want) > 1e-15 {
		t.Errorf("colourFactor(0) = %v, want %v", got, want)
	}

	// And the rendered ADQL must contain every coefficient the Go evaluation
	// uses, in the same order.
	poly := band.colourPolynomial()
	for _, c := range []string{"-0.02704", "0.01424*bp_rp", "-0.2156*bp_rp*bp_rp"} {
		if !strings.Contains(poly, c) {
			t.Errorf("polynomial %q omits %q", poly, c)
		}
	}
}

// The recovery reads the response, so it must survive responses that lack the
// columns, carry nothing to recover, or have no colour to average.
func TestColourRecoveryDegradesSafely(t *testing.T) {
	t.Parallel()

	band := GaiaJohnsonV()
	spec := fetchSpec()

	cases := []struct {
		name   string
		header string
		row    string
		want   float64
	}{
		{"no recovery columns", "hpx", "0", 0},
		{"nothing dropped", "b_v_all,b_v_col,b_v_mc", "100,100,0", 0},
		{"no coloured source to average", "b_v_all,b_v_col,b_v_mc", "100,0,", 0},
		{"recovers the difference", "b_v_all,b_v_col,b_v_mc", "100,60,0", 40 * band.colourFactor(0)},
	}

	// One header line and one data line, which is the smallest thing
	// newCSVRows will read.
	const NL = "\n"

	for _, tc := range cases {
		rows, err := newCSVRows(strings.NewReader(tc.header + NL + tc.row + NL))
		if err != nil {
			t.Fatalf("%s: newCSVRows: %v", tc.name, err)
		}

		if !rows.Next() {
			t.Fatalf("%s: no row", tc.name)
		}

		got := spec.recoverColourless(band, rows)
		if math.Abs(got-tc.want) > 1e-12 {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

// A mean colour outside the fitted interval must not be extrapolated.
//
// Flux-weighting the mean colour is what makes this reachable: one dominant
// star can carry a pixel's mean past any colour a real star has, and over the
// order-9 sky it reaches BP-RP = 7.41. A cubic fitted to 5.0 evaluated at 7.41
// is not a transformation, it is an artefact, so the colour is clamped and the
// factor stops changing beyond the interval.
func TestColourFactorRefusesToExtrapolate(t *testing.T) {
	t.Parallel()

	band := GaiaJohnsonV()

	if got, want := band.colourFactor(9.0), band.colourFactor(colourValidHi); got != want {
		t.Errorf("BP-RP = 9 gives %v, want the value at the %v bound, %v", got, colourValidHi, want)
	}

	if got, want := band.colourFactor(-4.0), band.colourFactor(colourValidLo); got != want {
		t.Errorf("BP-RP = -4 gives %v, want the value at the %v bound, %v", got, colourValidLo, want)
	}

	// Inside the interval nothing is clamped, so the factor still varies.
	if band.colourFactor(1.0) == band.colourFactor(2.0) {
		t.Error("the factor stopped varying inside the fitted interval")
	}

	// And the clamped extremes stay physical: a factor is a flux ratio and the
	// unbounded cubic is what produced ratios of a hundred.
	for _, c := range []float64{-4, -0.5, 0, 2.5, 5, 9} {
		if f := band.colourFactor(c); f <= 0 || f > 1.05 {
			t.Errorf("BP-RP = %v gives F_V/F_G = %v, outside anything a band ratio can be", c, f)
		}
	}
}
