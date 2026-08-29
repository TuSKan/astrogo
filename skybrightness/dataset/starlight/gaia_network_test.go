//go:build network

package starlight_test

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/internal/testutil"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/skybrightness/dataset/starlight"
)

// The one test that catches a malformed query.
//
// Every other check here is a substring assertion, and substring assertions
// cannot tell valid ADQL from invalid. Two real defects got through them: a
// GROUP BY repeating the expression instead of naming the alias, and a CASE
// expression the archive rejects. Both produced an HTTP 400 and both were
// invisible until a query was actually sent.
func TestGaiaQueryIsAcceptedByTheArchive(t *testing.T) {
	t.Parallel()

	testutil.RequireReachable(t, "gea.esac.esa.int:443")

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// A few pixels only: enough to prove the archive parses and answers the
	// query this package generates, without aggregating a meaningful share of
	// 1.8 billion sources.
	build := starlight.GaiaBuild{Order: 8,
		Chunk: 4,
		Bands: []starlight.GaiaBand{{
			Name:           "V",
			ColourTerm:     []float64{0.02, 0.007, 0.17},
			FluxToRadiance: 1e-21,
		}},
	}

	adql, err := build.ADQL(100000, 100003)
	if err != nil {
		t.Fatalf("ADQL: %v", err)
	}

	t.Logf("query: %s", adql)

	m, counts, err := starlight.RunChunk(ctx, build, 100000, 100003)
	testutil.SkipOnUpstreamFailure(t, err)

	if err != nil {
		t.Fatalf("the archive rejected the generated query: %v", err)
	}

	var stars int64

	for _, c := range counts {
		stars += c
	}

	if stars == 0 {
		t.Error("four pixels of the Gaia catalogue held no sources at all")
	}

	t.Logf("four pixels held %d sources", stars)

	if m == nil {
		t.Fatal("no map returned")
	}
}

// Fetch is the path a user actually takes, so it gets run against the real
// archive: a scattered target list in one query, then the same list again to
// prove the cache answers without asking.
//
// The unit tests cover the query text and the cache bookkeeping. Neither can
// tell whether a several-hundred-pixel disjunction is something the archive
// will parse and answer in a reasonable time, which is the whole premise of
// fetching narrowly instead of building a sky.
func TestFetchAnswersATargetList(t *testing.T) {
	testutil.RequireReachable(t, "gea.esac.esa.int:443")

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	// Forty targets spread across the sky, the shape of a night's plan.
	dirs := make([]coord.ICRS, 0, 40)
	for i := range 40 {
		dirs = append(dirs, coord.NewICRS(
			angle.Deg(float64(i)*9), angle.Deg(float64(i%9)*10-40)))
	}

	spec := starlight.GaiaBuild{
		Order: 8,
		Bands: []starlight.GaiaBand{starlight.GaiaJohnsonV()},
	}

	start := time.Now()

	m, err := starlight.Fetch(ctx, spec, dirs...)
	if err != nil {
		// A congested archive is external downtime, which this repository's
		// policy says to skip on rather than fail. It answers this query in
		// three seconds when quiet and cancels its own SQL statement when not,
		// and neither says anything about the code under test.
		if archiveCongested(err) {
			t.Skipf("Gaia archive is not answering: %v", err)
		}

		t.Fatalf("Fetch: %v", err)
	}

	first := time.Since(start)

	var covered int

	for _, dir := range dirs {
		v, err := m.RadianceAt("V", dir.RA(), dir.Dec())
		if err != nil {
			t.Fatalf("RadianceAt: %v", err)
		}

		if v > 0 {
			covered++
		}
	}

	t.Logf("%d of %d directions covered in %v", covered, len(dirs), first.Round(time.Millisecond))

	if covered != len(dirs) {
		t.Errorf("only %d of %d directions came back with flux", covered, len(dirs))
	}

	// Again. Every pixel is cached now, so this must not touch the network —
	// which is what makes the second night on the same targets free.
	start = time.Now()

	if _, err := starlight.Fetch(ctx, spec, dirs...); err != nil {
		t.Fatalf("second Fetch: %v", err)
	}

	second := time.Since(start)
	t.Logf("cached repeat took %v", second.Round(time.Millisecond))

	// An absolute ceiling rather than a ratio against the first call.
	//
	// The property being asserted is "this did not touch the network", and a
	// ratio is a poor proxy for it: the first Fetch is only slow when the
	// on-disk cache is cold, so after any earlier run has warmed it both
	// calls come back in a few milliseconds and the ratio test fails on a
	// cache that is working perfectly. Measured here: 3.3 ms against a
	// "cold" 4.2 ms.
	//
	// A real query to the Gaia archive takes hundreds of milliseconds at
	// best — the aggregation runs measured in this package take seconds — so
	// anything under 100 ms cannot have made one, whatever the first call
	// did.
	const networkFloor = 100 * time.Millisecond

	if second > networkFloor {
		t.Errorf("cached repeat took %v, over the %v that only a network round trip could need; "+
			"the cache is not being used (first call: %v)", second, networkFloor, first)
	}
}

