package coord_test

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/epoch"
	"github.com/TuSKan/astrogo/internal/metrology"
	"github.com/TuSKan/astrogo/unit"
)

// lttFixturePath is the checked-in Astropy reference table for the
// barycentric light-travel delay.
const lttFixturePath = "testdata/ltt_astropy.json"

// lttFixtureSchemaVersion is the layout this reader understands, checked on
// read for the reason rvFixtureSchemaVersion is.
const lttFixtureSchemaVersion = 1

// lttFixture is the whole reference document, written by
// testdata/lttfixture/generate.py.
type lttFixture struct {
	SchemaVersion int    `json:"schema_version"`
	Generated     string `json:"generated"`
	AstrogoCommit string `json:"astrogo_commit"`

	Generator struct {
		Astropy string `json:"astropy"`
		PyERFA  string `json:"pyerfa"`
	} `json:"generator"`

	Model          string `json:"model"`
	SharedAncestry string `json:"shared_ancestry"`

	Cases []lttFixtureCase `json:"cases"`
}

// lttFixtureCase is one (site, epoch, target) triple: the delay, as Astropy
// publishes it and as the geometric sum of its own pieces gives it, and the
// barycentric arrival time the geometric delay gives, as a two-part TDB
// Julian Date. See testdata/lttfixture/generate.py for why the two differ.
type lttFixtureCase struct {
	Name string `json:"name"`

	Site struct {
		Lon     float64 `json:"lon_deg"`
		Lat     float64 `json:"lat_deg"`
		HeightM float64 `json:"height_m"`
	} `json:"site"`

	EpochUTC  string `json:"epoch_utc"`
	EpochNote string `json:"epoch_note"`

	Target struct {
		RA  float64 `json:"ra_deg"`
		Dec float64 `json:"dec_deg"`
	} `json:"target"`

	DelayPublishedS float64 `json:"delay_published_s"`
	DelayGeometricS float64 `json:"delay_geometric_s"`
	BJD1            float64 `json:"bjd_tdb_jd1"`
	BJD2            float64 `json:"bjd_tdb_jd2"`
}

func loadLTTFixture(t *testing.T) lttFixture {
	t.Helper()

	data, err := os.ReadFile(filepath.Clean(lttFixturePath))
	if err != nil {
		t.Fatalf("reading %s: %v\n  regenerate with: cd coord/testdata/lttfixture && uv run generate.py",
			lttFixturePath, err)
	}

	var f lttFixture
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("decoding %s: %v", lttFixturePath, err)
	}

	if f.SchemaVersion != lttFixtureSchemaVersion {
		t.Fatalf("%s is schema version %d, this reader understands %d",
			lttFixturePath, f.SchemaVersion, lttFixtureSchemaVersion)
	}

	if len(f.Cases) == 0 {
		t.Fatalf("%s carries no cases", lttFixturePath)
	}

	return f
}

