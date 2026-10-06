package atmosphere_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
)

// TestRefractionWorksWithTheNilModelEveryConstructorLeaves is the defect.
//
// Every constructor leaves Model nil — AtAltitude sets it so deliberately, and
// a Refraction built as a literal (which its own doc comment invites) has no
// model either. The package documented a pluggable RefractionModel, so anyone
// following that into env.Model.RefractFromTrue got a nil pointer dereference.
//
// Refraction now answers for itself.
func TestRefractionWorksWithTheNilModelEveryConstructorLeaves(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  atmosphere.Refraction
	}{
		{"AtAltitude at sea level", atmosphere.AtAltitude(0)},
		{"AtAltitude at 2400 m", atmosphere.AtAltitude(2400)},
		{"a bare literal, as the type invites", atmosphere.Refraction{
			Pressure: 1013.25, Temperature: 15, Humidity: 0.5, Wavelength: 0.55,
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.env.Model != nil {
				t.Fatalf("precondition: this constructor now sets a Model (%T); the "+
					"test no longer covers the nil case it was written for", tc.env.Model)
			}

			// Would have panicked. Both directions, since both were reachable.
			fromTrue := tc.env.RefractFromTrue(angle.Deg(45))
			fromApparent := tc.env.RefractFromApparent(angle.Deg(45))

			// At 45 degrees refraction is about one arcminute at sea level and
			// less higher up, where there is less air. A wide bracket, because
			// the point here is "a real number, not a panic".
			for _, got := range []angle.Angle{fromTrue, fromApparent} {
				if s := got.Arcseconds(); s < 20 || s > 90 {
					t.Errorf("refraction at 45 deg = %.2f arcsec, want a plausible "+
						"20-90 arcsec", s)
				}
			}
		})
	}
}

// TestEffectiveModelResolvesTheNilConvention pins the three-way resolution, and
// in particular that nil does not mean "no refraction".
//
// A zero pressure is the only thing that means a vacuum. Reading nil as "no
// atmosphere" is what made coord.Disperse report zero dispersion for an
// environment coord.Reduce had just refracted through.
func TestEffectiveModelResolvesTheNilConvention(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  atmosphere.Refraction
		want atmosphere.RefractionModel
	}{
		{
			"an explicit model wins",
			atmosphere.Refraction{Model: atmosphere.RefractionApproximate{}, Pressure: 1013},
			atmosphere.RefractionApproximate{},
		},
		{
			"an explicit model wins even at zero pressure",
			atmosphere.Refraction{Model: atmosphere.RefractionRigorous{}},
			atmosphere.RefractionRigorous{},
		},
		{
			"nil with a pressure means SOFA",
			atmosphere.Refraction{Pressure: 1013.25, Temperature: 15},
			atmosphere.RefractionSOFA{},
		},
		{
			"nil without a pressure means a vacuum",
			atmosphere.Refraction{},
			atmosphere.RefractionNone{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.env.EffectiveModel()
			if got == nil {
				t.Fatal("EffectiveModel returned nil, which it never may")
			}

			if gotType, wantType := typeName(got), typeName(tc.want); gotType != wantType {
				t.Errorf("EffectiveModel() = %s, want %s", gotType, wantType)
			}
		})
	}
}

func typeName(m atmosphere.RefractionModel) string {
	switch m.(type) {
	case atmosphere.RefractionNone:
		return "RefractionNone"
	case atmosphere.RefractionApproximate:
		return "RefractionApproximate"
	case atmosphere.RefractionRigorous:
		return "RefractionRigorous"
	case atmosphere.RefractionSOFA:
		return "RefractionSOFA"
	default:
		return "unknown"
	}
}

// TestRefractionSOFAAgreesWithTheClassicTangentRule checks the new model
// against something that is not SOFA, so this is a check on the physics rather
// than on the plumbing.
//
// The textbook rule of thumb is R ≈ 58.3″ · tan z for zenith distances inside
// about 75°, quoted at 1010 hPa and 10 °C. Scaling to this environment's
// 1013.25 hPa and 15 °C by the usual density ratio gives 57.4″ at the zenith
// distance where tan z = 1.
//
// The tolerance is 3%: the rule is itself a one-term approximation of the
// two-term series, so agreeing much more closely than that would be suspicious
// rather than reassuring.
func TestRefractionSOFAAgreesWithTheClassicTangentRule(t *testing.T) {
	env := atmosphere.Refraction{
		Pressure: 1013.25, Temperature: 15.0, Humidity: 0.5, Wavelength: 0.55,
	}

	// 58.3" at 1010 hPa / 10 C, scaled to this environment.
	const ruleAtUnitTangent = 58.3 * (1013.25 / 1010.0) * (283.15 / 288.15)

	for _, altDeg := range []float64{60, 45, 30, 20} {
		zenithDistance := 90.0 - altDeg
		want := ruleAtUnitTangent * math.Tan(zenithDistance*math.Pi/180.0)

		got := env.RefractFromTrue(angle.Deg(altDeg)).Arcseconds()

		if rel := math.Abs(got-want) / want; rel > 0.03 {
			t.Errorf("at %.0f deg altitude: %.2f arcsec, want about %.2f from the "+
				"58.3\"·tan z rule (off by %.1f%%)", altDeg, got, want, rel*100)
		}
	}

	// And it vanishes at the zenith, where the tangent rule gives zero too.
	if got := env.RefractFromTrue(angle.Deg(90)).Arcseconds(); math.Abs(got) > 0.01 {
		t.Errorf("refraction at the zenith = %.4f arcsec, want 0", got)
	}
}

