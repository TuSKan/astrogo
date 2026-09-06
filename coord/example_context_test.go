package coord_test

import (
	"fmt"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
)

// siriusICRS is Sirius at its ICRS J2000 position, used by both examples so the
// second one is refracting a target the first has already shown to be up.
var siriusICRS = coord.NewAstrometric(angle.Deg(101.287155), angle.Deg(-16.716116))

// quintaCalixto returns the site both examples observe from: 22.53° S,
// 46.47° W, 835 m.
func quintaCalixto() *coord.Geodetic {
	site, err := coord.NewGeodetic(angle.Deg(-46.473002), angle.Deg(-22.528478), 835.05)
	if err != nil {
		panic(err)
	}

	return site
}

// The task this package exists for: take a catalogue position and say where in
// the sky it actually is, from a place, at an instant.
//
// The work is in [Context]. Building one runs SOFA's Apco13, which is the
// expensive part (~91 µs); every transform through it afterwards is ~325 ns. So
// a hot path builds one Context per epoch and reuses it across targets, rather
// than one per transform.
func Example() {
	when := time.Date(2026, time.March, 20, 3, 0, 0, 0, time.LocationUTC)

	// AtAltitude gives a standard atmosphere for the site's elevation. Its
	// Model field is deliberately left nil, which means "use the default"
	// rather than "no refraction" — see atmosphere.Refraction.EffectiveModel.
	ctx := coord.NewContext(when, quintaCalixto(), atmosphere.AtAltitude(835.05))

	altaz := ctx.AstrometricToObserved(siriusICRS)

	fmt.Printf("altitude %.2f°\n", altaz.Alt().Degrees())
	fmt.Printf("azimuth  %.2f°\n", altaz.Az().Degrees())

	// Output:
	// altitude 20.23°
	// azimuth  259.64°
}

// Refraction is the difference between where a body is and where it looks like
// it is. Building two Contexts that differ only in their atmosphere is the
// cheapest way to see how much of an altitude is the air.
//
// A zero [atmosphere.Refraction] is a vacuum: zero pressure means no
// refraction, which is the one case EffectiveModel reads as deliberate rather
// than as "no opinion".
func ExampleContext_AstrometricToObserved() {
	when := time.Date(2026, time.March, 20, 3, 0, 0, 0, time.LocationUTC)
	site := quintaCalixto()

	withAir := coord.NewContext(when, site, atmosphere.AtAltitude(835.05))
	vacuum := coord.NewContext(when, site, atmosphere.Refraction{})

	// Sirius, which the example above puts at 20° — well above the horizon,
	// where refraction is a real correction rather than an extrapolation.
	a := withAir.AstrometricToObserved(siriusICRS).Alt()
	v := vacuum.AstrometricToObserved(siriusICRS).Alt()

	fmt.Printf("true      %.4f°\n", v.Degrees())
	fmt.Printf("observed  %.4f°\n", a.Degrees())
	// ~2.4 arcmin is what standard refraction gives at 20°, once scaled for the
	// ~918 hPa of 835 m rather than sea level's 1013.
	fmt.Printf("the air lifts it by %.2f arcmin\n", (a - v).Arcminutes())

	// Output:
	// true      20.1896°
	// observed  20.2290°
	// the air lifts it by 2.37 arcmin
}
