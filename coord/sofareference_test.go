package coord_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/internal/metrology"
	"github.com/TuSKan/astrogo/time"
)

// TestTopocentricPathIsSOFAsAtco13 pins astrogo's two-step topocentric
// reduction against SOFA's one-call iauAtco13 for the same inputs.
//
// # What this is, and what it is not
//
// It is not independent validation, and it is recorded as a consistency check
// rather than as accuracy. Both sides call the same gofa routines: astrogo
// reaches them as Apco13 once per epoch, then Atciq and Atioq per position,
// while the reference calls Atco13, which is those three in one function.
// The physics is shared and cannot be the residual.
//
// What is *not* shared is the wiring, and that is the whole subject. Between a
// caller's inputs and Apco13's arguments sit a time-scale conversion, a
// two-part Julian date, longitude and latitude in radians, height in metres,
// polar motion in radians, and a pressure that has to become zero when a
// custom refraction model is present. Between Atioq's outputs and an AltAz sit
// a zenith-distance-to-altitude subtraction and an azimuth wrap. Every one of
// those is a place where a correct model produces a wrong answer, and every
// one of them is invisible in a self-consistent round trip.
//
// # Why it exists
//
// Issue #256 opened on a ~0.5 arcsecond cross-track difference between
// astrogo's observed azimuth and Horizons'. #254 and #260 excluded, by
// measurement, every geocentric stage — ephemeris, light time, apparent right
// ascension, apparent declination, Earth rotation — and found polar motion
// reducing the residual rather than causing it. That left the topocentric step
// itself as the only remaining candidate.
//
// This test is what closed it. Feeding both sides the same topocentric
// astrometric place, over three real observatories, two planets and a year of
// epochs, astrogo and Atco13 agreed to 0.0000 arcseconds — worst case, not
// mean — while both differed from Horizons identically. Since a rotation
// preserves great-circle separation, and the same comparison put astrogo's
// apparent right ascension and declination within 0.049 arcseconds of Horizons
// while the horizon-frame separation was 0.380, the extra 0.33 arcseconds is
// in the equator-to-horizon rotation and nowhere else. Horizons rotates
// through SPICE's ITRF93 frame — its own response header says so — and astrogo
// through the IAU 2006/2000A CIO chain on IERS finals2000A. Switching polar
// motion off on the SOFA side moved Paranal to 0.012 arcseconds and Mauna Kea
// to 0.142 but left Greenwich at 0.452, so it is not one term either.
//
// The conclusion is that the residual is a difference between two Earth
// orientation realisations, not an astrogo defect — and the reason that
// conclusion is safe is the 0.0000 measured here. Which is exactly why it is
// worth a permanent, offline test rather than a paragraph.
//
// # Why it is offline
//
// Earth orientation is injected, not fetched. That is deliberate twice over:
// the assertion is about arithmetic, so pinning the inputs is what makes it an
// assertion about arithmetic; and a test that needed a bulletin would run in
// no CI job this repository has. The injected values are realistic rather than
// zero, because zero polar motion would leave the xpl/ypl terms in Apco13
// untested and those are precisely the ones a wiring error hides in.
func TestTopocentricPathIsSOFAsAtco13(t *testing.T) {
	// Not parallel: the EOP model is process-wide.
	t.Cleanup(time.ResetEOP)

	// Realistic magnitudes for 2026: DUT1 within the 0.9 s UTC bound, polar
	// motion a few tenths of an arcsecond.
	const (
		dut1Seconds = 0.1237
		xpArcsec    = 0.1834
		ypArcsec    = 0.4021
	)

	time.RegisterModel(fixedEOP{
		dut1: dut1Seconds,
		xp:   angle.Arcsec(xpArcsec).Radians(),
		yp:   angle.Arcsec(ypArcsec).Radians(),
	})

	// RefractionNone rather than a pressure: it makes NewContext pass zero
	// pressure to Apco13, which is the airless case Atco13 gets below, and
	// makes the model's own contribution exactly zero rather than small.
	atm := atmosphere.StandardRefraction
	atm.Model = atmosphere.RefractionNone{}

	type namedSite struct {
		name              string
		lon, lat, heightM float64
	}

	// Real observatories plus the geometric extremes: the equator, where the
	// polar-motion meridian term vanishes; a high latitude, where hour angle
	// projects into azimuth most steeply; and a site at the antimeridian,
	// where a longitude sign error stops cancelling.
	sites := []namedSite{
		{"Greenwich", 0, 51.4778, 46},
		{"Paranal", -70.4042, -24.6272, 2635},
		{"Mauna Kea", -155.4681, 19.8207, 4205},
		{"Equator", 100, 0, 0},
		{"High latitude", 15.65, 78.22, 78},
		{"Antimeridian", 179.95, -45, 12},
	}

	type namedTarget struct {
		name       string
		raHr, decD float64
	}

	// Directions chosen for the transform's awkward places rather than for
	// astronomical interest: the right-ascension wrap, both celestial poles,
	// and the celestial equator.
	targets := []namedTarget{
		{"Vega", 18.6156, 38.7837},
		{"Sirius", 6.7525, -16.7161},
		{"RA wrap", 0.0, 5.0},
		{"just before wrap", 23.9999, -5.0},
		{"north pole", 12.0, 89.9},
		{"south pole", 12.0, -89.9},
		{"equator", 9.0, 0.0},
	}

	epochs := []time.Time{
		time.Date(2026, time.January, 5, 3, 17, 41, 0, time.LocationUTC),
		time.Date(2026, time.March, 20, 12, 0, 0, 0, time.LocationUTC),
		time.Date(2026, time.June, 21, 18, 42, 9, 0, time.LocationUTC),
		time.Date(2026, time.September, 23, 0, 5, 33, 0, time.LocationUTC),
		time.Date(2026, time.December, 21, 21, 55, 2, 0, time.LocationUTC),
	}

	ref := metrology.Reference{
		Kind:    metrology.KindSOFA,
		Name:    "SOFA iauAtco13, via gofa",
		Version: "IAU 2006/2000A",
		Source:  "https://www.iausofa.org/",
		Dataset: "Earth orientation injected: " +
			"DUT1 0.1237 s, xp 0.1834\", yp 0.4021\"",

		// Both sides are the same three SOFA routines. Naming that here is
		// what keeps the generated accuracy table from reading this as
		// independent agreement.
		SharedAncestor: "gofa/SOFA — Apco13, Atciq and Atioq on both sides",
	}

	// The bound is not a physical tolerance. Both paths execute the same
	// arithmetic in the same order, so the only difference available is the
	// zenith-distance subtraction and the azimuth wrap on astrogo's side:
	// a few units in the last place of a radian, around 2e-11 arcseconds.
	// One microarcsecond leaves five orders of magnitude of headroom for
	// platform floating-point differences — CLAUDE.md's FMA caveat — while
	// staying far below any effect that could be mistaken for physics. A
	// residual above it is a wiring change, not a rounding one.
	contract := metrology.MustContract(1e-6, "arcsec",
		"astrogo's Apco13/Atciq/Atioq path and SOFA's Atco13 are the same three routines on "+
			"the same inputs, so agreement is exact up to the zenith-distance subtraction and "+
			"azimuth wrap astrogo adds — a few units in the last place of a radian, ~2e-11 "+
			"arcsec. 1 microarcsecond is five orders of magnitude of headroom for cross-platform "+
			"FMA differences and still far below any physical effect",
		"SOFA iauAtco13; astrogo coord/context.go NewContext, AstrometricToApparent, ApparentToObserved")

	// Two suites because they exercise two public entry points. The
	// step-by-step path is what a caller reusing a Context per epoch takes;
	// AstrometricToObserved collapses it into Atcoq and could drift from the
	// other without anything else noticing.
	stepwise := metrology.NewSuite("coord.topocentric.vs_sofa.stepwise", ref, contract)
	collapsed := metrology.NewSuite("coord.topocentric.vs_sofa.collapsed", ref, contract)

	for _, s := range sites {
		site, err := coord.NewGeodetic(angle.Deg(s.lon), angle.Deg(s.lat), s.heightM)
		if err != nil {
			t.Fatalf("%s: NewGeodetic: %v", s.name, err)
		}

		for _, epoch := range epochs {
			ctx := coord.NewContext(epoch, site, atm)
			utc1, utc2 := epoch.UTC().JDParts()

			for _, tg := range targets {
				ra, dec := angle.Hour(tg.raHr), angle.Deg(tg.decD)
				astrometric := coord.NewAstrometric(ra, dec)

				got := ctx.ApparentToObserved(ctx.AstrometricToApparent(astrometric))
				oneShot := ctx.AstrometricToObserved(astrometric)

				aob, zob, _, _, _, _, status := gofaext.Atco13(
					ra.Radians(), dec.Radians(),
					0, 0, 0, 0,
					utc1, utc2, dut1Seconds,
					site.Lon().Radians(), site.Lat().Radians(), site.Height(),
					angle.Arcsec(xpArcsec).Radians(), angle.Arcsec(ypArcsec).Radians(),
					0, atm.Temperature, atm.Humidity, atm.Wavelength,
				)
				if status < 0 {
					t.Fatalf("%s @ %s %s: Atco13 status %d",
						tg.name, s.name, epoch.Format(time.RFC3339), status)
				}

				want := coord.NewAltAz(angle.Rad(math.Pi/2-zob), angle.Rad(aob).Wrap360())

				label := tg.name + " @ " + s.name
				scenario := epoch.Format(time.RFC3339)

				stepwise.Add(metrology.Sample{
					Error:   horizonSeparationArcsec(got, want),
					Label:   label,
					Context: scenario,
				})
				collapsed.Add(metrology.Sample{
					Error:   horizonSeparationArcsec(oneShot, want),
					Label:   label,
					Context: scenario,
				})
			}
		}
	}

	if stepwise.Len() != len(sites)*len(epochs)*len(targets) {
		t.Fatalf("collected %d samples, expected %d — the matrix did not run in full",
			stepwise.Len(), len(sites)*len(epochs)*len(targets))
	}

	stepwise.Report(t)
	collapsed.Report(t)
}

