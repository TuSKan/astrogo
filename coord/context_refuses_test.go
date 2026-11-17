package coord_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// refusalSite and refusalEpoch are a real site and instant, so a refusal in
// the tests below is the bad argument's alone.
var (
	refusalSite  = coord.MustGeodetic(angle.Deg(-70.40417), angle.Deg(-24.62722), 2635) // Paranal
	refusalEpoch = time.Date(2026, time.June, 15, 22, 0, 0, 0, time.LocationUTC)
)

// TestNewContextRefusesWhatItCannotBuildFrom: a nil site is ErrNilGeodetic
// and a time that is not finite is ErrNotFinite, from NewContext and from
// NewReducer, which builds one. NewContext used to return a Context with no
// error: a nil site panicked inside it, and a NaN time made every place NaN
// (#1186). Air that is not air is TestContextRefusesAirThatIsNot.
func TestNewContextRefusesWhatItCannotBuildFrom(t *testing.T) {
	t.Parallel()

	if ctx, err := coord.NewContext(refusalEpoch, nil, atmosphere.Refraction{}); ctx != nil || !errors.Is(err, coord.ErrNilGeodetic) {
		t.Errorf("NewContext with a nil site = %v, %v, want ErrNilGeodetic", ctx, err)
	}

	if r, err := coord.NewReducer(nil, refusalEpoch, atmosphere.Refraction{}); r != nil || !errors.Is(err, coord.ErrNilGeodetic) {
		t.Errorf("NewReducer with a nil site = %v, %v, want ErrNilGeodetic", r, err)
	}

	for _, jd := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		at := time.FromJD(jd, time.UTC)

		if ctx, err := coord.NewContext(at, refusalSite, atmosphere.Refraction{}); ctx != nil || !errors.Is(err, coord.ErrNotFinite) {
			t.Errorf("NewContext at FromJD(%v) = %v, %v, want ErrNotFinite", jd, ctx, err)
		}

		if r, err := coord.NewReducer(refusalSite, at, atmosphere.Refraction{}); r != nil || !errors.Is(err, coord.ErrNotFinite) {
			t.Errorf("NewReducer at FromJD(%v) = %v, %v, want ErrNotFinite", jd, r, err)
		}
	}
}

// TestTransformsRefuseWhatIsNotFinite: ICRSToAltAz and ICRSToHourAngle refuse
// a position or kinematics that is not finite, and AltAzToICRS an altitude or
// azimuth, with ErrNotFinite. Each returned an error that could never fire,
// and a NaN place with it (#1186). The same transforms of finite input still
// answer, so the refusals are the input's.
func TestTransformsRefuseWhatIsNotFinite(t *testing.T) {
	t.Parallel()

	ctx := mustContext(t, refusalEpoch, refusalSite, atmosphere.StandardRefraction())

	nan, inf := math.NaN(), math.Inf(1)
	ra, dec := angle.Hour(16.5), angle.Deg(-26.4) // Antares, high over Paranal
	pm, plx := angle.Arcsec(-0.010), angle.Arcsec(0.0059)
	rv := unit.KmPerSec(-3.4)

	for _, c := range []struct {
		name string
		icrs coord.ICRS
	}{
		{"NaN right ascension", coord.NewICRS(angle.Rad(nan), dec)},
		{"infinite declination", coord.NewICRS(ra, angle.Rad(inf))},
		{"NaN proper motion in RA", coord.NewICRSWithKinematics(ra, dec, angle.Rad(nan), pm, plx, rv)},
		{"infinite proper motion in Dec", coord.NewICRSWithKinematics(ra, dec, pm, angle.Rad(-inf), plx, rv)},
		{"NaN parallax", coord.NewICRSWithKinematics(ra, dec, pm, pm, angle.Rad(nan), rv)},
		{"NaN radial velocity", coord.NewICRSWithKinematics(ra, dec, pm, pm, plx, unit.Velocity(nan))},
	} {
		if aa, err := ctx.ICRSToAltAz(c.icrs); !errors.Is(err, coord.ErrNotFinite) {
			t.Errorf("ICRSToAltAz, %s: %v, %v, want ErrNotFinite", c.name, aa, err)
		}

		if ha, err := ctx.ICRSToHourAngle(c.icrs); !errors.Is(err, coord.ErrNotFinite) {
			t.Errorf("ICRSToHourAngle, %s: %v, %v, want ErrNotFinite", c.name, ha, err)
		}
	}

	for _, c := range []struct {
		name    string
		alt, az angle.Angle
	}{
		{"NaN altitude", angle.Rad(nan), angle.Deg(120)},
		{"infinite azimuth", angle.Deg(40), angle.Rad(inf)},
	} {
		if icrs, err := ctx.AltAzToICRS(coord.NewAltAz(c.alt, c.az)); !errors.Is(err, coord.ErrNotFinite) {
			t.Errorf("AltAzToICRS, %s: %v, %v, want ErrNotFinite", c.name, icrs, err)
		}
	}

	star := coord.NewICRSWithKinematics(ra, dec, pm, pm, plx, rv)

	aa, err := ctx.ICRSToAltAz(star)
	if err != nil || math.IsNaN(aa.Alt().Radians()) {
		t.Fatalf("ICRSToAltAz of a finite star: %v, %v", aa, err)
	}

	if ha, err := ctx.ICRSToHourAngle(star); err != nil || math.IsNaN(ha.Radians()) {
		t.Errorf("ICRSToHourAngle of a finite star: %v, %v", ha, err)
	}

	if back, err := ctx.AltAzToICRS(aa); err != nil || math.IsNaN(back.RA().Radians()) {
		t.Errorf("AltAzToICRS of a finite place: %v, %v", back, err)
	}
}