// TestRefractionSOFADispersesByWavelength pins the property that makes this
// model worth having over the empirical formulas beside it.
//
// gofa's Refco integrates the refractive index of moist air at the wavelength
// given, so blue light refracts measurably more than red. That difference is
// atmospheric dispersion — the reason a low star shows a color-fringed
// image, and the quantity coord.Disperse exists to report.
func TestRefractionSOFADispersesByWavelength(t *testing.T) {
	base := atmosphere.Refraction{Pressure: 1013.25, Temperature: 15.0, Humidity: 0.5}

	blue, red := base, base
	blue.Wavelength, red.Wavelength = 0.40, 0.70

	const alt = 30.0

	b := blue.RefractFromTrue(angle.Deg(alt))
	r := red.RefractFromTrue(angle.Deg(alt))

	spread := (b - r).Arcseconds()

	if spread <= 0 {
		t.Fatalf("blue refracted %.3f arcsec and red %.3f — blue must refract more",
			b.Arcseconds(), r.Arcseconds())
	}

	// Differential refraction between 0.40 and 0.70 um at 60 degrees zenith
	// distance is a couple of arcseconds; it is a well-known observing problem
	// at this scale, which is why atmospheric dispersion correctors exist.
	if spread < 1.0 || spread > 5.0 {
		t.Errorf("dispersion 0.40-0.70 um at %.0f deg = %.3f arcsec, want roughly "+
			"1-5 arcsec", alt, spread)
	}

}

// TestRefractionRigorousDispersesLikeSOFA holds the empirical model's
// wavelength law to SOFA's.
//
// The two models differ in total refraction: the Bennett and Saemundsson fits
// sit 3 to 5 percent above SOFA's A·tan z + B·tan³ z at sea-level standard
// conditions, which is the empirical calibration and not this test's subject.
// What must agree is how refraction scales with wavelength, so this compares
// dispersion as a fraction of each model's own refraction. Measured, they
// agree to 0.1 percent from 80 down to 5 degrees.
//
// It used to scale by an unsourced 1 + 0.005·(0.55 − λ), and a test here
// pinned that as the smaller of the two: 0.155 arcsec of dispersion between
// 0.40 and 0.70 µm at 30 degrees, against SOFA's 2.467 (#527). A caller of
// coord.Reducer.Disperse with atmosphere.StandardRefraction, which carries
// this model, was told dispersion was 16 times smaller than it is.
//
// Below 5 degrees SOFA's tan z expansion is itself the weaker model, and
// the comparison stops meaning anything.
func TestRefractionRigorousDispersesLikeSOFA(t *testing.T) {
	base := atmosphere.Refraction{Pressure: 1013.25, Temperature: 15.0, Humidity: 0.5}

	for _, alt := range []float64{80, 60, 45, 30, 20, 15, 10, 5} {
		sofa := fractionalDispersion(base, angle.Deg(alt))

		rig := base
		rig.Model = atmosphere.RefractionRigorous{}

		empirical := fractionalDispersion(rig, angle.Deg(alt))

		if rel := empirical/sofa - 1; math.Abs(rel) > 0.01 {
			t.Errorf("at %.0f degrees: RefractionRigorous disperses %.5f of its refraction between "+
				"0.40 and 0.70 µm and SOFA %.5f, %.1f%% apart; want within 1%%",
				alt, empirical, sofa, 100*rel)
		}
	}
}

// fractionalDispersion is the refraction difference between 0.40 and 0.70 µm
// as a fraction of the refraction at 0.55 µm, under env's model.
func fractionalDispersion(env atmosphere.Refraction, alt angle.Angle) float64 {
	blue, red, mid := env, env, env
	blue.Wavelength, red.Wavelength, mid.Wavelength = 0.40, 0.70, 0.55

	return (blue.RefractFromTrue(alt) - red.RefractFromTrue(alt)).Radians() / mid.RefractFromTrue(alt).Radians()
}