// TestBarycentricDelayAgainstAstropy compares BarycentricDelay with the delay
// Astropy's pieces give over 175 cases, and ArrivalAtObserver with the instant
// Astropy's barycentric time came from: the forward conversion is Astropy's,
// so the inverse is checked against something other than itself.
//
// The delay is compared with the geometric sum, Earth's barycentric position
// plus the site's, rather than with what light_travel_time publishes, which
// removes stellar aberration from the site's direction (see
// testdata/lttfixture/generate.py). That gap is asserted instead: it is at
// most the site's distance times Earth's speed over c squared, about 2.1
// microseconds, and it is there at that size, so if either side changes its
// model the test says so.
func TestBarycentricDelayAgainstAstropy(t *testing.T) {
	t.Parallel()

	fixture := loadLTTFixture(t)

	t.Logf("fixture: %d cases, generated %s at astrogo %s, astropy %s, pyerfa %s",
		len(fixture.Cases), fixture.Generated, fixture.AstrogoCommit,
		fixture.Generator.Astropy, fixture.Generator.PyERFA)
	t.Logf("  model: %s", fixture.Model)
	t.Logf("  shared ancestry: %s", fixture.SharedAncestry)

	ref := metrology.Reference{
		Kind:    metrology.KindImplementation,
		Name:    "Astropy Time.light_travel_time",
		Version: fixture.Generator.Astropy,
		Source:  "coord/testdata/lttfixture/generate.py",
		Dataset: "testdata/ltt_astropy.json, generated " + fixture.Generated,

		// Both reach Earth's barycentric position by SOFA's epv00.
		SharedAncestor: "SOFA/ERFA epv00",
	}

	delay := metrology.NewSuite("coord.ltt.delay", ref, delayContract)
	arrival := metrology.NewSuite("coord.ltt.arrival", ref, arrivalContract)

	var largestGap float64

	// Refraction has no bearing on a light-travel time.
	atm := atmosphere.Refraction{Pressure: 0}

	for _, tc := range fixture.Cases {
		site, err := coord.NewGeodetic(angle.LatDeg(tc.Site.Lat), angle.LonDeg(tc.Site.Lon), unit.Meters(tc.Site.HeightM))
		if err != nil {
			t.Fatalf("%s: %v", tc.Name, err)
		}

		parsed, err := time.Parse(time.RFC3339, tc.EpochUTC+"Z")
		if err != nil {
			t.Fatalf("%s: parsing %q: %v", tc.Name, tc.EpochUTC, err)
		}

		observed := epoch.FromGo(parsed)

		ctx, err := coord.NewContext(observed, site, atm)
		if err != nil {
			t.Fatal(err)
		}

		target := coord.NewICRS(angle.Deg(tc.Target.RA), angle.Deg(tc.Target.Dec))
		context := tc.EpochUTC + " (" + tc.EpochNote + ")"

		got := ctx.BarycentricDelay(target).Seconds()

		delay.Add(metrology.Sample{
			Error:   got - tc.DelayGeometricS,
			Label:   tc.Name,
			Context: context,
		})

		gap := math.Abs(tc.DelayPublishedS - got)
		if gap > aberratedSiteBound {
			t.Errorf("%s: Astropy's light_travel_time is %.3f µs from astrogo's delay, more than the %.3f µs "+
				"its aberrated site vector can explain", tc.Name, gap*1e6, aberratedSiteBound*1e6)
		}

		largestGap = max(largestGap, gap)

		bary := epoch.FromJDParts(tc.BJD1, tc.BJD2, epoch.TDB)

		arrival.Add(metrology.Sample{
			Error:   ctx.ArrivalAtObserver(target, bary).Sub(observed).Seconds(),
			Label:   tc.Name,
			Context: context,
		})
	}

	delay.Report(t)
	arrival.Report(t)

	t.Logf("Astropy's published light_travel_time is at most %.3f µs from astrogo's delay, "+
		"its aberrated site vector bounding that at %.3f µs", largestGap*1e6, aberratedSiteBound*1e6)

	// Half the bound: the aberration is there at its size, not shrunk to
	// nothing, which would mean Astropy changed its model.
	if largestGap < aberratedSiteBound/2 {
		t.Errorf("Astropy's light_travel_time is never more than %.3f µs from astrogo's delay, where its "+
			"aberrated site vector reaches %.3f µs: Astropy has changed its model, and this test's account "+
			"of the gap needs revisiting", largestGap*1e6, aberratedSiteBound*1e6)
	}
}

// aberratedSiteBound is how far removing stellar aberration from the site's
// geocentric direction can move the delay: Earth's equatorial radius, times
// Earth's greatest orbital speed plus the site's rotation, over c squared.
var aberratedSiteBound = 6378137.0 * (30.29e3 + 465.1) / (299792458.0 * 299792458.0)

// delayContract bounds the delay suite by the Earth rotation the untagged build
// does not load: UTC keeps |DUT1| under 0.9 s, which turns an equatorial site
// through 0.42 km of its rotation, 1.40 µs of delay, and polar motion under
// 0.6 arcseconds adds under 0.1 µs. Everything else either side computes is
// shared (epv00) or float64 noise on a 500-second quantity.
var delayContract = metrology.MustContract(1.5e-6, "s",
	"the site rotation an unloaded DUT1 (|DUT1| < 0.9 s) and polar motion leave out: 1.40 + 0.06 µs",
	"UTC's definition for the DUT1 bound; IERS for polar motion; coord/barycentric_delay.go")

// arrivalContract adds to delayContract the truncation of astrogo's TDB−TT
// series, within 0.8 µs of SOFA's iauDtdb over 1900–2100, since the arrival is
// a TDB instant read back as UTC.
var arrivalContract = metrology.MustContract(2.3e-6, "s",
	"the delay contract's 1.46 µs plus the 0.8 µs truncation of astrogo's TDB−TT series against iauDtdb",
	"epoch/tdb.go (TestTDBMinusTTAgainstDtdb); coord/barycentric_delay.go")