// horizonSeparationArcsec is the great-circle angle between two horizon
// directions, in arcseconds.
//
// Separation rather than separate azimuth and altitude differences: near the
// zenith an azimuth difference is almost free, and near the horizon it is
// almost the whole error. One number that means the same thing everywhere is
// what a contract can be stated against.
//
// It does not reuse altAzSeparationArcsec from eopsensitivity_test.go, and the
// reason is the whole difficulty of this measurement. That helper takes an
// acos of the dot product, which is well conditioned for the ~13 arcsecond
// shifts it measures and useless here: for two nearly identical directions the
// cosine is 1 - eps^2/2, so float64 stops resolving eps somewhere around a
// milliarcsecond and returns a clean zero below it. Against a 1 microarcsecond
// contract that would not merely lose precision — it would report perfect
// agreement for a real defect up to a thousand times the bound.
//
// coord.Separation forms the great circle from the cross and dot products and
// takes an atan2, which stays accurate all the way down.
func horizonSeparationArcsec(a, b coord.AltAz) float64 {
	// coord.Separation takes any (longitude, latitude) pair through ICRS's
	// constructor; the spherical geometry underneath is frame-agnostic, and
	// no AltAz.Separation exists.
	return coord.Separation(
		coord.NewICRS(a.Az(), a.Alt()),
		coord.NewICRS(b.Az(), b.Alt()),
	).Arcseconds()
}
