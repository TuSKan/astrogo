package fits_test

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"testing"

	"github.com/TuSKan/astrogo/fits"
	"github.com/TuSKan/astrogo/internal/metrology"
)

// wcsFixturePath is the checked-in Astropy/WCSLIB reference table, written by
// testdata/wcsfixture/generate.py.
const wcsFixturePath = "testdata/wcs_astropy.json"

// wcsFixtureSchemaVersion is the layout this reader understands. Checked on
// read and never guessed at: reinterpreting an older layout would compare two
// things that were not measured the same way.
const wcsFixtureSchemaVersion = 1

// wcsFixture is the whole reference document.
type wcsFixture struct {
	SchemaVersion int    `json:"schema_version"`
	Generated     string `json:"generated"`
	AstrogoCommit string `json:"astrogo_commit"`

	Generator struct {
		Astropy string `json:"astropy"`
		WCSLIB  string `json:"wcslib"`
		NumPy   string `json:"numpy"`
		Python  string `json:"python"`
	} `json:"generator"`

	Convention string `json:"convention"`

	Cases []struct {
		Name    string         `json:"name"`
		Header  map[string]any `json:"header"`
		LonPole float64        `json:"lonpole"`
		LatPole float64        `json:"latpole"`
		Points  []struct {
			Pixel [2]float64 `json:"pixel"`
			World [2]float64 `json:"world"`
		} `json:"points"`
	} `json:"cases"`
}

// TestWCSAgreesWithWCSLIB holds PixelToWorld and WorldToPixel to WCSLIB, the
// reference implementation of Calabretta & Greisen, reached through Astropy
// (#523).
//
// # Why this and not the round trip
//
// The other WCS tests check the transform against itself: the reference
// pixel maps to CRVAL, a pixel round-trips, an axis step moves the sky the way
// CDELT's sign says. A projection wrong the same way in both directions passes
// all of them. This one cannot be passed that way.
//
// Both sides read the same cards. Each case in the fixture records its header
// verbatim, Astropy built its WCS from those cards, and this builds a
// fits.Header from them, so a disagreement is about the transform and never
// about a header re-expressed differently on one side.
//
// # The contracts
//
// Measured, the two agree to 4e-10 arcsec on the sky and 3e-10 pixel on the
// detector at worst, which is float64 rounding on angles of a few hundred
// degrees. The contracts sit more than three orders above that, and several
// below any convention error: a wrong LONPOLE, a flipped axis or a CD matrix
// split the wrong way moves a point by arcseconds at least.
//
// # What it found
//
// That last one, on its first run. A CD matrix was split into CDELT and PC by
// columns, so an ordinary rotated sky image — CD1_1 < 0, CD2_2 > 0 — was read
// rotated the other way: 324 arcsec out at 900 pixels, and every off-center
// point of the two affected headers past the contract (#524).
func TestWCSAgreesWithWCSLIB(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(wcsFixturePath)
	if err != nil {
		t.Fatalf("reading %s: %v", wcsFixturePath, err)
	}

	var fx wcsFixture
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatalf("parsing %s: %v", wcsFixturePath, err)
	}

	if fx.SchemaVersion != wcsFixtureSchemaVersion {
		t.Fatalf("%s has schema version %d; this reader understands %d",
			wcsFixturePath, fx.SchemaVersion, wcsFixtureSchemaVersion)
	}

	t.Logf("fixture: %d headers, generated %s at astrogo %s",
		len(fx.Cases), fx.Generated, fx.AstrogoCommit)
	t.Logf("  astropy %s, WCSLIB %s, numpy %s, python %s",
		fx.Generator.Astropy, fx.Generator.WCSLIB, fx.Generator.NumPy, fx.Generator.Python)

	ref := metrology.Reference{
		Kind:    metrology.KindImplementation,
		Name:    "WCSLIB via Astropy astropy.wcs",
		Version: "WCSLIB " + fx.Generator.WCSLIB + ", astropy " + fx.Generator.Astropy,
		Source:  "fits/testdata/wcsfixture/generate.py",
		Dataset: "testdata/wcs_astropy.json, generated " + fx.Generated,
		// Independent: astrogo's projections are its own Go, written from
		// Calabretta & Greisen (2002); WCSLIB is Calabretta's C. The paper is
		// shared, which is the point — it is the definition.
	}

	sky := metrology.NewSuite("fits.wcs.pixel_to_world", ref, metrology.MustContract(1e-6, "arcsec",
		"three orders above the ~4e-10 arcsec of float64 rounding on angles of hundreds of degrees, "+
			"and arcseconds to degrees below what a projection convention error produces",
		"Calabretta & Greisen (2002), A&A 395, 1077"))
	detector := metrology.NewSuite("fits.wcs.world_to_pixel", ref, metrology.MustContract(1e-6, "pixel",
		"the same margin on the detector side: float64 rounding is ~3e-10 pixel here, and a convention "+
			"error moves a point by whole pixels",
		"Calabretta & Greisen (2002), A&A 395, 1077"))

	for _, tc := range fx.Cases {
		wcs, err := fits.ExtractWCS(headerFromCards(t, tc.Header))
		if err != nil {
			t.Errorf("%s: ExtractWCS: %v", tc.Name, err)

			continue
		}

		for _, pt := range tc.Points {
			context := fmt.Sprintf("pixel (%g, %g), LONPOLE %g", pt.Pixel[0], pt.Pixel[1], tc.LonPole)

			world, err := wcs.PixelToWorld(pt.Pixel[:])
			if err != nil {
				t.Errorf("%s: PixelToWorld(%v): %v", tc.Name, pt.Pixel, err)

				continue
			}

			sky.Add(metrology.Sample{
				Error:   separationArcsec(world[0], world[1], pt.World[0], pt.World[1]),
				Label:   tc.Name,
				Context: context,
			})

			pixel, err := wcs.WorldToPixel(pt.World[:])
			if err != nil {
				t.Errorf("%s: WorldToPixel(%v): %v", tc.Name, pt.World, err)

				continue
			}

			detector.Add(metrology.Sample{
				Error:   math.Hypot(pixel[0]-pt.Pixel[0], pixel[1]-pt.Pixel[1]),
				Label:   tc.Name,
				Context: context,
			})
		}
	}

	sky.Report(t)
	detector.Report(t)
}

