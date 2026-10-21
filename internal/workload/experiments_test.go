//go:build integration

package workload_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/jpl"
	"github.com/TuSKan/astrogo/ephemeris/jpl/lsk"
	"github.com/TuSKan/astrogo/ephemeris/jpl/spk"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
	"github.com/TuSKan/astrogo/vector"
)

// The experiments behind docs/ephemeris.md. Each measures one question the
// design turns on and logs the answer (run with -v); each asserts only what
// makes its measurement trustworthy — a control, or the precision a
// floating-point argument predicts — never the size of an effect the design
// may later remove, since a test asserting a defect exists is a test that
// fails when the defect is fixed.

var (
	kmPerAU = constants.IAU.AstronomicalUnit.Value / 1e3
)

// TestNativeMoonAgreesWithTheBarycentricRoute: the kernel holds the Moon and
// the Earth relative to the Earth-Moon barycenter, and the provider reaches a
// geocentric Moon by carrying both to the solar-system barycenter and
// differencing. Evaluating the two native segments instead halves the
// evaluations (BenchmarkMoonAboutItsPlanet); this asks what it does to the
// answer. Both routes add the same polynomials in a different order, so they
// may differ only by rounding at the scale of the largest term — the
// barycentric route's 1 AU, about 3×10⁻⁸ km per operation.
func TestNativeMoonAgreesWithTheBarycentricRoute(t *testing.T) {
	for _, c := range []struct {
		name         string
		moon, planet int32
		kernel       string
		bound        float64 // km: rounding at the barycentric route's scale, with margin
	}{
		// 1 AU × 2⁻⁵³ ≈ 1.7×10⁻⁸ km; a few operations, ten times over.
		{"Moon-Earth", 301, 399, "", 1e-6},
		// Neptune's 30 AU, likewise.
		{"Triton-Neptune", 801, 899, "satellites/nep097.bsp", 3e-5},
	} {
		t.Run(c.name, func(t *testing.T) {
			p, f := openCounted(t)
			if c.kernel != "" {
				openCachedKernel(t, p, f, c.kernel)
			}

			var worst float64

			for i := range 100 {
				at := epoch.Add(unit.Days(3.65 * float64(i)))

				native, err := relativeNative(p, c.moon, c.planet, at)
				if err != nil {
					t.Fatalf("native at %v: %v", at, err)
				}

				m, err := eph.At(p, at).Geometric(core.ID(c.moon), eph.Earth)
				if err != nil {
					t.Fatalf("moon at %v: %v", at, err)
				}

				pl, err := eph.At(p, at).Geometric(core.ID(c.planet), eph.Earth)
				if err != nil {
					t.Fatalf("planet at %v: %v", at, err)
				}

				viaSSB := m.Pos.Sub(pl.Pos).MulScalar(kmPerAU)
				worst = math.Max(worst, native.Sub(viaSSB).Norm())
			}

			t.Logf("%s: native and barycentric routes differ by at most %.3g km over 2026-27", c.name, worst)

			if worst > c.bound {
				t.Errorf("%s: the routes differ by %.3g km, beyond rounding (%.0e km)", c.name, worst, c.bound)
			}
		})
	}
}

