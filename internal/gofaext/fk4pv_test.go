package gofaext

import (
	"math"
	"testing"

	"github.com/hebl/gofa"
)

// A star at rest in FK4 at RA 123.4, Dec 0 — the case #339 tabulates — with
// the proper motion the FK4 -> FK5 leg gives it, expressed as SOFA wants it:
// dRA/dt and dDec/dt in radians per Julian year.
const (
	atRestRA   = 2.1537320412337
	atRestDec  = 0.0
	atRestDRdt = 1.6952e-3 / 206264.806247
	atRestDDdt = 2.4196e-3 / 206264.806247
)

// TestFK4GeometryDoesNotDependOnDistance is the measurement the distance-free
// route in fk4pv.go rests on, and the reason it is exact rather than an
// approximation.
//
// iauFk524 builds its pv-vector with a radius of 1.0, so the parallax reaches
// the geometry only through the radial rate w = rv*px*VF. With no radial
// velocity that term is zero whatever the parallax is, and the direction and
// proper motion that come out are therefore the same numbers — not merely
// close, the same bits.
//
// Which is what makes running the transformation with no parallax a lossless
// substitution for running it with an unusable one: nothing about the answer
// changes except that the radial velocity is no longer divided by a distance
// nobody knows.
func TestFK4GeometryDoesNotDependOnDistance(t *testing.T) {
	t.Parallel()

	var r0, d0, ur0, ud0, px0, rv0 float64

	gofa.Fk524(atRestRA, atRestDec, atRestDRdt, atRestDDdt, 0, 0,
		&r0, &d0, &ur0, &ud0, &px0, &rv0)

	for _, px := range []float64{1e-9, 1e-7, 1e-5, 1e-3, 1e-2, 1e-1, 1} {
		var r, d, ur, ud, pxOut, rv float64

		gofa.Fk524(atRestRA, atRestDec, atRestDRdt, atRestDDdt, px, 0,
			&r, &d, &ur, &ud, &pxOut, &rv)

		for _, f := range []struct {
			name     string
			got, at0 float64
		}{
			{"RA", r, r0},
			{"Dec", d, d0},
			{"dRA/dt", ur, ur0},
			{"dDec/dt", ud, ud0},
		} {
			if f.got != f.at0 {
				t.Errorf("parallax %g: %s is %.17g against %.17g with no parallax "+
					"(by %.3g). The distance-free route in this package assumes these "+
					"are identical, and it is no longer entitled to.",
					px, f.name, f.got, f.at0, math.Abs(f.got-f.at0))
			}
		}
	}
}

// TestFK4DoesNotInventARadialVelocity is #339 at the layer it happens in.
//
// Every row here is a star declared at rest with a parallax too small to be a
// distance. SOFA answers each with a radial velocity proportional to 1/px —
// 38.9 km/s at 1e-9 — because that is what dividing a frame artifact by an
// unusable parallax produces. The wrappers in fk4pv.go return zero, which is
// what was passed in.
func TestFK4DoesNotInventARadialVelocity(t *testing.T) {
	t.Parallel()

	for _, px := range []float64{0, 1e-12, 1e-9, 1e-8, 1e-7} {
		_, _, _, _, pxOut, rv := Fk524(atRestRA, atRestDec, atRestDRdt, atRestDDdt, px, 0)

		if rv != 0 {
			t.Errorf("parallax %g: a star at rest came back at %+.6g km/s, want 0", px, rv)
		}

		// The parallax is not changed by looking at the star from another
		// equinox, and the distance-free route must not quietly drop it.
		if pxOut != px {
			t.Errorf("parallax %g came back as %g", px, pxOut)
		}
	}
}

// TestFK4KeepsSOFAsAnswerForARealStar is the other half of the contract: the
// dispatch must not have widened into "always take the distance-free route".
//
// A star at a parallax a catalogue really contains keeps SOFA's answer,
// relativistic terms and light-time distortion included, because at that
// distance SOFA's answer is the better one.
func TestFK4KeepsSOFAsAnswerForARealStar(t *testing.T) {
	t.Parallel()

	// 0.1 arcsec is 10 pc, and 30 km/s is an ordinary space velocity.
	const (
		px = 0.1
		rv = 30.0
	)

	var wantR, wantD, wantUR, wantUD, wantPX, wantRV float64

	gofa.Fk524(atRestRA, atRestDec, atRestDRdt, atRestDDdt, px, rv,
		&wantR, &wantD, &wantUR, &wantUD, &wantPX, &wantRV)

	gotR, gotD, gotUR, gotUD, gotPX, gotRV := Fk524(atRestRA, atRestDec, atRestDRdt, atRestDDdt, px, rv)

	for _, f := range []struct {
		name      string
		got, want float64
	}{
		{"RA", gotR, wantR},
		{"Dec", gotD, wantD},
		{"dRA/dt", gotUR, wantUR},
		{"dDec/dt", gotUD, wantUD},
		{"parallax", gotPX, wantPX},
		{"radial velocity", gotRV, wantRV},
	} {
		if f.got != f.want {
			t.Errorf("%s = %.17g, want SOFA's own %.17g — a star at 10 pc must not "+
				"be taking the distance-free route", f.name, f.got, f.want)
		}
	}
}