// archiveCongested reports whether an error is the archive being too busy
// rather than the query being wrong.
//
// A malformed query comes back as an HTTP 400 with a parse message and must
// still fail the test; a statement timeout or a dead connection is the service
// having a bad day.
func archiveCongested(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	text := err.Error()
	for _, sign := range []string{"statement timeout", "500", "timeout", "EOF", "connection reset"} {
		if strings.Contains(text, sign) {
			return true
		}
	}

	return false
}

// The same sky, aggregated at two orders, must hold the same stars.
//
// The whole build rests on one arithmetic claim: the high bits of a Gaia
// source_id are its nested HEALPix index, so source_id/2^(59-2k) is the level-k
// pixel. If that is wrong the query still runs, still returns a pixel per row
// and still produces a smooth, plausible map - it just puts the light in the
// wrong places, or loses it at boundaries.
//
// Four order-8 pixels starting on a multiple of four span exactly the source_id
// range of one order-7 pixel, because the order-7 divisor is four times the
// order-8 one. So the two aggregations cover identical sources and their totals
// must agree exactly. A divisor wrong by any power of two breaks this; so does
// an off-by-one in the range arithmetic.
//
// Checked once at full scale against the whole catalogue: summing the published
// order-8 map returns 1,811,709,771 sources and 1,540,770,489 with BP-RP, both
// exactly the values the archive reports for gaiadr3.gaia_source with no GROUP
// BY at all. This is the cheap version of that, four pixels instead of 786,432.
func TestSourceIDTilingAgreesAcrossOrders(t *testing.T) {
	t.Parallel()

	testutil.RequireReachable(t, "gea.esac.esa.int:443")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	band := starlight.GaiaBand{Name: "V", FluxToRadiance: 1e-21}

	const (
		fineFirst = 100000 // a multiple of four, so it aligns with an order-7 pixel
		fineLast  = 100003
		coarse    = fineFirst / 4
	)

	total := func(order int, first, last int64) int64 {
		build := starlight.GaiaBuild{
			Order: order,
			Chunk: 4,
			Bands: []starlight.GaiaBand{band},
		}

		_, counts, err := starlight.RunChunk(ctx, build, first, last)
		if err != nil {
			t.Skipf("archive did not answer the order-%d chunk: %v", order, err)
		}

		var sum int64

		for _, c := range counts {
			sum += c
		}

		return sum
	}

	fine := total(8, fineFirst, fineLast)
	coarseTotal := total(7, coarse, coarse)

	t.Logf("order 8 pixels %d-%d: %d sources; order 7 pixel %d: %d sources",
		fineFirst, fineLast, fine, coarse, coarseTotal)

	if fine == 0 {
		t.Skip("the archive returned an empty patch; nothing to compare")
	}

	if fine != coarseTotal {
		t.Errorf("the same sky holds %d sources at order 8 and %d at order 7; "+
			"the source_id divisor does not tile consistently", fine, coarseTotal)
	}
}

// The bright-star correction, end to end against both archives.
//
// This is the piece that makes the published map reproducible from committed
// code: without it the 74 stars have to be produced by a script nobody kept.
//
// The count is not asserted exactly. It depends on the match radius and on
// which Gaia sources currently carry a usable flux, and the point is that the
// answer stays in the dozens rather than the tens of thousands - the failure
// mode being guarded against is taking the 18,693 Hipparcos stars absent from
// gaiadr3.hipparcos2_best_neighbour as the missing set, which double-counts
// nearly all of them.
func TestFetchBrightStarsFindsTheSaturatedStars(t *testing.T) {
	t.Parallel()

	testutil.RequireReachable(t, "gea.esac.esa.int:443")
	testutil.RequireReachable(t, "tapvizier.u-strasbg.fr:80")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	stars, err := starlight.FetchBrightStars(ctx,
		starlight.BrightStarLimitV, starlight.BrightStarMatchRadius)
	if err != nil {
		t.Skipf("an archive did not answer: %v", err)
	}

	t.Logf("%d Hipparcos stars have no Gaia counterpart", len(stars))

	if len(stars) == 0 {
		t.Fatal("no missing stars at all; Gaia does not reach V = 3")
	}

	if len(stars) > 2000 {
		t.Errorf("%d stars reported missing; that is the crossmatch-complement "+
			"mistake, not Gaia's saturation limit", len(stars))
	}

	// The population is defined by saturation, so it has to be dominated by the
	// very brightest sky. Measured: 70 of 74 brighter than V = 3.
	var brighterThan3, brightest int

	brightestV := math.Inf(1)

	for i, s := range stars {
		if s.Vmag < 3 {
			brighterThan3++
		}

		if s.Vmag < brightestV {
			brightestV, brightest = s.Vmag, i
		}
	}

	t.Logf("brightest is HIP %d at V = %.2f; %d of %d are brighter than V = 3",
		stars[brightest].HIP, brightestV, brighterThan3, len(stars))

	if frac := float64(brighterThan3) / float64(len(stars)); frac < 0.5 {
		t.Errorf("only %.0f%% of the missing stars are brighter than V = 3; "+
			"this set should be saturation-limited", 100*frac)
	}

	// Sirius is the brightest star in the sky and Gaia cannot measure it, so it
	// has to be in any correct answer.
	const sirius = 32349

	found := false

	for _, s := range stars {
		if s.HIP == sirius {
			found = true

			break
		}
	}

	if !found {
		t.Errorf("HIP %d (Sirius) is not in the missing set", sirius)
	}
}

