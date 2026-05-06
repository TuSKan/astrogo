package coord_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// Helper to provide a target at a specific geometric altitude
func getTargetAtAltitude(site *coord.Geodetic, obsTime time.Time, minAlt, maxAlt float64) vector.Vec3 {
	reducer := coord.NewReducer(site, obsTime, atmosphere.Atmosphere{Pressure: 0})
	for i := 0; i < 360; i += 5 {
		for j := -90; j <= 90; j += 5 {
			ra := angle.Deg(float64(i)).Radians()
			dec := angle.Deg(float64(j)).Radians()
			v := vector.Vec3{
				X: math.Cos(dec) * math.Cos(ra),
				Y: math.Cos(dec) * math.Sin(ra),
				Z: math.Sin(dec),
			}.Unit()

			alt := reducer.Reduce(v).Geometric.Alt().Degrees()
			if alt >= minAlt && alt <= maxAlt {
				return v
			}
		}
	}
	// Fallback safe vector
	return vector.Vec3{X: 1, Y: 0, Z: 0}.Unit()
}

// Helper to check numerical safety
func assertFinite(t *testing.T, val float64, name string) {
	if math.IsNaN(val) || math.IsInf(val, 0) {
		t.Errorf("Numerical safety failed: %s is NaN or Inf (%v)", name, val)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// GROUP 1 — Topocentric reduction invariants
// ─────────────────────────────────────────────────────────────────────────────

func TestReducer_Group1_GeometricConsistency(t *testing.T) {
	site, _ := coord.NewGeodetic(angle.Deg(-70), angle.Deg(40), 100)
	obsTime := time.NowUTC()
	atmZero := atmosphere.Atmosphere{Pressure: 0}

	vec := getTargetAtAltitude(site, obsTime, 40, 80)

	// Using the formal Reducer pipeline
	pipeline := coord.NewReducer(site, obsTime, atmZero)
	res := pipeline.Reduce(vec)

	// In the absence of atmosphere, Geometric and Observed must be perfectly equal numerically.
	if !res.Geometric.Equal(res.Observed) {
		t.Errorf("Geometric and Observed altitudes differ without atmosphere: Geom=%v, Obs=%v", res.Geometric.Alt(), res.Observed.Alt())
	}
}

func TestReducer_Group1_SiteDependence(t *testing.T) {
	site1, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(0), 0)
	site2, _ := coord.NewGeodetic(angle.Deg(10), angle.Deg(0), 0) // Moved East
	site3, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(10), 0) // Moved North

	obsTime := time.NowUTC()
	vec := getTargetAtAltitude(site1, obsTime, 40, 80)

	r1 := coord.NewReducer(site1, obsTime, atmosphere.Atmosphere{Pressure: 0}).Reduce(vec)
	r2 := coord.NewReducer(site2, obsTime, atmosphere.Atmosphere{Pressure: 0}).Reduce(vec)
	r3 := coord.NewReducer(site3, obsTime, atmosphere.Atmosphere{Pressure: 0}).Reduce(vec)

	// Expected: Same target should yield different AltAz coordinates for different locations
	if r1.Geometric.Equal(r2.Geometric) || r1.Geometric.Equal(r3.Geometric) || r2.Geometric.Equal(r3.Geometric) {
		t.Errorf("Site dependence failed: different sites returned identical geometric topocentric coordinates.")
	}
}

func TestReducer_Group1_TimeDependence(t *testing.T) {
	site, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(45), 0)
	obsTime1 := time.NowUTC()
	obsTime2 := obsTime1.Add(time.Hour)

	vec := getTargetAtAltitude(site, obsTime1, 40, 80)

	r1 := coord.NewReducer(site, obsTime1, atmosphere.Atmosphere{Pressure: 0}).Reduce(vec)
	r2 := coord.NewReducer(site, obsTime2, atmosphere.Atmosphere{Pressure: 0}).Reduce(vec)

	if r1.Geometric.Equal(r2.Geometric) {
		t.Errorf("Time dependence failed: different times returned identical geometric coordinates.")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// GROUP 2 — Atmospheric refraction invariants
// ─────────────────────────────────────────────────────────────────────────────

func TestReducer_Group2_ZeroPressure(t *testing.T) {
	site, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(45), 0)
	obsTime := time.NowUTC()

	atmStandard := atmosphere.StandardAtmosphere
	atmZero := atmosphere.StandardAtmosphere
	atmZero.Pressure = 0

	vec := getTargetAtAltitude(site, obsTime, 2, 10)

	rStandard := coord.NewReducer(site, obsTime, atmStandard).Reduce(vec)
	rZero := coord.NewReducer(site, obsTime, atmZero).Reduce(vec)

	// Zero pressure must perfectly mirror the Geometric pipeline (no observed offset)
	if rZero.Geometric.Alt().Degrees() != rZero.Observed.Alt().Degrees() {
		t.Errorf("Zero pressure did not perfectly disable refraction offset")
	}

	// But Standard pressure MUST cause an observed offset
	if rStandard.Geometric.Alt().Degrees() == rStandard.Observed.Alt().Degrees() {
		t.Errorf("Standard pressure failed to apply refraction offset")
	}
}