// TestObservedPlaceParallaxAndAberrationOrder measures what
// GeocentricToObserved's order costs. plan hands it the apparent geocentric
// vector — annual aberration already applied to the direction seen from the
// geocenter — and it then subtracts the observer's position. Rigorously the
// observer's own direction is what aberration acts on: the astrometric place
// is taken from the observer's barycentric position, and aberration and
// deflection apply to that (SOFA's iauAtciq after iauApco13).
//
// The difference scales as the parallax times v/c ≈ 10⁻⁴, so it is a few
// milliarcseconds for a planet and, for the Moon's 57′ horizontal parallax,
// tenths of an arcsecond. Mars is the control: if the two paths disagreed
// there beyond the deflection model's few milliarcseconds, the difference
// would not be the ordering and the Moon's number would mean nothing.
func TestObservedPlaceParallaxAndAberrationOrder(t *testing.T) {
	p, _ := openCounted(t)

	site, err := coord.NewGeodetic(angle.Deg(2.1686), angle.Deg(41.3874), unit.Meters(0))
	if err != nil {
		t.Fatalf("site: %v", err)
	}

	for _, c := range []struct {
		id      core.ID
		control float64 // arcsec bound when the body is the control, else 0
	}{
		{core.MarsBarycenter, 0.05},
		{core.Moon, 0},
	} {
		var (
			worst, sumSq, worstAlt float64
			n                      int
		)

		for h := range 30 * 24 {
			at := epoch.Add(unit.Hours(float64(h)))

			// A Context built for this instant rather than moved to it, so both
			// paths read state computed for exactly this epoch; and no
			// refraction, since the two refract through different routines and
			// only the geometry is under test.
			ctx := coord.NewContext(at, site, atmosphere.Refraction{})

			app, err := eph.At(p, at).Apparent(c.id, eph.Geocenter())
			if err != nil {
				t.Fatalf("%v at %v: apparent: %v", c.id, at, err)
			}

			today := ctx.GeocentricToObserved(app.Pos)
			if today.Alt().Degrees() < 5 {
				continue
			}

			if far, err := elongatedFromSun(p, c.id, at, 20); err != nil {
				t.Fatalf("%v at %v: elongation: %v", c.id, at, err)
			} else if !far {
				continue // star deflection near the Sun is not the finite-distance one
			}

			u, err := topocentricAstrometric(p, ctx, c.id, at)
			if err != nil {
				t.Fatalf("%v at %v: topocentric: %v", c.id, at, err)
			}

			ra, dec := u.ToSpherical()
			rigorous := ctx.AstrometricToObserved(coord.NewAstrometric(angle.Rad(ra), angle.Rad(dec)))

			d := separationArcsec(today, rigorous)
			sumSq += d * d
			n++

			if d > worst {
				worst, worstAlt = d, today.Alt().Degrees()
			}
		}

		if n == 0 {
			t.Fatalf("%v was never above 5° and 20° from the Sun in June 2026", c.id)
		}

		t.Logf("%v: %d epochs; observed-place difference max %.4f″ (at altitude %.1f°), rms %.4f″",
			c.id, n, worst, worstAlt, math.Sqrt(sumSq/float64(n)))

		if c.control > 0 && worst > c.control {
			t.Errorf("control %v: the two paths differ by %.4f″, beyond %.2f″ — the comparison itself is wrong",
				c.id, worst, c.control)
		}
	}
}

// TestEpv00EarthAgainstTheKernel measures the Earth a Kepler orbit is placed
// against. kepler.Provider turns a heliocentric state geocentric with SOFA's
// Epv00 Earth whatever its base is, so with a JPL base one place mixes two
// ephemerides: the Kepler body's Earth from Epv00, its Sun (for deflection
// and elongation) from the kernel. SOFA documents Epv00 against DE405 at
// 3.7 km rms and 11.2 km at worst over 1900-2100; this measures it against
// the kernel actually loaded, as the heliocentric Earth (Earth minus Sun)
// the Kepler path subtracts.
func TestEpv00EarthAgainstTheKernel(t *testing.T) {
	p, _ := openCounted(t)

	var worst, sumSq float64

	var worstAt time.Time

	const n = 2000

	start := time.Date(1900, time.January, 1, 0, 0, 0, 0, time.LocationUTC)

	for i := range n {
		at := start.Add(unit.Days(73049 * float64(i) / n)) // to 2100

		sun, err := eph.At(p, at).Geometric(core.Sun, eph.Earth)
		if err != nil {
			t.Fatalf("Sun at %v: %v", at, err)
		}

		d1, d2 := at.TDB().JDParts()

		pvh, _, status := gofaext.Epv00(d1, d2)
		if status < 0 {
			t.Fatalf("Epv00 at %v: status %d", at, status)
		}

		// The kernel's heliocentric Earth is minus its geocentric Sun.
		kernel := sun.Pos.MulScalar(-1)
		sofa := vector.V3(pvh[0][0], pvh[0][1], pvh[0][2])

		d := sofa.Sub(kernel).Norm() * kmPerAU
		sumSq += d * d

		if d > worst {
			worst, worstAt = d, at
		}
	}

	t.Logf("Epv00 heliocentric Earth against de440s, 1900-2100: rms %.2f km, max %.2f km (at %v)",
		math.Sqrt(sumSq/n), worst, worstAt)

	// The control: SOFA's own bound against DE405, with a factor of two for
	// DE405's own departure from DE440. Beyond it the comparison, not Epv00,
	// is wrong.
	if worst > 2*11.2 {
		t.Errorf("Epv00 differs from the kernel by %.2f km, beyond twice SOFA's documented 11.2 km", worst)
	}
}

