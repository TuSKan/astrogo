package plan

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/angle"
)

// TestParseHorizonProfileReadsAHorizonTable: a table in the form Stellarium,
// APT and NINA write, with comments, blank lines, commas, tabs and a trailing
// column, read into a profile that passes through each point, interpolates
// linearly between them and across north, and steps where two points share
// an azimuth.
func TestParseHorizonProfileReadsAHorizonTable(t *testing.T) {
	t.Parallel()

	const table = `# horizon of a garden
; an APT-style comment

350 10
10,20
90	20	the house
90 40
180 5 trailing words
`

	p, err := ParseHorizonProfile(strings.NewReader(table))
	if err != nil {
		t.Fatalf("ParseHorizonProfile: %v", err)
	}

	for _, c := range []struct {
		az, want float64
	}{
		{350, 10},
		{10, 20},
		{0, 15},                  // across north, halfway from 350° to 10°
		{370, 20},                // an azimuth outside [0, 360) is wrapped
		{50, 20},                 // level between 10° and 90°
		{89.99, 20},              // up to the step
		{90.01, 40 - 35*0.01/90}, // away from it, toward 180°
		{135, 22.5},
		{265, 7.5}, // from 180° back to 350°
	} {
		if got := p(angle.Deg(c.az)).Degrees(); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("horizon at azimuth %g° = %.9f°, want %g°", c.az, got, c.want)
		}
	}
}

// TestParseHorizonProfileOnePointIsLevel: one point is a level horizon.
func TestParseHorizonProfileOnePointIsLevel(t *testing.T) {
	t.Parallel()

	p, err := ParseHorizonProfile(strings.NewReader("123 7.5\n"))
	if err != nil {
		t.Fatal(err)
	}

	for _, az := range []float64{0, 123, 300} {
		// angle.Angle holds radians, so 7.5° comes back to within an ulp.
		if got := p(angle.Deg(az)).Degrees(); math.Abs(got-7.5) > 1e-12 {
			t.Errorf("horizon at %g° = %g°, want 7.5° everywhere", az, got)
		}
	}
}

// TestParseHorizonProfileRefusesWhatIsNotATable: every line it cannot read is
// an error naming the line, never a profile that guesses.
func TestParseHorizonProfileRefusesWhatIsNotATable(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name, table string
	}{
		{"empty", ""},
		{"only comments", "# nothing\n; here\n"},
		{"one number", "90\n"},
		{"not numbers", "north high\n"},
		{"an altitude above the zenith", "90 91\n"},
		{"an altitude below the nadir", "90 -91\n"},
		{"a NaN altitude", "90 NaN\n"},
		{"an infinite azimuth", "Inf 10\n"},
		{"a line too long to be a point", strings.Repeat("9", maxHorizonProfileLine+1) + " 10\n"},
	} {
		if _, err := ParseHorizonProfile(strings.NewReader(c.table)); !errors.Is(err, ErrHorizonProfile) {
			t.Errorf("%s: err = %v, want ErrHorizonProfile", c.name, err)
		}
	}
}

// TestParseHorizonProfileDrivesTheHorizonConstraint: a parsed profile set on
// a site is what Site.HorizonAt and the Horizon constraint read.
func TestParseHorizonProfileDrivesTheHorizonConstraint(t *testing.T) {
	t.Parallel()

	p, err := ParseHorizonProfile(strings.NewReader("0 5\n90 30\n180 5\n270 5\n"))
	if err != nil {
		t.Fatal(err)
	}

	site, err := NewSiteEarthLocation("garden", 51.48, 0, 46, WithHorizonProfile(p))
	if err != nil {
		t.Fatal(err)
	}

	if got := site.HorizonAt(angle.Deg(90)).Degrees(); math.Abs(got-30) > 1e-12 {
		t.Errorf("HorizonAt(90°) = %g°, want the table's 30°", got)
	}
}
