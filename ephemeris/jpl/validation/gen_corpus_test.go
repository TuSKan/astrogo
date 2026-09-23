//go:build network

package jpl_test

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/time"
)

// updateCorpus gates every write to the checked-in corpus.
//
// Without it TestGenerateCorpus fetches, compares and reports, and writes
// nothing. That is the whole point: the previous generator called
// os.WriteFile unconditionally, so running it — including by accident, as
// part of "go test -tags=network ./..." — silently replaced the reference
// data that every regression test is measured against. Reference data is
// evidence; overwriting it to make a test pass is the one move that
// invalidates the entire suite.
var updateCorpus = flag.Bool("update-corpus", false,
	"rewrite the checked-in Horizons corpus (otherwise the generator only reports what would change)")

// corpusSites is the observer list, named and with its provenance.
//
// The first three come from plan.KnownSites, whose coordinates are published
// values cross-checked against the IAU Minor Planet Center's observatory
// codes. The last two are synthetic, and they are here because KnownSites is
// entirely mid-latitude and tropical: its extremes are Greenwich at 51 north
// and Paranal at 25 south, so nothing in it exercises the geometry where
// hour-angle-to-azimuth projection blows up or where a body passes through
// the zenith.
// corpusTargets are the bodies Horizons answers topocentric OBSERVER queries
// for under CENTER='coord@399'.
//
// The Sun, Mercury and the Moon are deliberately absent: Horizons returns a
// bare HTTP 500 for those three under this exact query shape, reproducibly
// and independently of astrogo — confirmed with curl in isolation, and
// documented at length in observer_precision_test.go. Including them would
// put a permanent skip in the generator and a gap in the corpus that looked
// like an oversight.
func corpusTargets() []observerPrecisionBody {
	return []observerPrecisionBody{
		{499, "Mars"},
		{599, "Jupiter"},
		{699, "Saturn"},
	}
}

// corpusSpan is one contiguous set of epochs fetched in a single request.
type corpusSpan struct {
	class samplingClass
	start string
	stop  string
	step  string
	why   string
}

// A span must end in the settled past, and that is not a detail.
//
// Horizons' answer for a future epoch depends on predicted Earth orientation,
// which firms up as IERS publishes. The regular span used to run to
// 2027-01-01, so its last epoch — 2026-12-16 — moved by up to 8.4e-05 degrees
// between generation and the next run, and TestGenerateCorpus failed every
// time thereafter. A generator that can never report a clean diff is as
// useless as one that can never report a dirty one.
//
// It is worse than a failing test. A frozen entry whose true value is still
// moving is a reference that goes quietly out of date: the consumer compares
// astrogo, using whatever IERS series is on the machine today, against a
// prediction made on the day the corpus was written, and the two diverge for
// reasons that have nothing to do with astrogo.
//
// IERS values reach final status roughly a year after the fact, so the stop
// date sits well behind that. [TestCorpusEpochsAreSettled] enforces it.
func corpusSpans() []corpusSpan {
	return []corpusSpan{
		{
			class: classRegular,
			start: "2021-01-01 00:00", stop: "2025-01-01 00:00", step: "120d",
			why: "even sampling across four years, which is what catches secular " +
				"drift, and ends far enough in the past that every epoch has final Earth " +
				"orientation rather than a prediction",
		},
		{
			class: classBoundary,
			start: "2016-12-30 00:00", stop: "2017-01-02 00:00", step: "1d",
			why: "straddles the 2017-01-01 leap second: the day before, the day of, and the day after",
		},
		{
			class: classBoundary,
			start: "1999-12-31 12:00", stop: "2000-01-02 12:00", step: "1d",
			why: "straddles J2000.0, the origin every reduction in the pipeline is expressed against",
		},
	}
}

