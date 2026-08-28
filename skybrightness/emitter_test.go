package skybrightness_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/skybrightness"
)

// Garstang's Q and q in Kocifaj (2007)'s own numerical runs.
const (
	garstangQ = 0.15
	garstangq = 0.15
)

// Garstang emission at the horizon is the largest lobe of the shape, not
// zero.
//
// # Why this is the test that had to exist
//
// Because the guard here was `sin <= 0`, and [ArtificialSkyglow] evaluates
// the emission function at exactly zero elevation by default — a ground
// source beyond a few kilometres sits at the observer's horizon, which is
// that component's own documented reasoning. The two combined produced
// exactly zero radiance for every source, at every distance, in every
// direction. Not an error and not a small number: an artificial-skyglow term
// that silently contributed nothing.
//
// Nothing caught it because every artificial test builds its city with
// [UpwardEmission], whose guard is `sin < 0`, and the one place
// [GarstangEmission] was exercised — [CloudySkyglow] — derives its elevation
// from geometry and so lands on exactly zero with probability zero.
//
// The physics says the same thing as the arithmetic. The reflected term
// vanishes at the horizon with cos z0 and the direct term does not, so what
// survives is 0.554*q*(pi/2)^4, and that near-horizontal light is what
// carries skyglow to distance. It is the reason the function has a z0^4 term
// at all.
func TestGarstangEmissionAtTheHorizonIsItsLargestLobe(t *testing.T) {
	t.Parallel()

	shape := skybrightness.GarstangEmission{
		ReflectedFraction: garstangQ,
		DirectFraction:    garstangq,
	}

	got := shape.Weight(angle.Deg(0))

	// B(z0 = pi/2) = 2Q(1-q)cos(pi/2) + 0.554*q*(pi/2)^4, over the zenith
	// value 2Q(1-q).
	quarterTurn := math.Pi / 2
	want := 0.554 * garstangq * math.Pow(quarterTurn, 4) / (2 * garstangQ * (1 - garstangq))

	if math.Abs(got-want) > 1e-12 {
		t.Errorf("the weight at the horizon is %.6f, want %.6f", got, want)
	}

	// And the claim that makes it matter: more light leaves near the
	// horizontal than straight up.
	if zenith := shape.Weight(angle.Deg(90)); got <= zenith {
		t.Errorf("the horizon weight is %.4f and the zenith weight %.4f; for these Q and q "+
			"the near-horizontal lobe is the larger, and it is the one that carries "+
			"skyglow to distance", got, zenith)
	}
}

// The shape is normalised at the zenith, which is what makes it a shape.
func TestGarstangEmissionIsNormalisedAtTheZenith(t *testing.T) {
	t.Parallel()

	shape := skybrightness.GarstangEmission{
		ReflectedFraction: garstangQ,
		DirectFraction:    garstangq,
	}

	if got := shape.Weight(angle.Deg(90)); math.Abs(got-1) > 1e-12 {
		t.Errorf("the weight at the zenith is %.15f, want exactly 1 — the value there is "+
			"the normaliser, so anything else means the two disagree", got)
	}
}

// Below the source's horizon nothing escapes upward.
//
// The boundary is the point: at zero it emits and below zero it does not, so
// this and TestGarstangEmissionAtTheHorizonIsItsLargestLobe fix the guard
// from both sides. A comparison that only checked "not negative" would have
// passed against the bug that motivated them.
func TestGarstangEmissionBelowTheHorizonIsZero(t *testing.T) {
	t.Parallel()

	shape := skybrightness.GarstangEmission{
		ReflectedFraction: garstangQ,
		DirectFraction:    garstangq,
	}

	for _, deg := range []float64{-0.001, -1, -45, -90} {
		if got := shape.Weight(angle.Deg(deg)); got != 0 {
			t.Errorf("the weight at %g degrees is %g, want exactly 0", deg, got)
		}
	}
}

// A Garstang-shaped city produces artificial skyglow.
//
// The unit test above pins the shape; this pins the consequence, which is the
// part that was actually broken. [ArtificialSkyglow] evaluates the emission
// function at zero elevation, so a shape returning zero there zeroes the
// whole component — and a component contributing nothing looks exactly like a
// dark site rather than like a defect.
//
// It runs the same falls-with-distance claim as the [UpwardEmission] tests
// rather than only checking for a positive number, because "nonzero" would be
// satisfied by any constant and this is a propagation model.
func TestArtificialSkyglowWorksWithGarstangEmission(t *testing.T) {
	t.Parallel()

	scene := artificialScene(t)
	prev := math.Inf(1)

	for _, km := range []float64{10, 20, 40, 80} {
		city := cityAt(t, 0, km, 1e-3)
		city.Emission = skybrightness.GarstangEmission{
			ReflectedFraction: garstangQ,
			DirectFraction:    garstangq,
		}

		component, err := skybrightness.NewArtificialSkyglow(
			[]skybrightness.GroundEmitter{city})
		if err != nil {
			t.Fatalf("NewArtificialSkyglow: %v", err)
		}

		got := radianceAt(t, component, scene, 45, 0)

		if got <= 0 {
			t.Fatalf("a Garstang-shaped city at %g km contributed %g; the component "+
				"evaluates emission at zero elevation, so a shape that returns zero "+
				"there silently removes artificial light from every sky", km, got)
		}

		if got >= prev {
			t.Errorf("a city at %g km contributes %.4g, more than the %.4g of the nearer "+
				"one", km, got, prev)
		}

		prev = got
	}
}