// headerFromCards builds a header from the fixture's cards, in a fixed order so
// a failure reproduces.
func headerFromCards(t *testing.T, cards map[string]any) *fits.Header {
	t.Helper()

	keys := make([]string, 0, len(cards))
	for k := range cards {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	h := fits.NewHeader()

	for _, k := range keys {
		var value string

		switch v := cards[k].(type) {
		case string:
			value = v
		case float64:
			value = strconv.FormatFloat(v, 'g', -1, 64)
		default:
			t.Fatalf("card %s has a %T value; the fixture holds only strings and numbers", k, v)
		}

		h.Append(fits.Card{Keyword: k, Value: value})
	}

	return h
}

// separationArcsec is the great-circle distance between two (longitude,
// latitude) points in degrees, by the haversine-stable Vincenty form, which
// stays accurate at the tiny separations being measured.
func separationArcsec(lon1, lat1, lon2, lat2 float64) float64 {
	const deg = math.Pi / 180

	dLon := (lon2 - lon1) * deg
	sinDLon, cosDLon := math.Sincos(dLon)
	sin1, cos1 := math.Sincos(lat1 * deg)
	sin2, cos2 := math.Sincos(lat2 * deg)

	num := math.Hypot(cos2*sinDLon, cos1*sin2-sin1*cos2*cosDLon)
	den := sin1*sin2 + cos1*cos2*cosDLon

	return math.Atan2(num, den) / deg * 3600
}