// TestGenerateCorpus refreshes the Horizons reference corpus.
//
// Without -update-corpus it fetches, compares against what is checked in, and
// prints what would change — then fails if anything would. With the flag it
// writes, and prints the same summary for the commit message.
//
//	go test -tags=network -run TestGenerateCorpus ./ephemeris/jpl/validation/
//	go test -tags=network -run TestGenerateCorpus ./ephemeris/jpl/validation/ -args -update-corpus
func TestGenerateCorpus(t *testing.T) {
	requireHorizons(t)

	sites := corpusSites(t)
	targets := corpusTargets()
	spans := corpusSpans()

	fresh := &corpus{Manifest: newCorpusManifest(t, spans, sites)}

	for _, target := range targets {
		for _, span := range spans {
			// One vector series per (body, span) — the geocentric state
			// does not depend on the site, so this is fetched once and
			// reused across every site below.
			vectors, err := fetchVectorSeries(target.command(), target.name, span.start, span.stop, span.step)
			if err != nil {
				// Not omitted any more, which is what it used to be. A span left
				// out of the fetch is a span missing from the diff, and the diff
				// then reported "removed" entries and failed with "the corpus
				// would change" — so a rate-limited Horizons read as upstream
				// drift. A partial fetch cannot be compared with a whole corpus,
				// so it either skips, when Horizons is the one failing, or fails
				// outright when it is not.
				skipIfHorizonsDown(t, err)
				t.Fatalf("vectors for %s over %s: %v", target.name, span.class, err)
			}

			for _, site := range sites {
				points, err := fetchObserverSeries(target.command(), target.name,
					site.Lon, site.Lat, site.Height, span.start, span.stop, span.step)
				if err != nil {
					skipIfHorizonsDown(t, err)
					t.Fatalf("observer table for %s @ %s over %s: %v",
						target.name, site.Name, span.class, err)
				}

				n := min(len(points), len(vectors))
				if len(points) != len(vectors) {
					t.Logf("%s @ %s: %d observer rows against %d vector rows — pairing the first %d",
						target.name, site.Name, len(points), len(vectors), n)
				}

				class := span.class
				if site.Name == "Polar (synthetic, 78N)" || site.Name == "Equator (synthetic, 0N 0E)" {
					class = classAdversarial
				}

				for i := range n {
					fresh.Entries = append(fresh.Entries, corpusEntry{
						Class:      class,
						TargetID:   target.naifID,
						TargetName: target.name,
						SiteName:   site.Name,
						EpochJDUT:  2451545.0 + vectors[i].ET/86400.0,
						GeoVector: [3]float64{
							vectors[i].Pos[0], vectors[i].Pos[1], vectors[i].Pos[2],
						},
						GeoVelocity: [3]float64{
							vectors[i].Vel[0], vectors[i].Vel[1], vectors[i].Vel[2],
						},
						Observed: corpusPoint{
							AstroRA:   points[i].AstroRA,
							AstroDec:  points[i].AstroDec,
							AppRA:     points[i].AppRA,
							AppDec:    points[i].AppDec,
							Azimuth:   points[i].Azimuth,
							Elevation: points[i].Elevation,
							Range:     points[i].Range,
						},
					})
				}
			}
		}
	}

	if len(fresh.Entries) == 0 {
		t.Fatal("Horizons returned nothing for any body, site or span — refusing to write an empty corpus")
	}

	fresh.sortEntries()

	summary := diffCorpus(loadForDiff(t), fresh)
	t.Log(summary)

	if !*updateCorpus {
		if strings.Contains(summary, "no change") {
			t.Logf("the checked-in corpus already matches Horizons (%d entries)", len(fresh.Entries))

			return
		}

		t.Fatalf("the corpus would change; rerun with -args -update-corpus to accept it, " +
			"after reading the summary above and understanding why each number moved")
	}

	writeCorpus(t, fresh)
}

// loadForDiff returns the checked-in corpus, or an empty one when there is
// none yet, so the first generation reports every entry as added rather than
// failing.
func loadForDiff(t *testing.T) *corpus {
	t.Helper()

	existing, err := loadCorpus()
	if err != nil {
		t.Logf("no comparable corpus on disk (%v) — everything below is new", err)

		return &corpus{}
	}

	return existing
}