// TestFK4RoundTripClosesAtEveryParallax is the property #339 is named for.
//
// FK4 -> FK5 -> FK4 on a star at rest returns it at rest, at parallaxes
// spanning from a real catalogue's down to ones that describe no distance at
// all. Before this it went to 38.9 km/s at 1e-9 arcsec, growing without bound
// as 1/px.
//
// # Two different claims, because there are two different regimes
//
// Where the parallax describes no usable distance the fabricated velocity is
// gone outright, and zero is the whole contract: nothing was measured, so
// nothing is reported.
//
// Where it describes a real one SOFA answers, and its answer carries a
// residual of its own — the frame artifact really does imply a small space
// velocity at a known distance, and at 1e-5 arcsec that is 3.9 mm/s. That is
// not this package's to remove, and #339 states the same bound: at any
// parallax a real catalogue contains, under 4e-3 km/s.
func TestFK4RoundTripClosesAtEveryParallax(t *testing.T) {
	t.Parallel()

	// Below this the distance is not a distance and the answer must be exact.
	// Above it SOFA answers and its own residual stands.
	const noDistanceAbove = 1e-6

	for _, px := range []float64{0, 1e-9, 1e-7, 1e-5, 1e-3, 1e-2, 1e-1} {
		r5, d5, dr5, dd5, px5, rv5 := Fk425(atRestRA, atRestDec, 0, 0, px, 0)
		_, _, _, _, pxBack, rvBack := Fk524(r5, d5, dr5, dd5, px5, rv5)

		switch {
		case px < noDistanceAbove:
			if rvBack != 0 {
				t.Errorf("parallax %g describes no distance, so a star at rest must come "+
					"back at exactly 0 km/s, not %+.6g", px, rvBack)
			}
		default:
			// #339's own bound, and it falls as 1/px from here.
			if math.Abs(rvBack) > 4e-3 {
				t.Errorf("parallax %g: a star at rest came back at %+.6g km/s, above the "+
					"4e-3 km/s this is contracted to", px, rvBack)
			}
		}

		// The parallax is not changed by looking at the star from another
		// equinox. SOFA's own route reconstructs it through the pv-vector and
		// returns it a few parts in 1e11 different, which is its residual and
		// not this package's business.
		if math.Abs(pxBack-px) > 1e-10*math.Max(px, 1e-9) {
			t.Errorf("parallax %g came back as %.17g", px, pxBack)
		}
	}
}

// TestFk52hDoesNotInventARadialVelocityInTheBand is the second half of #339,
// and the reason maxStellarSpeed exists.
//
// #331 taught the FK5 <-> Hipparcos dispatch to recognize the two things
// iauStarpv says it had to override. Between them and a usable parallax there
// is a band where it overrides nothing, reports success, and still describes a
// star crossing the sky at a significant fraction of light speed — because the
// parallax it was handed puts the star at 10 Mpc and the proper motion is
// FK4's own fictitious 2.4 mas/yr.
//
// Measured before the speed test was added: -9046 km/s at a parallax of
// exactly PXMIN, with a status of zero.
func TestFk52hDoesNotInventARadialVelocityInTheBand(t *testing.T) {
	t.Parallel()

	// Parallaxes spanning the band: at SOFA's own minimum, just above it, and
	// far enough above that the implied speed is merely large.
	for _, px := range []float64{1e-7, 1.1e-7, 1.5e-7, 5e-7, 1e-6} {
		_, _, _, _, pxOut, rv := Fk52h(atRestRA, atRestDec, atRestDRdt, atRestDDdt, px, 0)

		if rv != 0 {
			t.Errorf("parallax %g: a star with no radial velocity came back at %+.6g km/s. "+
				"iauStarpv reports success at this parallax, so only the speed test "+
				"stands between this and a fabricated measurement.", px, rv)
		}

		if pxOut != px {
			t.Errorf("parallax %g came back as %.17g", px, pxOut)
		}
	}
}
