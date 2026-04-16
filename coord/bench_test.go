package coord_test

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// ── Context Creation ─────────────────────────────────────────────────────────
// This is the single most expensive call in the pipeline: it invokes
// gofaext.Apco13 which computes precession, nutation, Earth rotation,
// site geometry, and refraction parameters.

func BenchmarkNewContext(b *testing.B) {
	loc, _ := coord.NewGeodetic(angle.Deg(-70.4), angle.Deg(-24.6), 2635)
	atm := atmosphere.AtAltitude(2635)
	t := time.FromJD(2460000.5, time.UTC)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = coord.NewContext(t, loc, atm)
	}
}

func BenchmarkNewContext_SeaLevel(b *testing.B) {
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(45), 0)
	atm := atmosphere.StandardAtmosphere
	t := time.FromJD(2460000.5, time.UTC)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = coord.NewContext(t, loc, atm)
	}
}

// ── ICRS → AltAz Pipeline ───────────────────────────────────────────────────
// Measures the full astrometric pipeline: Context creation + coordinate
// transformation. This is what every IsVisible / constraint check calls.

func BenchmarkICRSToAltAz(b *testing.B) {
	loc, _ := coord.NewGeodetic(angle.Deg(-70.4), angle.Deg(-24.6), 2635)
	atm := atmosphere.AtAltitude(2635)
	t := time.FromJD(2460000.5, time.UTC)
	pos := coord.NewICRS(angle.Hour(5.5), angle.Deg(-5.4)) // Orion

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := coord.NewContext(t, loc, atm)
		_, _ = ctx.ICRSToAltAz(pos)
	}
}

// BenchmarkICRSToAltAz_CachedContext measures ONLY the coordinate
// transform cost when the Context is pre-built (amortized).
// This is the "cached context" scenario the user asked about.
func BenchmarkICRSToAltAz_CachedContext(b *testing.B) {
	loc, _ := coord.NewGeodetic(angle.Deg(-70.4), angle.Deg(-24.6), 2635)
	atm := atmosphere.AtAltitude(2635)
	t := time.FromJD(2460000.5, time.UTC)
	ctx := coord.NewContext(t, loc, atm)
	pos := coord.NewICRS(angle.Hour(5.5), angle.Deg(-5.4))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ctx.ICRSToAltAz(pos)
	}
}

// ── Batched vs Scalar ────────────────────────────────────────────────────────
// Measures the overhead of creating a new Context at each time step vs
// reusing one (which is technically incorrect but shows the cost ratio).

func BenchmarkICRSToAltAz_100Stars_NewContext(b *testing.B) {
	loc, _ := coord.NewGeodetic(angle.Deg(-70.4), angle.Deg(-24.6), 2635)
	atm := atmosphere.AtAltitude(2635)
	t := time.FromJD(2460000.5, time.UTC)

	stars := make([]*coord.ICRS, 100)
	for i := range stars {
		ra := angle.Deg(float64(i) * 3.6) // spread across sky
		dec := angle.Deg(float64(i)*1.8 - 90)
		stars[i] = coord.NewICRS(ra, dec)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// One context per star (scalar pattern — what the current code does)
		for _, s := range stars {
			ctx := coord.NewContext(t, loc, atm)
			_, _ = ctx.ICRSToAltAz(s)
		}
	}
}

func BenchmarkICRSToAltAz_100Stars_CachedContext(b *testing.B) {
	loc, _ := coord.NewGeodetic(angle.Deg(-70.4), angle.Deg(-24.6), 2635)
	atm := atmosphere.AtAltitude(2635)
	t := time.FromJD(2460000.5, time.UTC)

	stars := make([]*coord.ICRS, 100)
	for i := range stars {
		ra := angle.Deg(float64(i) * 3.6)
		dec := angle.Deg(float64(i)*1.8 - 90)
		stars[i] = coord.NewICRS(ra, dec)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// One context for all stars (batched pattern)
		ctx := coord.NewContext(t, loc, atm)
		for _, s := range stars {
			_, _ = ctx.ICRSToAltAz(s)
		}
	}
}

// ── Reducer Pipeline ─────────────────────────────────────────────────────────
// Full topocentric reduction (ICRS → TIRS → topocentric → AltAz + refraction).

func BenchmarkReducer(b *testing.B) {
	loc, _ := coord.NewGeodetic(angle.Deg(-70.4), angle.Deg(-24.6), 2635)
	atm := atmosphere.AtAltitude(2635)
	t := time.FromJD(2460000.5, time.UTC)
	// Simulate a geocentric Moon-like vector (in AU)
	v := vector.Vec3{X: 0.00257, Y: 0.00010, Z: 0.00005}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := coord.NewReducer(loc, t, atm)
		_ = r.Reduce(v)
	}
}