// topocentricAstrometric is the astrometric place of id seen from the site:
// the body at emission time from the observer's barycentric position at
// reception, light time iterated from the observer, in km.
//
// The barycentric states come from the kernel's own segment chains, since the
// provider answers only geocentric states. An ID is the NAIF code the chains
// are indexed by (#809).
func topocentricAstrometric(p *jpl.Provider, ctx *coord.Context, id core.ID, at time.Time) (vector.Vec3, error) {
	naif := int32(id)

	et := lsk.UTCToET(at, p.LSK)

	earth, err := barycentric(p, 399, et)
	if err != nil {
		return vector.Vec3{}, err
	}

	observer := earth.Add(ctx.ObsVec().MulScalar(kmPerAU))
	lightKmPerSec := constants.SI2019.SpeedOfLight.Value / 1e3

	var u vector.Vec3

	tau := 0.0 // seconds

	for range 6 {
		body, err := barycentric(p, naif, et-tau)
		if err != nil {
			return vector.Vec3{}, err
		}

		u = body.Sub(observer)
		tau = u.Norm() / lightKmPerSec
	}

	return u, nil
}

// barycentric is target's position from the solar-system barycenter at et,
// in km, summed along the kernel's own chain of segment centers.
func barycentric(p *jpl.Provider, target int32, et float64) (vector.Vec3, error) {
	var sum vector.Vec3

	for id := target; id != 0; {
		ref, err := p.FindSegment(id, et)
		if err != nil {
			return vector.Vec3{}, err //nolint:wrapcheck // FindSegment's error names the target
		}

		k := p.Kernels[ref.KernelIndex]
		s := &k.Segments[ref.SegmentIndex]

		pos, _, err := spk.EvaluateSegment(s, k.Reader, et)
		if err != nil {
			return vector.Vec3{}, err //nolint:wrapcheck // the reader's error describes the segment
		}

		sum = sum.Add(pos)
		id = s.Center
	}

	return sum, nil
}

// elongatedFromSun reports whether id is at least minDeg from the Sun.
func elongatedFromSun(p core.Provider, id core.ID, at time.Time, minDeg float64) (bool, error) {
	body, err := eph.At(p, at).Geometric(id, eph.Earth)
	if err != nil {
		return false, err //nolint:wrapcheck // the caller names the body and epoch
	}

	sun, err := eph.At(p, at).Geometric(core.Sun, eph.Earth)
	if err != nil {
		return false, err //nolint:wrapcheck // the caller names the body and epoch
	}

	cos := body.Pos.Dot(sun.Pos) / (body.Pos.Norm() * sun.Pos.Norm())

	return math.Acos(math.Max(-1, math.Min(1, cos))) >= minDeg*math.Pi/180, nil
}