func TestReducer_Group2_RefractionRaisesAltitude(t *testing.T) {
	site, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(45), 0)
	obsTime := time.NowUTC()
	atm := atmosphere.StandardAtmosphere

	vec := getTargetAtAltitude(site, obsTime, 2, 10)
	res := coord.NewReducer(site, obsTime, atm).Reduce(vec)

	// Atmospheric refraction physically must increase the apparent altitude above horizon
	if res.Observed.Alt().Degrees() <= res.Geometric.Alt().Degrees() {
		t.Errorf("Refraction must raise altitude! Geometric=%v, Observed=%v", res.Geometric.Alt().Degrees(), res.Observed.Alt().Degrees())
	}
}

func TestReducer_Group2_RefractionWeakensAtZenith(t *testing.T) {
	site, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(45), 0)
	obsTime := time.NowUTC()
	atm := atmosphere.StandardAtmosphere

	reducer := coord.NewReducer(site, obsTime, atm)

	// We compare the difference in Alt for a low target vs a high target
	vecLow := getTargetAtAltitude(site, obsTime, 2, 10)
	resLow := reducer.Reduce(vecLow)
	shiftLow := resLow.Observed.Alt().Degrees() - resLow.Geometric.Alt().Degrees()

	vecHigh := getTargetAtAltitude(site, obsTime, 70, 90)
	resHigh := reducer.Reduce(vecHigh)
	shiftHigh := resHigh.Observed.Alt().Degrees() - resHigh.Geometric.Alt().Degrees()

	if math.Abs(shiftHigh) >= math.Abs(shiftLow) {
		t.Errorf("Refraction should be weaker at high altitudes! ShiftHigh=%v, ShiftLow=%v", shiftHigh, shiftLow)
	}
}

