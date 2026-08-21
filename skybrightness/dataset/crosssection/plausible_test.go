package crosssection

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
)

// chappuisFile writes a two-column file shaped like the atlas', with the
// Chappuis band peaking at the given cross section so a substitution can be
// simulated by scaling it.
func chappuisFile(peak float64) string {
	var b strings.Builder

	for nm := 400; nm <= 700; nm += 10 {
		// A crude bump centred on 603 nm — the shape does not matter here,
		// only the magnitude at the top of it.
		x := (float64(nm) - 603) / 80
		sigma := peak * math.Exp(-x*x)

		fmt.Fprintf(&b, "%d %g\n", nm, sigma)
	}

	return b.String()
}

// A file that parses is not thereby an ozone cross section.
//
// Parse and CrossSection.Validate establish structure only: two columns,
// increasing wavelengths, non-negative finite values. Every substitution below
// satisfies all of that and none of them is ozone.
func TestOzonePlausibility(t *testing.T) {
	t.Parallel()

	// The real thing: Serdyuchenko et al. peak near 4.8e-21 cm^2 per molecule.
	// This is the value that yields an optical depth near 0.04 at 300 DU, which
	// is what the network test checks end to end.
	const realPeak = 4.8e-21

	if err := parseAndCheck(chappuisFile(realPeak)); err != nil {
		t.Errorf("the published cross section was rejected: %v", err)
	}

	// A revision of the measurement moving the peak by a factor of a few must
	// still be accepted — the guard is against unit errors, not against the
	// laboratory disagreeing with itself.
	for _, factor := range []float64{0.2, 0.5, 2, 5, 10} {
		if err := parseAndCheck(chappuisFile(realPeak * factor)); err != nil {
			t.Errorf("a peak %gx the published value was rejected: %v", factor, err)
		}
	}

	// The substitutions that must not pass.
	for _, c := range []struct {
		name string
		file string
	}{
		{
			// cm^2 read as m^2, or the reverse: a factor of 1e4 either way.
			"cross section in square metres", chappuisFile(realPeak * 1e-4),
		},
		{
			"cross section scaled up by a unit error", chappuisFile(realPeak * 1e4),
		},
		{
			// The uncertainty column read as the value: same shape, orders
			// of magnitude smaller, entirely plausible on its own.
			"an uncertainty column", chappuisFile(realPeak * 1e-3 * 1e-3),
		},
		{
			// Wavelength and cross section transposed, which parses as a
			// perfectly monotonic file of very large numbers.
			"columns transposed", "1e-21 400\n2e-21 500\n3e-21 600\n4e-21 700\n",
		},
		{
			// A file that never reaches the visible: the Hartley band alone.
			// Structurally fine, and useless for the sky this models.
			"ultraviolet only", "220 1.0e-17\n230 1.1e-17\n240 1.05e-17\n250 1.1e-17\n",
		},
	} {
		if err := parseAndCheck(c.file); !errors.Is(err, ErrImplausible) {
			t.Errorf("%s: err = %v, want ErrImplausible", c.name, err)
		}
	}
}

// parseAndCheck runs the same two steps Ozone's validator does, so the test
// exercises the guard as it is actually wired rather than in isolation.
func parseAndCheck(file string) error {
	xs, err := Parse(strings.NewReader(file), "O3", Nanometre)
	if err != nil {
		return err
	}

	return plausibleOzone(xs)
}
