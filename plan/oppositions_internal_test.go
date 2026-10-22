package plan

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
	"github.com/TuSKan/astrogo/vector"
)

// conicState is the two-body state at true anomaly nu on the conic of
// perihelion distance q, eccentricity e, inclination i, ascending node at
// node and argument of perihelion w about a center of parameter mu: the closed form, a
// producer independent of the code under test.
func conicState(q, e, i, node, w, mu, nu float64) (r, v vector.Vec3) {
	p := q * (1 + e)
	rad := p / (1 + e*math.Cos(nu))
	k := math.Sqrt(mu / p)

	r = vector.Vec3{X: rad * math.Cos(nu), Y: rad * math.Sin(nu)}
	v = vector.Vec3{X: -k * math.Sin(nu), Y: k * (e + math.Cos(nu))}

	turn := func(u vector.Vec3) vector.Vec3 { return u.RotateZ(w).RotateX(i).RotateZ(node) }

	return turn(r), turn(v)
}

// TestLongitudeCurvatureBoundOnConics holds longitudeCurvatureBound to the
// exact rate of change of a two-body orbit's longitude rate, which on such an
// orbit is −2·h_z·(x·ẋ + y·ẏ)/ρ⁴, at every true anomaly of ellipses,
// parabolas and hyperbolas, prograde and retrograde, in and out of the
// ecliptic. Two things are required of the bound: that it hold, and that it
// be an invariant of the orbit, the same from every point of it.
//
// Without its inclination term the bound fails here, on the inclined
// circles, whose longitude rate changes only by the projection; that is the
// canary this was checked with.
func TestLongitudeCurvatureBoundOnConics(t *testing.T) {
	t.Parallel()

	const mu = 2.9591220828559115e-4 // the Sun's, AU³/day²

	worst := 0.0

	for _, e := range []float64{0, 0.0167, 0.2, 0.6, 0.95, 1, 1.4} {
		for _, iDeg := range []float64{0, 3, 20, 60, 89, 120, 165, 180} {
			for _, w := range []float64{0, 1.1, 2.5} {
				i := iDeg * math.Pi / 180
				q := 0.8

				// The range of true anomaly the conic reaches.
				nuMax := math.Pi
				if e >= 1 {
					nuMax = math.Acos(-1/e) * 0.999
				}

				r0, v0 := conicState(q, e, i, 0.7, w, mu, 0)
				bound := longitudeCurvatureBound(r0, v0, mu)

				// Rounding: what the bound's own inputs carry, at the
				// orbit's scale of curvature.
				slack := 1e-12 * mu / (q * q * q)

				for nu := -nuMax; nu <= nuMax; nu += nuMax / 500 {
					r, v := conicState(q, e, i, 0.7, w, mu, nu)

					if b := longitudeCurvatureBound(r, v, mu); math.Abs(b-bound) > 1e-9*bound+slack {
						t.Fatalf("e %g, i %g°, ω %g: the bound is %g at ν = 0 and %g at ν = %.3f", e, iDeg, w, bound, b, nu)
					}

					hz := r.X*v.Y - r.Y*v.X
					rho2 := r.X*r.X + r.Y*r.Y
					curv := math.Abs(-2 * hz * (r.X*v.X + r.Y*v.Y) / (rho2 * rho2))

					if curv > bound*(1+1e-12)+slack {
						t.Errorf("e %g, i %g°, ω %g, ν %.3f: |L''| = %g exceeds the bound %g", e, iDeg, w, nu, curv, bound)
					}

					if bound > 0 {
						worst = math.Max(worst, curv/bound)
					}
				}
			}
		}
	}

	// The bound is not slack everywhere: some orbit comes within a factor of
	// two of it.
	if worst < 0.5 {
		t.Errorf("the largest |L''| reached is %.3f of the bound", worst)
	}
}

// TestLongitudeCurvatureBoundHolds measures what perturbationAllowance must
// cover: how far the rate of the scan's longitude difference changes faster
// than the two-body bound allows, through the scan's own samples, for the
// Earth alone (the difference to a star), for planets from Mercury out, and
// for the Moon. The Moon's pull on the Earth, which carries it about their
// barycentre each month, takes the Earth past its bound by 13%, and nothing
// here passes its bound by more; doubling it leaves room to spare.
func TestLongitudeCurvatureBoundHolds(t *testing.T) {
	t.Parallel()

	prov := eph.Default()
	sun := NewSun(prov)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.LocationUTC)

	// span and step in days; d is the half-width of the central difference.
	span := map[bool]float64{false: 4 * 365, true: 120}[testing.Short()]

	for _, c := range []struct {
		target     Observable
		span, step float64
		d          float64
		over       bool // the bound is known to be passed: by the Earth's wobble
	}{
		{NewStar("a star", angle.Deg(0), angle.Deg(0)), span, 0.25, 0.1, true},
		{NewMercury(prov), span, 0.5, 0.1, false},
		{NewVenus(prov), span, 1, 0.1, false},
		{NewMars(prov), span, 1, 0.1, false},
		{NewJupiter(prov), span, 1, 0.1, true},
		{NewSaturn(prov), span, 1, 0.1, true},
		{NewMoon(prov), span / 4, 1.0 / 24, 1.0 / 96, false},
	} {
		scan, err := newOppositionScan(c.target, sun, start)
		if err != nil {
			t.Fatal(err)
		}

		sample := func(x float64) oppositionSample {
			s, err := scan.sample(start.Add(unit.Days(x)))
			if err != nil {
				t.Fatal(err)
			}

			return s
		}

		worst := 0.0

		for x := c.d; x < c.span; x += c.step {
			bound := sample(x).curv / perturbationAllowance
			curv := math.Abs(sample(x+c.d).rate-sample(x-c.d).rate) / (2 * c.d)
			worst = math.Max(worst, curv/bound)
		}

		if worst > 1.15 {
			t.Errorf("%s: the longitude difference's rate changes %.3f times as fast as its two-body bound", c.target.Name(), worst)
		}

		if c.over && worst < 1.05 && !testing.Short() {
			t.Errorf("%s: %.3f of its bound, where the Earth's wobble about the barycentre passes it", c.target.Name(), worst)
		}
	}
}
