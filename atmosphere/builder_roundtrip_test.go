package atmosphere_test

import (
	"math"
	"testing"
	gotime "time"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/unit"
)

// flatHorizon is a HorizonProfile that reports the same altitude everywhere,
// used only to check that the one a caller set is the one that comes back.
type flatHorizon angle.Angle

func (h flatHorizon) AltitudeAt(_ angle.Angle) angle.Angle { return angle.Angle(h) }

// Everything the Builder sets is what the accessors return.
//
// # Why a round trip, and why every value is distinct
//
// Because a builder writing to the wrong field is invisible from outside. The
// atmosphere still builds, every accessor still answers, and each answer is a
// number of the right type in a plausible range — the only symptom is a sky
// computed from somebody else's aerosol. There are seventeen setters here and
// most of them take a float, so the failure is a plausible one.
//
// Every value below is therefore deliberately unlike every other. An albedo
// of 0.3 and an asymmetry of 0.3 would let the two be swapped with nothing to
// show for it; 0.31 and 0.62 would not. That is the whole design of this
// test, and it is why the numbers look arbitrary.
func TestBuilderRoundTripsEveryField(t *testing.T) {
	t.Parallel()

	issued := gotime.Date(2026, gotime.March, 20, 3, 30, 0, 0, gotime.UTC)
	horizon := flatHorizon(angle.Deg(7.5))

	cloud := atmosphere.CloudLayer{
		Fraction:     0.42,
		BaseAlt:      1150,
		TopAlt:       2350,
		OpticalDepth: 17.5,
		Phase:        atmosphere.CloudIce,
		EffRadius:    12.5,
		Albedo:       0.63,
		Asymmetry:    0.81,
		Morphology:   atmosphere.MorphologyCirriform,
	}

	source := atmosphere.SourceRef{Name: "round-trip fixture", Fidelity: atmosphere.FidelitySynthetic}
	ground := atmosphere.SurfaceOptical{Albedo: 0.31, SnowFraction: 0.17}

	air, err := atmosphere.NewBuilder().
		Surface(812.5, 271.25).
		Aerosol(0.137, 532, 1.42, 0.938, 0.671).
		AerosolScaleHeight(1637).
		DiffuseScattering(0.63).
		MultipleScattering(true).
		Ozone(287.5).
		PrecipitableWater(3.75).
		SurfaceAlbedo(ground).
		Horizon(horizon).
		AddCloud(cloud).
		Source(source).
		IssuedAt(issued).
		LeadTime(90 * gotime.Minute).
		Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	pressure, temperature := air.Surface()
	if float64(pressure) != 812.5 {
		t.Errorf("surface pressure is %g hPa, want 812.5", float64(pressure))
	}

	// Surface takes kelvin and stores celsius, so the round trip is the one
	// place a 273.15 could go missing in either direction.
	if math.Abs(float64(temperature)-271.25) > 1e-9 {
		t.Errorf("surface temperature is %g K, want 271.25", float64(temperature))
	}

	aer := air.Aerosol()
	for _, c := range []struct {
		name string
		got  float64
		want float64
	}{
		{"optical depth", float64(aer.OpticalDepth), 0.137},
		{"reference wavelength", float64(aer.ReferenceWavelength), 532},
		{"Angstrom exponent", float64(aer.AngstromExp), 1.42},
		{"single-scattering albedo", float64(aer.SingleScatteringAlbedo), 0.938},
		{"asymmetry", float64(aer.Asymmetry), 0.671},
		{"scale height", float64(aer.ScaleHeight), 1637},
		{"diffuse kappa", air.DiffuseKappa(), 0.63},
		{"ozone", float64(air.Ozone()), 287.5},
		{"precipitable water", float64(air.PrecipitableWater()), 3.75},
		{"ground albedo", float64(air.SurfaceOptical().Albedo), 0.31},
		{"snow fraction", air.SurfaceOptical().SnowFraction, 0.17},
	} {
		if c.got != c.want {
			t.Errorf("%s is %g, want %g", c.name, c.got, c.want)
		}
	}

	if !air.MultipleScattering() {
		t.Error("multiple scattering was set on and came back off")
	}

	if got := air.Horizon(); got == nil {
		t.Error("the horizon profile is nil after one was set")
	} else if h := got.AltitudeAt(angle.Deg(0)); math.Abs(h.Degrees()-7.5) > 1e-9 {
		t.Errorf("the horizon reports %g degrees, want 7.5", h.Degrees())
	}

	if got := air.Provenance().Source; got.Name != source.Name || got.Fidelity != source.Fidelity {
		t.Errorf("provenance source is %+v, want %+v", got, source)
	}

	clouds := air.Clouds()
	if len(clouds) != 1 {
		t.Fatalf("got %d cloud layers, want 1", len(clouds))
	}

	if clouds[0] != cloud {
		t.Errorf("the cloud layer came back as %+v, want %+v", clouds[0], cloud)
	}

	// Age is measured from the issue time, so an hour later is an hour old.
	if got := air.Age(issued.Add(gotime.Hour)); got != gotime.Hour {
		t.Errorf("age one hour after issue is %v, want 1h", got)
	}
}

