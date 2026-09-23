package dim_test

import (
	"testing"

	"github.com/TuSKan/astrogo/unit/dim"
)

// TestTheAlgebraComposesTheDeclaredDimensions checks that the derived values in
// this package are what their definitions say, by deriving them again from the
// base ones.
//
// It is worth doing here rather than only from `unit`, which used to hold these
// tests: a table of seven int8 exponents is easy to mistype and impossible to
// notice by reading, and every conversion built on a wrong one would be wrong
// together and self-consistent.
func TestTheAlgebraComposesTheDeclaredDimensions(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		derived dim.Dimension
		want    dim.Dimension
	}{
		{"area is length squared", dim.Length.PowInt(2), dim.Area},
		{"volume is length cubed", dim.Length.PowInt(3), dim.Volume},
		{"velocity is length over time", dim.Length.Div(dim.Time), dim.Velocity},
		{"acceleration is velocity over time", dim.Velocity.Div(dim.Time), dim.Accel},
		{"force is mass times acceleration", dim.Mass.Mul(dim.Accel), dim.Force},
		{"pressure is force over area", dim.Force.Div(dim.Area), dim.Pressure},
		{"energy is force times length", dim.Force.Mul(dim.Length), dim.Energy},
		{"power is energy over time", dim.Energy.Div(dim.Time), dim.Power},

		// W/(m²·Hz), which reduces to kg·s⁻² once the meters and the hertz
		// cancel — the one derived value whose name does not read as its
		// definition, and so the one most worth deriving here.
		{
			name:    "spectral flux is power over area over frequency",
			derived: dim.Power.Div(dim.Area).Div(dim.Time.PowInt(-1)),
			want:    dim.SpectralFlux,
		},
	} {
		if !tc.derived.Equals(tc.want) {
			t.Errorf("%s: derived %+v, declared %+v", tc.name, tc.derived, tc.want)
		}
	}
}

// TestDimensionlessIsTheIdentity covers the value that is easy to assume and
// never check.
//
// It has to be the identity of the algebra in both directions, because that is
// what makes a ratio of like dimensions come out dimensionless — an aspect
// ratio, a refractive index, the fine-structure constant.
func TestDimensionlessIsTheIdentity(t *testing.T) {
	t.Parallel()

	for _, d := range []dim.Dimension{
		dim.Length, dim.Mass, dim.Time, dim.Velocity, dim.Energy, dim.SpectralFlux,
	} {
		if got := d.Mul(dim.Dimensionless); !got.Equals(d) {
			t.Errorf("%+v multiplied by Dimensionless gave %+v", d, got)
		}

		if got := d.Div(dim.Dimensionless); !got.Equals(d) {
			t.Errorf("%+v divided by Dimensionless gave %+v", d, got)
		}

		// A quantity over itself is dimensionless, which is the property the
		// identity exists for.
		if got := d.Div(d); !got.Equals(dim.Dimensionless) {
			t.Errorf("%+v divided by itself gave %+v, want Dimensionless", d, got)
		}

		// And the zeroth power, which is the same statement through PowInt.
		if got := d.PowInt(0); !got.Equals(dim.Dimensionless) {
			t.Errorf("%+v to the power 0 gave %+v, want Dimensionless", d, got)
		}
	}
}

// TestMulAndDivAreInverses is the round trip, over every base dimension so no
// exponent field is left untested.
//
// Seven fields are added and subtracted independently, and a transposition
// between two of them — Theta where N should be — survives any test that only
// checks a dimension it happens not to involve.
func TestMulAndDivAreInverses(t *testing.T) {
	t.Parallel()

	base := []dim.Dimension{
		dim.Length, dim.Mass, dim.Time, dim.Current,
		dim.Temperature, dim.Amount, dim.Luminosity,
	}

	for _, a := range base {
		for _, b := range base {
			if got := a.Mul(b).Div(b); !got.Equals(a) {
				t.Errorf("(%+v * %+v) / %+v = %+v, want %+v", a, b, b, got, a)
			}

			if got := a.Div(b).Mul(b); !got.Equals(a) {
				t.Errorf("(%+v / %+v) * %+v = %+v, want %+v", a, b, b, got, a)
			}
		}
	}
}

// TestDistinctDimensionsStayDistinct is the negative that the round trips
// cannot give: an algebra where everything collapsed to the same value would
// satisfy every identity above.
func TestDistinctDimensionsStayDistinct(t *testing.T) {
	t.Parallel()

	named := map[string]dim.Dimension{
		"Dimensionless": dim.Dimensionless,
		"Length":        dim.Length,
		"Mass":          dim.Mass,
		"Time":          dim.Time,
		"Current":       dim.Current,
		"Temperature":   dim.Temperature,
		"Amount":        dim.Amount,
		"Luminosity":    dim.Luminosity,
		"Area":          dim.Area,
		"Volume":        dim.Volume,
		"Velocity":      dim.Velocity,
		"Accel":         dim.Accel,
		"Force":         dim.Force,
		"Pressure":      dim.Pressure,
		"Energy":        dim.Energy,
		"Power":         dim.Power,
		"SpectralFlux":  dim.SpectralFlux,
	}

	for aName, a := range named {
		for bName, b := range named {
			if aName == bName {
				continue
			}

			if a.Equals(b) {
				t.Errorf("%s and %s are the same dimension %+v", aName, bName, a)
			}
		}
	}
}