func TestReducer_Group2_LowAltitudeGuard(t *testing.T) {
	atm := atmosphere.StandardAtmosphere

	// Geometric altitude heavily below the horizon (-6 degrees)
	shiftDeep := atm.Model.RefractFromTrue(angle.Deg(-6.0), atm)
	if shiftDeep != 0 {
		t.Errorf("Expected Refraction model to guard deeply depressed altitudes correctly (return 0), got %v", shiftDeep.Degrees())
	}

	// Geometric altitude right on the horizon
	shiftHz := atm.Model.RefractFromTrue(angle.Deg(0), atm)
	assertFinite(t, float64(shiftHz), "Horizon Refraction")

	// Rigorous Saemundsson expectation near 0 is roughly ~34 arcminutes (+0.5667 ish)
	if shiftHz.Degrees() < 0.4 || shiftHz.Degrees() > 0.7 {
		t.Errorf("Expected realistic horizon refraction ~0.56 degrees, got %v deg", shiftHz.Degrees())
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// GROUP 3 — Atmospheric dispersion invariants
// ─────────────────────────────────────────────────────────────────────────────

func TestReducer_Group3_Dispersion(t *testing.T) {
	site, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(45), 0)
	obsTime := time.NowUTC()
	atm := atmosphere.StandardAtmosphere

	reducer := coord.NewReducer(site, obsTime, atm)
	vecLow := getTargetAtAltitude(site, obsTime, 2, 10)
	vecHigh := getTargetAtAltitude(site, obsTime, 70, 90)

	wls := []float64{0.35, 0.55, 2.0} // UV, Visual, IR

	// Group 3.1 & 3.2: Blue light refracts more
	resLow := reducer.Disperse(vecLow, wls)
	altUVLow := resLow.Dispersion[0.35].Alt().Degrees()
	altVisLow := resLow.Dispersion[0.55].Alt().Degrees()
	altIRLow := resLow.Dispersion[2.0].Alt().Degrees()

	if !(altUVLow > altVisLow && altVisLow > altIRLow) {
		t.Errorf("Dispersion failed sorting by wavelength index! UV=%v, VIS=%v, IR=%v", altUVLow, altVisLow, altIRLow)
	}

	// Group 3.3: Dispersion shrinks at high altitude
	resHigh := reducer.Disperse(vecHigh, wls)
	diffLow := altUVLow - altIRLow
	diffHigh := resHigh.Dispersion[0.35].Alt().Degrees() - resHigh.Dispersion[2.0].Alt().Degrees()

	if math.Abs(diffHigh) >= math.Abs(diffLow) {
		t.Errorf("Expected dispersion spreading to shrink at high altitudes. High Diff=%v, Low Diff=%v", diffHigh, diffLow)
	}

	// Group 3.4: No atm means no dispersion
	reducerZero := coord.NewReducer(site, obsTime, atmosphere.Atmosphere{Pressure: 0})
	resZero := reducerZero.Disperse(vecLow, wls)

	if resZero.Dispersion[0.35].Alt() != resZero.Dispersion[2.0].Alt() {
		t.Errorf("No atmosphere must completely disable dispersion!")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// GROUP 4 — API / Semantic correctness
// ─────────────────────────────────────────────────────────────────────────────

func TestReducer_Group4_Semantics(t *testing.T) {
	site, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(45), 0)
	atm := atmosphere.StandardAtmosphere
	obsTime := time.NowUTC()

	vec := getTargetAtAltitude(site, obsTime, 2, 10)
	pipeline := coord.NewReducer(site, obsTime, atm)
	res := pipeline.Reduce(vec)

	// With value types, we verify the Reduction fields are populated (non-zero-value).
	if res.Geometric.Alt().Radians() == 0 && res.Geometric.Az().Radians() == 0 &&
		res.Observed.Alt().Radians() == 0 && res.Observed.Az().Radians() == 0 {
		t.Fatalf("API violated: returned zero-value state points")
	}

	// 4.1 distinction
	if res.Geometric.Equal(res.Observed) {
		t.Errorf("API failed: Geometric and Observed states inappropriately aliased")
	}

	// 4.2 Apparent vector should roughly mirror typical vector norms
	astrometric := coord.NewAstrometric(angle.Zero(), angle.Zero()) // Placeholder Astrometric
	ctxTest := coord.NewContext(obsTime, site, atm)
	app := ctxTest.AstrometricToApparent(astrometric)
	if app.RA().Radians() == 0 && app.Dec().Radians() == 0 {
		t.Error("AstrometricToApparent returned uninitialized zero coordinates")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// GROUP 5 — Regression / Fixture tests & Numerical Safety
// ─────────────────────────────────────────────────────────────────────────────

func TestReducer_Group5_FixturesAndSafety(t *testing.T) {
	// Table driven fixture simulation
	// Evaluating typical constraints without infinite / NaN leaks

	type fixture struct {
		name       string
		lat, lon   float64
		vec        vector.Vec3
		expectMinH float64
		expectMaxH float64
	}

	tests := []fixture{
		{"HighAltitude", 45, 0, vector.Vec3{X: 1, Y: 0, Z: 1}.Unit(), 20, 90},
		{"MidAltitude", -30, -70, vector.Vec3{X: 0, Y: 1, Z: 0.5}.Unit(), 0, 45},
		{"LowAltitude", 0, 0, vector.Vec3{X: 1, Y: 0, Z: 0.05}.Unit(), -5, 10},
		{"Nadir", 45, -80, vector.Vec3{X: 0, Y: 0, Z: -1}.Unit(), -90, -10}, // Straight down
	}

	obsTime := time.Date(2026, 4, 6, 12, 0, 0, 0, time.LocationUTC)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			site, _ := coord.NewGeodetic(angle.Deg(tt.lon), angle.Deg(tt.lat), 0)

			// Test standard refraction
			reducer := coord.NewReducer(site, obsTime, atmosphere.StandardAtmosphere)
			res := reducer.Reduce(tt.vec)

			// Numerical safety checks
			assertFinite(t, res.Observed.Alt().Degrees(), tt.name+"_ObsAlt")
			assertFinite(t, res.Observed.Az().Degrees(), tt.name+"_ObsAz")
			assertFinite(t, res.Geometric.Alt().Degrees(), tt.name+"_GeoAlt")

			// Bound validation
			if res.Observed.Az().Degrees() < 0 || res.Observed.Az().Degrees() > 360 {
				t.Errorf("Azimuth out of standard wrapped bounds [0, 360]: %v", res.Observed.Az().Degrees())
			}

			// Extreme discontinuity check (e.g., dispersion blowing up)
			wls := []float64{0.4, 0.7}
			disp := reducer.Disperse(tt.vec, wls)
			diff := math.Abs(disp.Dispersion[0.4].Alt().Degrees() - disp.Dispersion[0.7].Alt().Degrees())

			assertFinite(t, diff, tt.name+"_DispersionDiff")
			if diff > 1.0 {
				t.Errorf("Unrealistic massive dispersion recorded: %v degrees", diff)
			}
		})
	}
}