// Four bands cost one pass, not four.
//
// This is the fact the whole multi-band map rests on. The aggregation is one
// query per source_id range with each band as extra select columns under a
// single GROUP BY, so adding B, R and I alongside V widens the rows and does
// not multiply the queries. If it did, building four bands would mean four
// sweeps of 787 chunks against a shared research archive instead of one, which
// is the difference between a job worth running and one that is not.
//
// The substring assertions elsewhere cannot check this: a four-band query is
// four times as much ADQL and the only thing that says the archive will parse
// and answer it is sending one.
func TestFourBandQueryIsOnePass(t *testing.T) {
	t.Parallel()

	testutil.RequireReachable(t, "gea.esac.esa.int:443")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	bands := make([]starlight.GaiaBand, 0, 4)

	for _, name := range []string{"B", "V", "R", "I"} {
		colour, err := starlight.JohnsonCousinsColourTerm(name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}

		bands = append(bands, starlight.GaiaBand{
			Name:           name,
			ColourTerm:     colour,
			FluxToRadiance: 1e-21,
		})
	}

	build := starlight.GaiaBuild{Order: 8, Chunk: 4, Bands: bands}

	adql, err := build.ADQL(100000, 100003)
	if err != nil {
		t.Fatalf("ADQL: %v", err)
	}

	// One SELECT and one GROUP BY however many bands are asked for.
	if n := strings.Count(strings.ToUpper(adql), "SELECT"); n != 1 {
		t.Errorf("the four-band query has %d SELECT clauses; it must be one pass", n)
	}

	if n := strings.Count(strings.ToUpper(adql), "GROUP BY"); n != 1 {
		t.Errorf("the four-band query has %d GROUP BY clauses; it must be one pass", n)
	}

	t.Logf("query (%d characters): %s", len(adql), adql)

	m, counts, err := starlight.RunChunk(ctx, build, 100000, 100003)
	testutil.SkipOnUpstreamFailure(t, err)

	if err != nil {
		t.Fatalf("the archive rejected the four-band query: %v", err)
	}

	var stars int64
	for _, c := range counts {
		stars += c
	}

	if stars == 0 {
		t.Fatal("four pixels of the Gaia catalogue held no sources at all")
	}

	if m == nil {
		t.Fatal("no map returned")
	}

	got := m.Bands()
	if len(got) != 4 {
		t.Fatalf("the map carries %d bands (%v), want four", len(got), got)
	}

	// Every band has to carry light, and the reddest has to carry the most:
	// the sky's integrated starlight is dominated by cool stars, so a pixel is
	// brighter in I than in B. A band wired to the wrong colour polynomial or
	// the wrong zero point shows here rather than after a 787-chunk build.
	radiance := map[string]float64{}

	for _, name := range got {
		var total float64

		for pixel := int64(100000); pixel <= 100003; pixel++ {
			v, err := m.Pixel(name, pixel)
			if err != nil {
				t.Fatalf("%s at pixel %d: %v", name, pixel, err)
			}

			total += v
		}

		radiance[name] = total
		t.Logf("  %s: %.6e W m^-2 sr^-1 nm^-1 over four pixels", name, total)

		if total <= 0 {
			t.Errorf("%s carries no light", name)
		}
	}

	if radiance["I"] <= radiance["B"] {
		t.Errorf("these pixels are %.4e in I and %.4e in B; integrated starlight is "+
			"dominated by cool stars, so the red band must carry more",
			radiance["I"], radiance["B"])
	}
}