// Clouds hands out a copy, so a caller cannot reach back into the atmosphere.
//
// The accessor documents this and nothing checked it. A shared backing array
// would let one caller's edit change what every other caller computes, which
// is the kind of defect that shows up as an unreproducible sky rather than as
// a failure.
func TestCloudsIsADefensiveCopy(t *testing.T) {
	t.Parallel()

	air, err := atmosphere.NewBuilder().
		AddCloud(atmosphere.CloudLayer{Fraction: 0.5, BaseAlt: 1000, TopAlt: 1500}).
		Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	got := air.Clouds()
	got[0].Fraction = 0.99

	if again := air.Clouds(); again[0].Fraction != 0.5 {
		t.Errorf("mutating the returned slice changed the atmosphere: fraction is now %g",
			float64(again[0].Fraction))
	}
}

// Clear removes every layer that was added.
func TestClearRemovesTheClouds(t *testing.T) {
	t.Parallel()

	air, err := atmosphere.NewBuilder().
		AddCloud(atmosphere.CloudLayer{Fraction: 1, BaseAlt: 800, TopAlt: 1200}).
		AddCloud(atmosphere.CloudLayer{Fraction: 0.3, BaseAlt: 4000, TopAlt: 4500}).
		Clear().
		Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if got := air.Clouds(); len(got) != 0 {
		t.Errorf("%d layers survived Clear", len(got))
	}
}

// An unset atmosphere reports its documented defaults rather than zeros.
//
// The distinction matters: a kappa of zero would mean the air brightens an
// extended source, and an age of zero on a state that was never stamped is
// the honest answer where "now minus the zero time" would be about 2000
// years.
func TestUnsetAtmosphereUsesItsDefaults(t *testing.T) {
	t.Parallel()

	air, err := atmosphere.NewBuilder().Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if got := air.DiffuseKappa(); got != atmosphere.DefaultDiffuseKappa {
		t.Errorf("kappa with none set is %g, want the documented default %g",
			got, atmosphere.DefaultDiffuseKappa)
	}

	if got := air.Age(gotime.Now()); got != 0 {
		t.Errorf("age with no issue time is %v, want 0", got)
	}

	if air.MultipleScattering() {
		t.Error("multiple scattering defaults on; it must default off so a first-order " +
			"model reproduces the paper it came from")
	}

	if got := air.Horizon(); got != nil {
		t.Errorf("horizon with none set is %v, want nil", got)
	}
}

