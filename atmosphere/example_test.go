package atmosphere_test

import (
	"fmt"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
)

// The task this package exists for, and the one thing about it worth reading
// before use: what a zero value means.
//
// [Refraction] is a plain struct a caller can build literally, and every
// constructor leaves Model nil. Nil does not mean "no refraction" — it means
// "no opinion, use the default". Only a zero pressure means vacuum. Reaching
// into Model directly gets a nil dereference; [Refraction.EffectiveModel] is
// the accessor that is never nil.
func Example() {
	// Standard atmosphere for a site at 835 m.
	env := atmosphere.AtAltitude(835.05)

	fmt.Printf("pressure %.1f hPa, Model set: %v\n", env.Pressure, env.Model != nil)
	fmt.Printf("refracts with: %T\n", env.EffectiveModel())

	// A zero value is a vacuum, and says so.
	fmt.Printf("zero value refracts with: %T\n", atmosphere.Refraction{}.EffectiveModel())

	// Output:
	// pressure 916.9 hPa, Model set: false
	// refracts with: atmosphere.RefractionSOFA
	// zero value refracts with: atmosphere.RefractionNone
}

// Refraction grows steeply toward the horizon, which is why rise and set are
// the most air-sensitive quantities this library computes.
//
// The altitudes here stop at 10° deliberately. The default model is SOFA's
// two-term series, A·tan z + B·tan³z, which is accurate well above the horizon
// and degrades badly at it — at a true altitude of 0° it returns about 10.8
// arcmin against the ~34 arcmin actually observed. That is a known property of
// the series, not a defect, and it is the reason rise and set are defined
// against a fixed 34' convention (plan's Site.SunRiseSetThreshold) rather than
// against a refraction computation.
func ExampleRefraction_RefractFromTrue() {
	env := atmosphere.AtAltitude(0)

	for _, alt := range []float64{10, 20, 45, 90} {
		r := env.RefractFromTrue(angle.Deg(alt))
		fmt.Printf("%2.0f° -> lifted %5.2f arcmin\n", alt, r.Arcminutes())
	}

	// Output:
	// 10° -> lifted  5.16 arcmin
	// 20° -> lifted  2.59 arcmin
	// 45° -> lifted  0.95 arcmin
	// 90° -> lifted  0.00 arcmin
}

// Airmass is how much atmosphere a sightline crosses, normalised to 1 at the
// zenith. It is the quantity extinction is proportional to, so it is what turns
// an altitude into a magnitude penalty.
//
// Two values are worth recognising: 1 at the zenith, and ~2 at 30°, since
// sin(30°) is a half and a sightline there crosses twice the vertical column.
func ExampleAirmass() {
	for _, alt := range []float64{90, 30, 10} {
		x, err := atmosphere.Airmass(angle.Deg(alt))
		if err != nil {
			panic(err)
		}

		fmt.Printf("%2.0f° -> %.3f airmasses\n", alt, x)
	}

	// Output:
	// 90° -> 1.000 airmasses
	// 30° -> 1.993 airmasses
	// 10° -> 5.581 airmasses
}