// newCorpusManifest records where this document came from.
func newCorpusManifest(t *testing.T, spans []corpusSpan, sites []corpusSite) corpusManifest {
	t.Helper()

	why := make([]string, 0, len(spans))
	for _, s := range spans {
		why = append(why, fmt.Sprintf("%s: %s (%s..%s step %s)", s.class, s.why, s.start, s.stop, s.step))
	}

	return corpusManifest{
		SchemaVersion: corpusSchemaVersion,
		Generated:     time.Now().UTC().Format(time.RFC3339),
		Commit:        gitCommit(t),
		Reference:     "JPL Horizons (https://ssd.jpl.nasa.gov/api/horizons.api)",
		ReferenceQuery: "EPHEM_TYPE=OBSERVER CENTER='coord@399' QUANTITIES='1,2,4,20' CAL_FORMAT=JD; " +
			"EPHEM_TYPE=VECTORS CENTER='@399' OUT_UNITS=AU-D REF_PLANE=FRAME VEC_TABLE=2 TIME_TYPE=UT",
		Sites:      sites,
		Refraction: "AIRLESS — no APPARENT parameter is sent, so Horizons applies its default and states it in the response header",
		Sampling: strings.Join(why, "; ") +
			"; no pseudo-random class: Horizons serves evenly spaced series, so each random " +
			"epoch would cost its own request for no coverage that boundary and adversarial " +
			"sampling do not already provide",
		NotPinned: []string{
			"planetary kernel and its hash: no astrogo kernel takes part in producing these values, " +
				"and the consumer feeds the geocentric state through a linear mock provider on purpose",
			"Earth-orientation data: the consumer builds a coord.Context and so depends on whichever " +
				"IERS series is on the machine when the comparison runs",
		},
	}
}

// diffCorpus describes what regenerating would change, in enough detail to
// review.
//
// A reference-data change is a source change and gets read like one. "247
// entries changed" is not reviewable; "3 entries changed, worst 4.2e-06
// degrees in azimuth at Saturn @ Paranal" is.
func diffCorpus(old, fresh *corpus) string {
	oldByKey := make(map[string]corpusEntry, len(old.Entries))
	for _, e := range old.Entries {
		oldByKey[e.key()] = e
	}

	freshByKey := make(map[string]corpusEntry, len(fresh.Entries))
	for _, e := range fresh.Entries {
		freshByKey[e.key()] = e
	}

	var (
		added, removed, changed []string
		worst                   float64
		worstWhere              string
	)

	// Per field, how many values moved and by how much at most.
	type fieldStat struct {
		count int
		max   float64
	}

	byField := map[string]fieldStat{}

	for key, e := range freshByKey {
		prev, ok := oldByKey[key]
		if !ok {
			added = append(added, key)

			continue
		}

		for _, d := range fieldDeltas(prev, e) {
			st := byField[d.name]
			st.count++

			if d.delta > st.max {
				st.max = d.delta
			}

			byField[d.name] = st
		}

		delta, field := largestFieldDelta(prev, e)
		if delta > 0 {
			changed = append(changed, fmt.Sprintf("%s (%s by %.3g)", key, field, delta))
		}

		if delta > worst {
			worst, worstWhere = delta, key+" "+field
		}
	}

	for key := range oldByKey {
		if _, ok := freshByKey[key]; !ok {
			removed = append(removed, key)
		}
	}

	slices.Sort(added)
	slices.Sort(removed)
	slices.Sort(changed)

	var b strings.Builder

	fmt.Fprintf(&b, "\n── corpus diff ──\n")
	fmt.Fprintf(&b, "  on disk: %d entries    fetched: %d entries\n", len(old.Entries), len(fresh.Entries))
	fmt.Fprintf(&b, "  added: %d   removed: %d   changed: %d\n", len(added), len(removed), len(changed))

	if worst > 0 {
		fmt.Fprintf(&b, "  largest numeric change: %.6g at %s\n", worst, worstWhere)
	}

	// Grouped by field, which is what says whether the diff has a shape. A
	// column that moves only at its own last printed digit is Horizons'
	// emission tipping; one that moves anywhere else is a different animal and
	// should not be accepted on the strength of the max alone.
	if len(byField) > 0 {
		names := make([]string, 0, len(byField))
		for name := range byField {
			names = append(names, name)
		}

		slices.Sort(names)

		fmt.Fprintf(&b, "  by field:\n")

		for _, name := range names {
			st := byField[name]
			fmt.Fprintf(&b, "    %-16s %4d values, largest %.3g\n", name, st.count, st.max)
		}
	}

	for _, group := range []struct {
		label string
		keys  []string
	}{{"added", added}, {"removed", removed}, {"changed", changed}} {
		for i, k := range group.keys {
			if i >= 10 {
				fmt.Fprintf(&b, "  ... and %d more %s\n", len(group.keys)-10, group.label)

				break
			}

			fmt.Fprintf(&b, "  %s: %s\n", group.label, k)
		}
	}

	if len(added)+len(removed)+len(changed) == 0 {
		b.WriteString("  no change\n")
	}

	return b.String()
}

