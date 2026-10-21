package plan

import (
	"testing"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// BenchmarkGeocentricVec is one apparent place on the analytical ephemeris,
// at a new instant each time, so nothing is reused from the call before:
// #791's measurement, the Sun, the Moon and Mars.
func BenchmarkGeocentricVec(b *testing.B) {
	prov := eph.Default()
	start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC)

	for _, body := range []*Planet{NewSun(prov), NewMoon(prov), NewMars(prov)} {
		b.Run(body.Name(), func(b *testing.B) {
			i := 0

			for b.Loop() {
				if _, err := body.GeocentricVec(start.Add(unit.Seconds(float64(i)))); err != nil {
					b.Fatal(err)
				}

				i++
			}
		})
	}
}

// BenchmarkMoonIllumination is one illumination of the Moon on the
// analytical ephemeris, at a new instant each time: #792's measurement.
func BenchmarkMoonIllumination(b *testing.B) {
	prov := eph.Default()
	start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC)
	i := 0

	for b.Loop() {
		if _, _, err := MoonIllumination(start.Add(unit.Seconds(float64(i))), prov); err != nil {
			b.Fatal(err)
		}

		i++
	}
}
