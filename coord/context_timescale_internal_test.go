package coord

import (
	"math"
	"reflect"
	"testing"

	"github.com/hebl/gofa"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// timescaleSite is the site the time-scale tests share.
func timescaleSite(t *testing.T) (*Geodetic, atmosphere.Refraction) {
	t.Helper()

	loc, err := NewGeodetic(angle.Deg(-70.4), angle.Deg(-24.6), 2635)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	return loc, atmosphere.AtAltitude(2635)
}

// sofaStepDays are the eleven days on which SOFA's TAI−UTC differs between
// that day's 0h and the next, and which Utctai and Utcut1 therefore treat as
// stretched, at an hour late enough in each for the stretch to matter.
var sofaStepDays = []time.Time{
	time.Date(1959, time.December, 31, 20, 57, 0, 0, time.LocationUTC),
	time.Date(1961, time.July, 31, 21, 0, 0, 0, time.LocationUTC),
	time.Date(1963, time.October, 31, 21, 0, 0, 0, time.LocationUTC),
	time.Date(1964, time.March, 31, 21, 0, 0, 0, time.LocationUTC),
	time.Date(1964, time.August, 31, 21, 0, 0, 0, time.LocationUTC),
	time.Date(1964, time.December, 31, 21, 0, 0, 0, time.LocationUTC),
	time.Date(1965, time.February, 28, 21, 0, 0, 0, time.LocationUTC),
	time.Date(1965, time.June, 30, 21, 0, 0, 0, time.LocationUTC),
	time.Date(1965, time.August, 31, 21, 0, 0, 0, time.LocationUTC),
	time.Date(1968, time.January, 31, 21, 0, 0, 0, time.LocationUTC),
	time.Date(1971, time.December, 31, 21, 0, 0, 0, time.LocationUTC),
}

// TestContextRotatesStarsByItsOwnUT1 is #474's defect, stated as the property
// the fix gives: the Earth rotation in a Context's astrometry, the one the
// stellar path applies, is the rotation of the Context's own UT1, the one its
// vector path and AtTime apply.
//
// They were different Earths. The astrometry came from SOFA's Apco13, which
// derives UT1 from UTC itself, and on the eleven days SOFA stretches for a
// step in TAI−UTC that differs from astrogo's by up to 0.82 s: a star and the
// Moon in one Context were rotated 11.8″ apart on 1959-12-31.
func TestContextRotatesStarsByItsOwnUT1(t *testing.T) {
	t.Parallel()

	loc, atm := timescaleSite(t)

	for _, epoch := range append(sofaStepDays,
		time.Date(1900, time.June, 1, 3, 0, 0, 0, time.LocationUTC),
		time.Date(2016, time.December, 31, 23, 30, 0, 0, time.LocationUTC),
		time.Date(2026, time.June, 15, 3, 0, 0, 0, time.LocationUTC),
	) {
		ctx := NewContext(epoch, loc, atm)

		ut1, ut2 := epoch.UTC().UT1Using(ctx.eop.DUT1).JDParts()
		want := gofaext.Era00(ut1, ut2) + ctx.astrom.Along

		if d := math.Abs(math.Remainder(ctx.astrom.Eral-want, 2*math.Pi)); d > 1e-15 {
			t.Errorf("%v: the stellar path rotates the Earth %.3g rad (%.3g s of UT1) away from the Context's own UT1",
				epoch, d, d/7.292115e-5)
		}
	}
}

// TestContextAstrometryUnchangedFrom1972 holds the unification to changing
// nothing where the two derivations already agreed: from 1972 on, a Context's
// astrometry and equation of the origins are what SOFA's Apco13 builds from
// the UTC date itself, bit for bit.
//
// On a leap-second day the numbers that depend on Earth rotation are allowed
// their last bits: astrogo and SOFA form UT1 on an 86401-second day by
// different arithmetic that rounds differently, and the observer's position
// and velocity follow the rotation angle. Measured, the angle moves by
// 2.1e-14 rad on 2016-12-31, which is 3e-10 s of UT1.
func TestContextAstrometryUnchangedFrom1972(t *testing.T) {
	t.Parallel()

	loc, atm := timescaleSite(t)

	epochs := []time.Time{
		time.Date(1972, time.June, 30, 23, 30, 0, 0, time.LocationUTC),
		time.Date(2016, time.December, 31, 23, 30, 0, 0, time.LocationUTC),
		time.Date(2016, time.December, 31, 12, 0, 0, 0, time.LocationUTC),
	}

	// 1972 to 2100 in steps of 23.7 days.
	for jd := 2441317.5; jd < 2488069.5; jd += 23.7 {
		epochs = append(epochs, time.FromJD(jd, time.UTC))
	}

	for _, epoch := range epochs {
		ctx := NewContext(epoch, loc, atm)

		utc1, utc2 := epoch.UTC().JDParts()

		var (
			want   gofa.ASTROM
			wantEO float64
		)

		gofa.Apco13(utc1, utc2, ctx.eop.DUT1,
			loc.Lon().Radians(), loc.Lat().Radians(), loc.Height().Meters(),
			ctx.eop.XP, ctx.eop.YP,
			atm.Pressure, atm.Temperature, atm.Humidity, atm.Wavelength,
			&want, &wantEO)

		if ctx.eo != wantEO {
			t.Fatalf("%v: the equation of the origins is not Apco13's", epoch)
		}

		if leapSecondDay(epoch) {
			if d := maxDiff(reflect.ValueOf(ctx.astrom), reflect.ValueOf(want)); d > 1e-13 {
				t.Fatalf("%v, a leap-second day: the astrometry differs from Apco13's by %.3g", epoch, d)
			}

			continue
		}

		if ctx.astrom != want {
			t.Fatalf("%v: from 1972 on a Context's astrometry must be Apco13's from the UTC date, bit for bit", epoch)
		}
	}
}

// leapSecondDay reports whether t's UTC day ends in a leap second, by SOFA's
// table: TAI−UTC a whole second larger at the next day's 0h.
func leapSecondDay(t time.Time) bool {
	y, m, d, _, _ := gofaext.JdToDate(t.UTC().JDParts())
	ny, nm, nd, _, _ := gofaext.JdToDate(time.Date(y, time.Month(m), d, 12, 0, 0, 0, time.LocationUTC).Add(unit.Days(1)).JDParts())

	var today, tomorrow float64

	gofa.Dat(y, m, d, 0, &today)
	gofa.Dat(ny, nm, nd, 0, &tomorrow)

	return tomorrow-today == 1
}

// maxDiff is the largest difference between corresponding numbers of two
// values of the same type, through arrays and struct fields.
func maxDiff(a, b reflect.Value) float64 {
	switch a.Kind() { //nolint:exhaustive // ASTROM holds only floats, arrays of them, and structs of those
	case reflect.Float64:
		return math.Abs(a.Float() - b.Float())
	case reflect.Array:
		var d float64
		for i := range a.Len() {
			d = math.Max(d, maxDiff(a.Index(i), b.Index(i)))
		}

		return d
	case reflect.Struct:
		var d float64
		for i := range a.NumField() {
			d = math.Max(d, maxDiff(a.Field(i), b.Field(i)))
		}

		return d
	}

	return 0
}

// TestAltAzToICRSInvertsICRSToAltAz: the inverse now runs on the Context's
// own astrometry, as the forward transform does. It used to rebuild its own
// through Atoc13, at SOFA's time scales rather than the Context's, so on the
// step days the two were not each other's inverse.
func TestAltAzToICRSInvertsICRSToAltAz(t *testing.T) {
	t.Parallel()

	loc, atm := timescaleSite(t)
	stars := []ICRS{
		NewICRS(angle.Hour(5.5), angle.Deg(-5.4)),
		NewICRS(angle.Hour(16.49), angle.Deg(-26.43)),
		NewICRS(angle.Hour(1.0), angle.Deg(-60)),
	}

	for _, epoch := range append(sofaStepDays, time.Date(2026, time.June, 15, 3, 0, 0, 0, time.LocationUTC)) {
		ctx := NewContext(epoch, loc, atm)

		for _, star := range stars {
			aa, err := ctx.ICRSToAltAz(star)
			if err != nil {
				t.Fatalf("ICRSToAltAz: %v", err)
			}

			// SOFA's refraction inverse in Atoiq is approximate near the
			// horizon: measured, 48 mas at 5° of altitude and 0.002 mas at 48°.
			// That is SOFA's, and Atoc13 used the same Atoiq.
			if aa.Alt().Degrees() < 30 {
				continue
			}

			back, err := ctx.AltAzToICRS(aa)
			if err != nil {
				t.Fatalf("AltAzToICRS: %v", err)
			}

			if sep := gofaext.Seps(star.RA().Radians(), star.Dec().Radians(), back.RA().Radians(), back.Dec().Radians()); sep > 0.05/3600e3*math.Pi/180 {
				t.Errorf("%v: %v came back %.3g mas away", epoch, star, sep*180/math.Pi*3600e3)
			}
		}
	}
}