// fieldDeltas returns every field that differs between two entries, with by
// how much.
//
// All of them rather than the largest, because the largest alone cannot tell a
// reviewer what kind of change they are looking at. "94 entries changed, the
// largest by 1e-06" is compatible with a rounding shift and with a body moving,
// and the two call for opposite responses. Grouped by field it is obvious at a
// glance: angles moving only at their last printed digit while range moves only
// at its own is a reference whose emission tipped, and anything that lands
// somewhere else is not.
func fieldDeltas(a, b corpusEntry) []fieldDelta {
	var out []fieldDelta

	consider := func(name string, x, y float64) {
		if d := math.Abs(x - y); d > 0 {
			out = append(out, fieldDelta{name: name, delta: d})
		}
	}

	consider("astro_ra", a.Observed.AstroRA, b.Observed.AstroRA)
	consider("astro_dec", a.Observed.AstroDec, b.Observed.AstroDec)
	consider("app_ra", a.Observed.AppRA, b.Observed.AppRA)
	consider("app_dec", a.Observed.AppDec, b.Observed.AppDec)
	consider("azimuth", a.Observed.Azimuth, b.Observed.Azimuth)
	consider("elevation", a.Observed.Elevation, b.Observed.Elevation)
	consider("range", a.Observed.Range, b.Observed.Range)

	for i := range 3 {
		consider(fmt.Sprintf("geo_vector[%d]", i), a.GeoVector[i], b.GeoVector[i])
		consider(fmt.Sprintf("geo_velocity[%d]", i), a.GeoVelocity[i], b.GeoVelocity[i])
	}

	return out
}

// fieldDelta is one field that moved, and by how much.
type fieldDelta struct {
	name  string
	delta float64
}

// largestFieldDelta returns the biggest absolute difference between two
// entries and the field it is in.
func largestFieldDelta(a, b corpusEntry) (float64, string) {
	var (
		worst float64
		field string
	)

	for _, d := range fieldDeltas(a, b) {
		if d.delta > worst {
			worst, field = d.delta, d.name
		}
	}

	return worst, field
}

// writeCorpus persists the document.
func writeCorpus(t *testing.T, c *corpus) {
	t.Helper()

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		t.Fatalf("encoding the corpus: %v", err)
	}

	if err := os.MkdirAll(filepath.Dir(corpusPath), 0o750); err != nil {
		t.Fatalf("creating the corpus directory: %v", err)
	}

	if err := os.WriteFile(corpusPath, append(data, '\n'), 0o600); err != nil {
		t.Fatalf("writing the corpus: %v", err)
	}

	t.Logf("wrote %d entries to %s", len(c.Entries), corpusPath)
}

// gitCommit stamps the corpus with the astrogo revision that produced it.
func gitCommit(t *testing.T) string {
	t.Helper()

	out, err := exec.Command("git", "rev-parse", "HEAD").Output() //nolint:noctx // a fixed command with no user input, run once at generation time
	if err != nil {
		t.Logf("git rev-parse: %v — the corpus will be stamped 'unknown'", err)

		return "unknown"
	}

	return strings.TrimSpace(string(out))
}

// skipIfHorizonsDown skips t when err is Horizons failing rather than astrogo:
// a status SkipOnUpstreamFailure reads as the service's, a dropped connection,
// or the HTML page JPL serves in place of an API answer under load.
//
// The last is the one testutil cannot see. errHorizonsUnavailable carries no
// status, because the page is identified by its body — which is also the only
// case left once every fetcher checks the status first, since Horizons
// normally sends that page with a 503.
func skipIfHorizonsDown(t *testing.T, err error) {
	t.Helper()

	if errors.Is(err, errHorizonsUnavailable) {
		t.Skipf("JPL Horizons served its error page rather than an API answer: %v", err)
	}

	testutil.SkipOnUpstreamFailure(t, err)
}