// separationArcsec is the angle between two observed places, in arcseconds.
func separationArcsec(a, b coord.AltAz) float64 {
	unitVec := func(c coord.AltAz) vector.Vec3 {
		sinAlt, cosAlt := math.Sincos(c.Alt().Radians())
		sinAz, cosAz := math.Sincos(c.Az().Radians())

		return vector.Vec3{X: cosAlt * cosAz, Y: cosAlt * sinAz, Z: sinAlt}
	}

	u, v := unitVec(a), unitVec(b)

	return math.Atan2(u.Cross(v).Norm(), u.Dot(v)) * 180 / math.Pi * 3600
}

// BenchmarkSegmentEvaluationParts splits one segment evaluation into what a
// record cache and a position-only path would each remove: the two reads (the
// directory, then the record), and the Chebyshev sums with and without the
// derivative.
func BenchmarkSegmentEvaluationParts(b *testing.B) {
	p, _ := openCounted(b)

	et := lsk.UTCToET(epoch, p.LSK)

	ref, err := p.FindSegment(301, et)
	if err != nil {
		b.Fatalf("Moon segment: %v", err)
	}

	k := p.Kernels[ref.KernelIndex]
	s := &k.Segments[ref.SegmentIndex]

	meta, err := k.Reader.ReadDoubles(s.EndAddr-3, s.EndAddr)
	if err != nil {
		b.Fatalf("directory: %v", err)
	}

	tInit, tLen, rSize := meta[0], meta[1], int32(meta[2])
	recStart := s.StartAddr + int32((et-tInit)/tLen)*rSize

	rec, err := k.Reader.ReadDoubles(recStart, recStart+rSize-1)
	if err != nil {
		b.Fatalf("record: %v", err)
	}

	nCoeffs := int((rSize - 2) / 3)
	coeffs := rec[2 : 2+nCoeffs]
	tau := (et - rec[0]) / rec[1]

	b.Run("read-directory", func(b *testing.B) {
		b.ReportAllocs()

		for b.Loop() {
			if _, err := k.Reader.ReadDoubles(s.EndAddr-3, s.EndAddr); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("read-record", func(b *testing.B) {
		b.ReportAllocs()

		for b.Loop() {
			if _, err := k.Reader.ReadDoubles(recStart, recStart+rSize-1); err != nil {
				b.Fatal(err)
			}
		}
	})

	for _, deriv := range []bool{false, true} {
		name := "chebyshev-position"
		if deriv {
			name = "chebyshev-state"
		}

		b.Run(name, func(b *testing.B) {
			for b.Loop() {
				for range 3 { // three axes
					spk.EvalChebyshev(coeffs, tau, rec[1], deriv)
				}
			}

			b.ReportMetric(float64(nCoeffs), "coeffs")
		})
	}

	b.Run("evaluate-segment", func(b *testing.B) {
		b.ReportAllocs()

		for b.Loop() {
			if _, _, err := spk.EvaluateSegment(s, k.Reader, et); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkType21Evaluation measures a Horizons small-body segment (SPK type
// 21), whose evaluation reads its epoch table on every call, against the
// planetary type 2 record above. It uses a cached Horizons kernel only.
func BenchmarkType21Evaluation(b *testing.B) {
	p, f := openCounted(b)
	openCachedKernel(b, p, f, "smallbody/1000026.bsp")

	var seg *spk.Segment

	var r *spk.Reader

	for _, k := range p.Kernels {
		for i := range k.Segments {
			if k.Segments[i].Type == 21 {
				seg, r = &k.Segments[i], k.Reader
			}
		}
	}

	if seg == nil {
		b.Fatal("smallbody/1000026.bsp holds no type 21 segment")
	}

	et := (seg.StartET + seg.EndET) / 2

	if _, _, err := spk.EvaluateSegment(seg, r, et); err != nil {
		b.Fatalf("type 21: %v", err)
	}

	// Reads, not evaluations: type 21 does not read twice per evaluation.
	f.reads.Store(0)
	b.ReportAllocs()

	for b.Loop() {
		if _, _, err := spk.EvaluateSegment(seg, r, et); err != nil {
			b.Fatalf("type 21: %v", err)
		}
	}

	b.ReportMetric(float64(f.reads.Load())/float64(b.N), "reads/op")
}