// A kappa outside (0,1] is ignored rather than stored.
//
// Above one would dim an extended source more than a point source in the same
// air, which is backwards; zero or below would brighten it. Both are rejected
// at the setter, so the accessor falls back to the default.
func TestDiffuseScatteringRejectsImpossibleKappa(t *testing.T) {
	t.Parallel()

	for _, kappa := range []float64{0, -0.5, 1.5, 100} {
		air, err := atmosphere.NewBuilder().DiffuseScattering(kappa).Build()
		if err != nil {
			t.Fatalf("Build: %v", err)
		}

		if got := air.DiffuseKappa(); got != atmosphere.DefaultDiffuseKappa {
			t.Errorf("kappa %g was accepted and reads back as %g; values outside (0,1] "+
				"must fall back to the default", kappa, got)
		}
	}

	// And one inside the range is kept.
	air, err := atmosphere.NewBuilder().DiffuseScattering(1).Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if got := air.DiffuseKappa(); got != 1 {
		t.Errorf("kappa 1 reads back as %g; it is the top of the valid range, not past it", got)
	}
}

// SurfaceAtAltitude sets thinner, colder air the higher it is asked for.
//
// The absolute values belong to the ICAO profile and are checked where that
// profile is; what this pins is that the builder is wired to it at all, and
// in the right direction. NewBuilder starts every atmosphere at sea level, so
// a caller who forgets this call overstates molecular scattering at a
// mountain site by about a quarter.
func TestSurfaceAtAltitudeThinsTheAir(t *testing.T) {
	t.Parallel()

	lastP, lastT := math.Inf(1), math.Inf(1)

	for _, h := range []float64{0, 1000, 2635, 4200} {
		air, err := atmosphere.NewBuilder().SurfaceAtAltitude(h).Build()
		if err != nil {
			t.Fatalf("Build at %g m: %v", h, err)
		}

		p, temp := air.Surface()

		if float64(p) >= lastP {
			t.Errorf("at %g m the pressure is %g hPa, no lower than the %g below it",
				h, float64(p), lastP)
		}

		if float64(temp) >= lastT {
			t.Errorf("at %g m the temperature is %g K, no lower than the %g below it",
				h, float64(temp), lastT)
		}

		lastP, lastT = float64(p), float64(temp)
	}
}

// TauAt applies the Angstrom power law, and falls back to the reference depth
// when it cannot.
func TestAerosolTauAtFollowsTheAngstromLaw(t *testing.T) {
	t.Parallel()

	a := atmosphere.Aerosol{
		OpticalDepth:        0.2,
		ReferenceWavelength: 500,
		AngstromExp:         1.5,
	}

	// tau(2*lambda0) = tau0 * 2^-alpha.
	got := float64(a.TauAt(1000))
	want := 0.2 * math.Pow(2, -1.5)

	if math.Abs(got-want) > 1e-15 {
		t.Errorf("tau at twice the reference is %g, want %g", got, want)
	}

	// At the reference wavelength it is the reference depth exactly.
	if got := float64(a.TauAt(500)); got != 0.2 {
		t.Errorf("tau at the reference wavelength is %g, want 0.2", got)
	}

	// A wavelength or reference that cannot enter a power law returns the
	// reference depth rather than a NaN that would poison everything after.
	for _, c := range []struct {
		name string
		a    atmosphere.Aerosol
		nm   unit.WavelengthNM
	}{
		{"zero wavelength", a, 0},
		{"negative wavelength", a, -500},
		{"zero reference", atmosphere.Aerosol{OpticalDepth: 0.2}, 550},
	} {
		if got := float64(c.a.TauAt(c.nm)); got != 0.2 {
			t.Errorf("%s gave %g, want the reference depth 0.2", c.name, got)
		}
	}
}

// The cloud enums name every value, and an unrecognised one says so.
func TestCloudEnumsName(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		got  string
		want string
	}{
		{atmosphere.CloudLiquid.String(), "Liquid"},
		{atmosphere.CloudIce.String(), "Ice"},
		{atmosphere.CloudMixed.String(), "Mixed"},
		{atmosphere.CloudPhase(9).String(), "CloudPhase(unknown)"},
		{atmosphere.MorphologyUnknown.String(), "Unknown"},
		{atmosphere.MorphologyStratiform.String(), "Stratiform"},
		{atmosphere.MorphologyCumuliform.String(), "Cumuliform"},
		{atmosphere.MorphologyCirriform.String(), "Cirriform"},
	} {
		if c.got != c.want {
			t.Errorf("got %q, want %q", c.got, c.want)
		}
	}
}
