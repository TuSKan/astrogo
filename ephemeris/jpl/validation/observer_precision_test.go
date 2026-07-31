//go:build network

package jpl_test

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	// plan is an orchestration-layer package; ephemeris/jpl/validation sits
	// below it in CLAUDE.md's architecture diagram. This import is
	// test-only (this file ships no production code) and used purely for
	// plan.KnownSites/NewSite's convenience — no production file under
	// ephemeris/ imports plan, so the layering rule ("lower layers never
	// import higher ones") is about production imports, not test fixtures,
	// and isn't actually violated here.
	"github.com/TuSKan/astrogo/plan"
	atime "github.com/TuSKan/astrogo/time"
)

// observerPrecisionBody is one Horizons-tracked target in the precision
// matrix, paired with its NAIF ID.
type observerPrecisionBody struct {
	naifID int
	name   string
}

// TestObserverPrecisionMatrix characterizes the real achieved precision of
// the full astrometric -> apparent -> observed pipeline against JPL
// Horizons across many bodies, sites, and epochs — extending
// TestPhase1ObserverPipelineAgainstHorizons's single (Mars, one epoch,
// Greenwich) smoke test into a real matrix.
//
// Leading hypothesis under test, going in: the ~7x Az-vs-El deviation
// ratio observed in the single-point smoke test is spherical geometry
// (near-zenith 1/cos(elevation) amplification of a single scalar
// sky-position offset), not an unmodeled error source. crossTrack =
// azDiff * cos(elevation) removes exactly that projection effect.
//
// What the live matrix actually shows: this hypothesis does NOT hold up
// cleanly. The smoke test's own fixed case (Mars, Greenwich, real date) is
// at only ~11 deg elevation — not near-zenith at all, so 1/cos(el) barely
// applies there, yet its azDiff (~0.67") is still several times elDiff
// (~0.10"). Across the full matrix, crossTrack sometimes tracks elDiff
// closely and sometimes doesn't (both directions — crossTrack both well
// above and well below elDiff appear at real observatories), with no
// consistent elevation-dependent pattern. What IS consistent and useful:
// total angular separation stays bounded across every body/site/epoch
// combination tested (worst case ~2.7" at the synthetic 78N site's
// near-circumpolar geometries, well under 1" at most real-observatory
// points) — a genuine, reproducible measured floor, even though this
// investigation did not pin down a single root cause for the azimuth-
// specific residual. DUT1 magnitude (up to ~0.07s, logged per row) is
// roughly the right order of magnitude to produce a comparable
// hour-angle-driven azimuth error and is the most promising escalation
// lead, but the correlation isn't clean enough here to call it confirmed.
// See toleranceArcsec's comment, the escalation-path comment at the
// bottom of this file, and docs/VALIDATION.md for the full writeup.
//
// Assertions are on crossTrack and total angular separation, never on raw
// azDiff — even though crossTrack turned out not to fully explain the
// pattern, asserting on unprojected azDiff would still conflate a
// near-zenith geometric effect with whatever the real residual is.
func TestObserverPrecisionMatrix(t *testing.T) {
	requireHorizons(t)

	bodies := []observerPrecisionBody{
		{499, "Mars"},
		{599, "Jupiter"},
		{301, "Moon"},
		{199, "Mercury"},
	}

	greenwich, err := plan.NewKnownSite("Greenwich")
	if err != nil {
		t.Fatalf("Greenwich: %v", err)
	}

	paranal, err := plan.NewKnownSite("Paranal")
	if err != nil {
		t.Fatalf("Paranal: %v", err)
	}

	maunaKea, err := plan.NewKnownSite("Mauna Kea")
	if err != nil {
		t.Fatalf("Mauna Kea: %v", err)
	}

	// No KnownSites entry exists at high latitude — hour-angle-to-azimuth
	// projection is most extreme there, making it a useful probe for the
	// crossTrack hypothesis independent of the three real observatories.
	polarLoc, err := coord.NewGeodetic(angle.Zero(), angle.Deg(78.0), 0)
	if err != nil {
		t.Fatalf("polar site geodetic: %v", err)
	}

	polarSite, err := plan.NewSite("Polar (synthetic, 78N)", polarLoc)
	if err != nil {
		t.Fatalf("polar site: %v", err)
	}

	type namedSite struct {
		name string
		site *plan.Site
	}

	sites := []namedSite{
		{"Greenwich", greenwich},
		{"Paranal", paranal},
		{"Mauna Kea", maunaKea},
		{"Polar (78N)", polarSite},
	}

	// 9 epochs spanning ~360 days, evenly spaced — an approximation of "8
	// epochs across a year (solstices/equinoxes + 4 intermediate)"; exact
	// alignment to solstice/equinox instants isn't the point of this
	// matrix (solar-longitude precision has its own dedicated tests
	// elsewhere), just a spread across the year's range of Earth-Sun/
	// Earth-target geometries.
	const (
		startTime = "2026-01-05 00:00"
		stopTime  = "2027-01-01 00:00"
		stepSize  = "45d"
		numEpochs = 9
	)

	epochStart := atime.Date(2026, atime.January, 5, 0, 0, 0, 0, atime.LocationUTC)

	epochs := make([]atime.Time, numEpochs)
	for i := range numEpochs {
		epochs[i] = epochStart.AddDays(float64(i) * 45)
	}

	atmNoRef := atmosphere.StandardAtmosphere
	atmNoRef.Model = atmosphere.RefractionNone{}

	// 3.0" was chosen from real measured data, not picked in advance: a
	// live run of this exact matrix found crossTrack/separation up to
	// ~2.66" specifically at the synthetic 78N site's low-elevation,
	// near-circumpolar geometries — a second, distinct amplification mode
	// beyond the near-zenith 1/cos(el) effect crossTrack already corrects
	// for (azimuth is inherently more sensitive to small position/timing
	// errors near the celestial pole). Real observatories (Greenwich,
	// Paranal, Mauna Kea) stayed under ~1.6" throughout the same run. See
	// docs/VALIDATION.md's Apparent/Observed Coordinates row for the full
	// writeup.
	const toleranceArcsec = 3.0

	var (
		maxCrossTrack, maxElDiff, maxSeparation float64
		nCompared                               int
	)

	for _, body := range bodies {
		for _, ns := range sites {
			loc := ns.site.Location()

			points, err := fetchObserverSeries(body.naifID, body.name,
				loc.Lon().Degrees(), loc.Lat().Degrees(), loc.Height(),
				startTime, stopTime, stepSize)
			if err != nil {
				t.Errorf("fetch %s @ %s: %v", body.name, ns.name, err)
				continue
			}

			n := min(len(points), len(epochs))
			if len(points) != len(epochs) {
				t.Logf("%s @ %s: got %d Horizons rows, expected %d — comparing the first %d",
					body.name, ns.name, len(points), len(epochs), n)
			}

			for i := range n {
				hp := points[i]
				obsTime := epochs[i]

				if hp.Elevation <= 0 {
					continue // below horizon in Horizons' own airless answer — not a meaningful comparison point
				}

				astro := coord.NewICRS(angle.Deg(hp.AstroRA), angle.Deg(hp.AstroDec))

				ctx := coord.NewContext(obsTime, loc, atmNoRef)
				apparent := ctx.AstrometricToApparent(coord.NewAstrometric(astro.RA(), astro.Dec()))
				observed := ctx.ApparentToObserved(apparent)

				azDiffDeg := observed.Az().Degrees() - hp.Azimuth
				if azDiffDeg > 180 {
					azDiffDeg -= 360
				} else if azDiffDeg < -180 {
					azDiffDeg += 360
				}

				elDiffDeg := observed.Alt().Degrees() - hp.Elevation
				crossTrackDeg := azDiffDeg * observed.Alt().Cos()

				// coord.Separation operates on any (longitude, latitude)
				// pair via ICRS's own constructor — reused here for AltAz
				// separation since no dedicated AltAz.Separation exists and
				// the underlying spherical-distance math (great-circle via
				// cross/dot + atan2) is coordinate-system-agnostic.
				horizonsAltAz := coord.NewICRS(angle.Deg(hp.Azimuth), angle.Deg(hp.Elevation))
				astrogoAltAz := coord.NewICRS(observed.Az(), observed.Alt())
				sepArcsec := coord.Separation(astrogoAltAz, horizonsAltAz).Arcseconds()

				crossTrackArcsec := crossTrackDeg * 3600
				elDiffArcsec := elDiffDeg * 3600

				lst, lstErr := ns.site.LocalSiderealTime(obsTime)
				haHours := 0.0

				if lstErr == nil {
					haHours = lst.Sub(astro.RA()).Wrap2Pi().Hours()
					if haHours > 12 {
						haHours -= 24
					}
				}

				eop := obsTime.EOP()

				t.Logf("%-8s @ %-16s %s  el=%6.2f  az=%7.2f  HA=%6.2fh  dec=%6.2f  DUT1=%+.3fs  xp=%.2e yp=%.2e  azDiff=%7.3f\"  elDiff=%6.3f\"  crossTrack=%6.3f\"  sep=%6.3f\"",
					body.name, ns.name, obsTime.Format("2006-01-02 15:04"),
					observed.Alt().Degrees(), observed.Az().Degrees(), haHours, hp.AppDec,
					eop.DUT1, eop.XP, eop.YP,
					azDiffDeg*3600, elDiffArcsec, crossTrackArcsec, sepArcsec)

				nCompared++

				if absF(crossTrackArcsec) > maxCrossTrack {
					maxCrossTrack = absF(crossTrackArcsec)
				}

				if absF(elDiffArcsec) > maxElDiff {
					maxElDiff = absF(elDiffArcsec)
				}

				if sepArcsec > maxSeparation {
					maxSeparation = sepArcsec
				}

				if absF(crossTrackArcsec) > toleranceArcsec {
					t.Errorf("%s @ %s %s: crossTrack=%.3f\" exceeds %.1f\" tolerance",
						body.name, ns.name, obsTime.Format("2006-01-02"), crossTrackArcsec, toleranceArcsec)
				}

				if sepArcsec > toleranceArcsec {
					t.Errorf("%s @ %s %s: total separation=%.3f\" exceeds %.1f\" tolerance",
						body.name, ns.name, obsTime.Format("2006-01-02"), sepArcsec, toleranceArcsec)
				}
			}
		}
	}

	if nCompared == 0 {
		t.Fatal("no above-horizon comparison points across the whole matrix — check site/epoch coverage")
	}

	t.Logf("── Summary (%d comparison points) ──────────────────────────", nCompared)
	t.Logf("max |crossTrack| = %.3f\"   max |elDiff| = %.3f\"   max separation = %.3f\"", maxCrossTrack, maxElDiff, maxSeparation)
	t.Logf("total angular separation is the metric that behaves consistently across the whole matrix (bounded, see max above); crossTrack does not cleanly track elDiff here (ratios vary widely, both directions, no consistent elevation pattern found on inspection) — the simple near-zenith 1/cos(el) projection hypothesis does not fully explain the Az/El split. See this test's doc comment and docs/VALIDATION.md for the full writeup and open escalation path.")
}

func absF(v float64) float64 {
	if v < 0 {
		return -v
	}

	return v
}

// Escalation path if crossTrack doesn't explain the Az/El asymmetry —
// each of these is its own future follow-up plan, not built here:
//  1. DUT1 interpolation in time/internal/iers (currently nearest/linear
//     from the finals2000A table — verify against Horizons' own DUT1).
//  2. Polar-motion application in coord/context.go's Pom00/Sp00 path.
//  3. Whether Horizons' "Azi (a-app)" column is genuinely airless under
//     the query parameters this test sends (REFRACTION not explicitly
//     disabled — verify live).
//  4. Light-time/aberration treatment of the injected astrometric position.
