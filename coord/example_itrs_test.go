package coord_test

import (
	"fmt"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// An Earth-fixed position is a rotation away from a celestial one, and the
// Context already holds the rotation for its epoch. Here a station's own
// coordinates make the round trip: geodetic to Earth-fixed, out to the
// celestial frame, and back.
func ExampleContext_ICRSToITRS() {
	site := coord.MustGeodetic(angle.Deg(-70.40417), angle.Deg(-24.62722), 2635) // Paranal

	ctx := coord.NewContext(
		time.Date(2026, 4, 15, 22, 0, 0, 0, time.LocationUTC),
		site,
		atmosphere.Refraction{},
	)

	// Where the station is, in meters from the geocenter.
	ecef := site.ToECEF(coord.WGS84())

	// The same point seen from the celestial frame, and back again.
	icrs := ctx.ITRSToICRS(ecef)
	back := ctx.ICRSToITRS(icrs)

	fmt.Printf("distance from the geocenter: %.3f km\n", ecef.Norm()/1000)
	fmt.Printf("unchanged by the round trip: %.3f km\n", back.Norm()/1000)
	// Printed as a threshold rather than a value: the residual is float
	// rounding on a 6377 km vector, so its last digits are not the same on
	// every platform and an Example compares text exactly.
	fmt.Printf("round trip moved under a micrometer: %v\n", back.Sub(ecef).Norm() < 1e-6)

	// Output:
	// distance from the geocenter: 6377.084 km
	// unchanged by the round trip: 6377.084 km
	// round trip moved under a micrometer: true
}

// The rotation is not a constant: a direction fixed in the sky sweeps through
// Earth-fixed longitude as the planet turns beneath it.
func ExampleContext_ICRSToITRS_earthRotation() {
	ctx := coord.NewContext(
		time.Date(2026, 4, 15, 22, 0, 0, 0, time.LocationUTC),
		coord.MustGeodetic(angle.Deg(0), angle.Deg(0), 0),
		atmosphere.Refraction{},
	)

	star := coord.NewICRS(angle.Deg(101.2871), angle.Deg(-16.7161)).ToUnitVector() // Sirius

	for _, hours := range []float64{0, 6, 12} {
		at := ctx.AtTime(ctx.Time().Add(unit.Days(hours / 24)))

		lon, lat := at.ICRSToITRS(star).ToSpherical()

		fmt.Printf("+%2.0f h: earth-fixed lon %+8.3f deg, lat %+7.3f deg\n",
			hours, angle.Rad(lon).Wrap180().Degrees(), angle.Rad(lat).Degrees())
	}

	// The latitude barely moves — it is the Earth's spin, so only the
	// longitude runs, and it runs at the sidereal rate rather than the solar
	// one: 6 hours takes it 90.25 degrees, not 90.

	// Output:
	// + 0 h: earth-fixed lon  -72.491 deg, lat -16.743 deg
	// + 6 h: earth-fixed lon -162.738 deg, lat -16.743 deg
	// +12 h: earth-fixed lon +107.016 deg, lat -16.743 deg
}
