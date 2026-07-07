package angle_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/internal/testutil"
)

const (
	tol     = 1e-12 // tight tolerance for unit conversions (round-trip safe)
	trigTol = 1e-14 // tolerance for stdlib trig cross-checks
)

// ── Constructors ──────────────────────────────────────────────────────────────

func TestRad(t *testing.T) {
	testutil.AssertNear(t, "Rad(1)", angle.Rad(1).Radians(), 1.0, 0)
	testutil.AssertNear(t, "Rad(0)", angle.Rad(0).Radians(), 0.0, 0)
	testutil.AssertNear(t, "Rad(-π)", angle.Rad(-math.Pi).Radians(), -math.Pi, 0)
}

func TestDeg(t *testing.T) {
	testutil.AssertNear(t, "Deg(0)", angle.Deg(0).Degrees(), 0, 0)
	testutil.AssertNear(t, "Deg(90)", angle.Deg(90).Degrees(), 90, tol)
	testutil.AssertNear(t, "Deg(180)", angle.Deg(180).Degrees(), 180, tol)
	testutil.AssertNear(t, "Deg(-90)", angle.Deg(-90).Degrees(), -90, tol)
	testutil.AssertNear(t, "Deg(360)", angle.Deg(360).Degrees(), 360, tol)
	// Wrapping edge cases
	testutil.AssertNear(t, "Wrap2Pi(-epsilon)", angle.Rad(-1e-15).Wrap2Pi().Radians(), 2*math.Pi, 1e-14)
	testutil.AssertNear(t, "Wrap2Pi(2pi + epsilon)", angle.Rad(2*math.Pi+1e-15).Wrap2Pi().Radians(), 0, 1e-14)
}

func TestHMSStringEdge(t *testing.T) {
	// 23h 59m 59.999s with precision 2 should round properly or stay < 24h
	a := angle.Hour(23.99999)

	s := a.HMSString(2)
	if s >= "24h" {
		t.Errorf("HMSString(23.99999) returned %q, expected < 24h", s)
	}
}

func TestArcmin(t *testing.T) {
	// 60 arcmin = 1 degree
	testutil.AssertNear(t, "Arcmin(60)=1°", angle.Arcmin(60).Degrees(), 1.0, tol)
	testutil.AssertNear(t, "Arcmin(-30)=-0.5°", angle.Arcmin(-30).Degrees(), -0.5, tol)
}

func TestArcsec(t *testing.T) {
	// 3600 arcsec = 1 degree
	testutil.AssertNear(t, "Arcsec(3600)=1°", angle.Arcsec(3600).Degrees(), 1.0, tol)
	testutil.AssertNear(t, "Arcsec(1).Arcseconds()", angle.Arcsec(1).Arcseconds(), 1.0, tol)
}

func TestHour(t *testing.T) {
	// 24 hours = 360 degrees
	testutil.AssertNear(t, "Hour(24)=360°", angle.Hour(24).Degrees(), 360.0, tol)
	testutil.AssertNear(t, "Hour(6)=90°", angle.Hour(6).Degrees(), 90.0, tol)
	testutil.AssertNear(t, "Hour(0)=0", angle.Hour(0).Hours(), 0, 0)
}

// ── Cross-unit consistency ────────────────────────────────────────────────────

func TestCrossUnitConsistency(t *testing.T) {
	cases := []struct {
		name string
		a    angle.Angle
		deg  float64
	}{
		{"Deg(90)", angle.Deg(90), 90},
		{"Arcmin(90)", angle.Arcmin(5400), 90},
		{"Arcsec(324000)", angle.Arcsec(324000), 90},
		{"Hour(6)", angle.Hour(6), 90},
		{"zero", angle.Rad(0), 0},
		{"negative", angle.Deg(-45), -45},
	}
	for i, c := range cases {
		testutil.AssertNear(t, testutil.CaseLabel(i, c.name)+" .Degrees()", c.a.Degrees(), c.deg, tol)
	}
}

func TestAccessorRoundTrips(t *testing.T) {
	// Each constructor/accessor pair must be a lossless round-trip.
	vals := []float64{0, 1, -1, 90, -90, 180, 360, 0.001, 720}
	for _, v := range vals {
		testutil.AssertNear(t, "Deg round-trip", angle.Deg(v).Degrees(), v, tol)
	}

	for _, v := range []float64{0, 1, -1, 6, 12, 24} {
		testutil.AssertNear(t, "Hour round-trip", angle.Hour(v).Hours(), v, tol)
	}

	for _, v := range []float64{0, 1, 60, -1, 3600} {
		testutil.AssertNear(t, "Arcmin round-trip", angle.Arcmin(v).Arcminutes(), v, tol)
	}

	for _, v := range []float64{0, 1, 3600, -1} {
		testutil.AssertNear(t, "Arcsec round-trip", angle.Arcsec(v).Arcseconds(), v, tol)
	}
}

// ── Trigonometry ──────────────────────────────────────────────────────────────

func TestTrig(t *testing.T) {
	cases := []struct {
		name string
		a    angle.Angle
		sin  float64
		cos  float64
	}{
		{"0", angle.Rad(0), 0, 1},
		{"π/6", angle.Deg(30), 0.5, math.Sqrt(3) / 2},
		{"π/4", angle.Deg(45), math.Sqrt2 / 2, math.Sqrt2 / 2},
		{"π/3", angle.Deg(60), math.Sqrt(3) / 2, 0.5},
		{"π/2", angle.Deg(90), 1, 0},
		{"π", angle.Deg(180), 0, -1},
		{"3π/2", angle.Deg(270), -1, 0},
		{"2π", angle.Deg(360), 0, 1},
		{"-π/2", angle.Deg(-90), -1, 0},
	}
	for i, c := range cases {
		lbl := testutil.CaseLabel(i, c.name)
		testutil.AssertNear(t, lbl+".Sin()", c.a.Sin(), c.sin, trigTol)
		testutil.AssertNear(t, lbl+".Cos()", c.a.Cos(), c.cos, trigTol)
	}
}

func TestTan(t *testing.T) {
	testutil.AssertNear(t, "Tan(0)", angle.Deg(0).Tan(), 0, trigTol)
	testutil.AssertNear(t, "Tan(45°)", angle.Deg(45).Tan(), 1, trigTol)
	testutil.AssertNear(t, "Tan(-45°)", angle.Deg(-45).Tan(), -1, trigTol)
	// Tan(90°) using float64 π/2 is NOT exactly ±Inf: the nearest float64 to
	// π/2 is not exactly ±π/2, so math.Tan returns a very large finite value
	// (~1.633×10¹⁶). We verify it is very large rather than checking for Inf.
	tan90 := angle.Deg(90).Tan()
	if math.Abs(tan90) < 1e15 {
		t.Errorf("Tan(90°) = %v, want |tan| > 1e15", tan90)
	}
}

// ── Wrap2Pi ───────────────────────────────────────────────────────────────────

func TestWrap2Pi(t *testing.T) {
	const twoPi = 2 * math.Pi

	cases := []struct {
		name string
		in   float64 // radians
		want float64 // expected result
	}{
		// Already in range
		{"0", 0, 0},
		{"π/2", math.Pi / 2, math.Pi / 2},
		{"π", math.Pi, math.Pi},
		{"π+ε", math.Pi + 1e-10, math.Pi + 1e-10},
		// Exactly 2π maps to 0
		{"2π", twoPi, 0},
		// Above full circle
		{"3π", 3 * math.Pi, math.Pi},
		{"4π", 4 * math.Pi, 0},
		{"2π+0.1", twoPi + 0.1, 0.1},
		// Negative values
		{"-ε", -1e-15, twoPi - 1e-15},
		{"-π", -math.Pi, math.Pi},
		{"-π/2", -math.Pi / 2, 3 * math.Pi / 2},
		{"-2π", -twoPi, 0},
		{"-3π", -3 * math.Pi, math.Pi},
		// Very large — note: 100*math.Pi has its own float64 representation
		// error, so we only test 10π where both the input and result are
		// representable cleanly.
		{"10π", 10 * math.Pi, 0},
		{"-10π", -10 * math.Pi, 0},
	}
	for i, c := range cases {
		got := angle.Rad(c.in).Wrap2Pi().Radians()
		// Result must be in [0, 2π). We accept a tiny tolerance at the upper
		// boundary for floating-point residuals in the Mod computation.
		if got < -tol || got >= twoPi+tol {
			t.Errorf("%s: Wrap2Pi(%v) = %v, outside [0,2π)", testutil.CaseLabel(i, c.name), c.in, got)
		}

		testutil.AssertNear(t, testutil.CaseLabel(i, c.name), got, c.want, tol)
	}
}

// ── WrapPi ────────────────────────────────────────────────────────────────────

func TestWrapPi(t *testing.T) {
	cases := []struct {
		name string
		in   float64
		want float64
	}{
		{"0", 0, 0},
		{"π/2", math.Pi / 2, math.Pi / 2},
		// π is included in the upper bound
		{"π", math.Pi, math.Pi},
		{"3π", 3 * math.Pi, math.Pi},
		// Values > π fold negative
		{"3π/2", 3 * math.Pi / 2, -math.Pi / 2},
		{"2π-ε", 2*math.Pi - 1e-10, -1e-10},
		// -π maps to +π (lower bound excluded)
		{"-π", -math.Pi, math.Pi},
		{"-π/2", -math.Pi / 2, -math.Pi / 2},
		{"-3π/2", -3 * math.Pi / 2, math.Pi / 2},
		{"-2π", -2 * math.Pi, 0},
		{"-3π", -3 * math.Pi, math.Pi},
	}
	for i, c := range cases {
		got := angle.Rad(c.in).WrapPi().Radians()
		// Result must be in (-π, π]
		if got <= -math.Pi || got > math.Pi {
			t.Errorf("%s: WrapPi(%v) = %v, outside (-π,π]", testutil.CaseLabel(i, c.name), c.in, got)
		}

		testutil.AssertNear(t, testutil.CaseLabel(i, c.name), got, c.want, tol)
	}
}

// ── Wrap360 / Wrap180 (alias correctness) ─────────────────────────────────────

func TestWrap360_equalsWrap2Pi(t *testing.T) {
	inputs := []float64{-360, -180, -90, 0, 90, 180, 270, 360, 540}
	for _, deg := range inputs {
		a := angle.Deg(deg)
		if a.Wrap360() != a.Wrap2Pi() {
			t.Errorf("Wrap360(%v°) ≠ Wrap2Pi(%v°)", deg, deg)
		}
	}
}

func TestWrap180_equalsWrapPi(t *testing.T) {
	inputs := []float64{-360, -180, -90, 0, 90, 180, 270, 360, 540}
	for _, deg := range inputs {
		a := angle.Deg(deg)
		if a.Wrap180() != a.WrapPi() {
			t.Errorf("Wrap180(%v°) ≠ WrapPi(%v°)", deg, deg)
		}
	}
}

// ── Arithmetic ────────────────────────────────────────────────────────────────

func TestArithmetic(t *testing.T) {
	a := angle.Deg(90)
	b := angle.Deg(45)

	testutil.AssertNear(t, "Add", a.Add(b).Degrees(), 135, tol)
	testutil.AssertNear(t, "Sub", a.Sub(b).Degrees(), 45, tol)
	testutil.AssertNear(t, "MulScalar(2)", a.MulScalar(2).Degrees(), 180, tol)
	testutil.AssertNear(t, "MulScalar(-1)", a.MulScalar(-1).Degrees(), -90, tol)
	testutil.AssertNear(t, "DivScalar(2)", a.DivScalar(2).Degrees(), 45, tol)
	testutil.AssertNear(t, "DivScalar(1)", a.DivScalar(1).Radians(), a.Radians(), tol)
}

func TestDivScalar_zero_isInf(t *testing.T) {
	result := angle.Deg(1).DivScalar(0).Radians()
	if !math.IsInf(result, 1) {
		t.Errorf("DivScalar(0) = %v, want +Inf", result)
	}
}

// ── Formatting ────────────────────────────────────────────────────────────────

func TestString(t *testing.T) {
	cases := []struct {
		want string
		deg  float64
	}{
		{"0.0000°", 0},
		{"90.0000°", 90},
		{"-45.5000°", -45.5},
	}
	for i, c := range cases {
		got := angle.Deg(c.deg).String()
		if got != c.want {
			testutil.FailCase(t, i, c.want, "String() = %q, want %q", got, c.want)
		}
	}
}

func TestDMSString(t *testing.T) {
	cases := []struct {
		name      string
		want      string
		deg       float64
		precision int
	}{
		{"zero p0", `+00°00'00"`, 0, 0},
		{"90 p0", `+90°00'00"`, 90, 0},
		{"negative p0", `-45°00'00"`, -45, 0},
		{"30'30\" p2", `+00°30'30.00"`, 0.5 + 30.0/3600, 2},
		// Carry: 59.995 s → rounds to 00.0 with carry to minutes
		{"carry p1", `+00°01'00.0"`, 0 + 59.995/3600, 1},
		// Known angle: Orion belt roughly at −1°12′6.9″
		{"known angle p1", `-01°12'06.9"`, -(1 + 12.0/60 + 6.9/3600), 1},
		// Regression: negative precision must still carry like p0 — this
		// previously skipped the carry check entirely (guarded on
		// precision >= 0) and rendered the invalid `20'60"`.
		{"carry negative precision", `+10°21'00"`, 10 + 20.0/60 + 59.6/3600, -1},
		{"carry p0 (same value)", `+10°21'00"`, 10 + 20.0/60 + 59.6/3600, 0},
	}
	for i, c := range cases {
		got := angle.Deg(c.deg).DMSString(c.precision)
		if got != c.want {
			testutil.FailCase(t, i, c.name, "DMSString(%d) = %q, want %q", c.precision, got, c.want)
		}
	}
}

func TestHMSString(t *testing.T) {
	cases := []struct {
		name      string
		want      string
		hours     float64
		precision int
	}{
		{"0h p0", "00h00m00s", 0, 0},
		{"6h p0", "06h00m00s", 6, 0},
		{"23h59m p1", "23h59m00.0s", 23 + 59.0/60, 1},
		// Values > 24h are normalised
		{"25h=1h p0", "01h00m00s", 25, 0},
		// Fractional seconds
		{"2h30m15.5s p1", "02h30m15.5s", 2 + 30.0/60 + 15.5/3600, 1},
		// Regression: negative precision must still carry, matching p0 —
		// see the identical DMSString regression case above.
		{"carry negative precision", "10h21m00s", 10 + 20.0/60 + 59.6/3600, -1},
	}
	for i, c := range cases {
		got := angle.Hour(c.hours).HMSString(c.precision)
		if got != c.want {
			testutil.FailCase(t, i, c.name, "HMSString(%d) = %q, want %q", c.precision, got, c.want)
		}
	}
}

// ── Parsing ───────────────────────────────────────────────────────────────────

func TestParseDMS(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantDeg float64
		wantErr bool
	}{
		// Degree-symbol separators
		{"positive", `+12°34'56.78"`, 12.0 + 34.0/60 + 56.78/3600, false},
		{"negative", `-12°34'56.78"`, -(12.0 + 34.0/60 + 56.78/3600), false},
		{"no sign", `45°00'00"`, 45, false},
		// Colon separators
		{"colon positive", "+12:34:56.78", 12.0 + 34.0/60 + 56.78/3600, false},
		{"colon negative", "-12:34:56.78", -(12.0 + 34.0/60 + 56.78/3600), false},
		// Partial fields
		{"degrees only", "30", 30, false},
		{"degrees+minutes", "30:15", 30.25, false},
		// Zero
		{"zero", `+00°00'00.00"`, 0, false},
		// Error cases
		{"empty", "", 0, true},
		{"bad minutes", `10°61'00"`, 0, true},
		{"bad seconds", `10°00'60"`, 0, true},
	}
	for i, c := range cases {
		got, err := angle.ParseDMS(c.input)

		lbl := testutil.CaseLabel(i, c.name)
		if c.wantErr {
			if err == nil {
				t.Errorf("%s: expected error for %q, got none", lbl, c.input)
			}

			continue
		}

		testutil.AssertNoError(t, err)
		testutil.AssertNear(t, lbl, got.Degrees(), c.wantDeg, tol)
	}
}

func TestParseHMS(t *testing.T) {
	cases := []struct {
		name      string
		input     string
		wantHours float64
		wantErr   bool
	}{
		{"letter sep", "12h34m56.78s", 12.0 + 34.0/60 + 56.78/3600, false},
		{"colon sep", "12:34:56.78", 12.0 + 34.0/60 + 56.78/3600, false},
		{"zero", "00h00m00s", 0, false},
		{"negative HA", "-06h00m00s", -6, false},
		{"hours only", "6", 6, false},
		// Error cases
		{"bad minutes", "12h61m00s", 0, true},
		{"bad seconds", "12h00m60s", 0, true},
		{"empty", "", 0, true},
	}
	for i, c := range cases {
		got, err := angle.ParseHMS(c.input)

		lbl := testutil.CaseLabel(i, c.name)
		if c.wantErr {
			if err == nil {
				t.Errorf("%s: expected error for %q, got none", lbl, c.input)
			}

			continue
		}

		testutil.AssertNoError(t, err)
		testutil.AssertNear(t, lbl, got.Hours(), c.wantHours, tol)
	}
}

// ── Round-trip: format → parse ────────────────────────────────────────────────

func TestDMSRoundTrip(t *testing.T) {
	inputs := []float64{0, 90, -45, 23.456789, -89.999}
	for _, deg := range inputs {
		orig := angle.Deg(deg)
		str := orig.DMSString(4)
		parsed, err := angle.ParseDMS(str)
		testutil.AssertNoError(t, err)
		// Round-trip tolerance: 4 decimal places on arcseconds ≈ 1e-4/3600 degrees
		const rtTol = 1e-4 / 3600
		testutil.AssertNear(t, "DMS round-trip "+str, parsed.Degrees(), deg, rtTol)
	}
}

func TestHMSRoundTrip(t *testing.T) {
	inputs := []float64{0, 6, 12, 18, 23.99, 1.5}
	for _, h := range inputs {
		orig := angle.Hour(h)
		str := orig.HMSString(4)
		parsed, err := angle.ParseHMS(str)
		testutil.AssertNoError(t, err)
		// Round-trip tolerance: 4 decimal places on seconds of time ≈ 1e-4/3600 hours
		const rtTol = 1e-4 / 3600
		testutil.AssertNear(t, "HMS round-trip "+str, parsed.Hours(), h, rtTol)
	}
}
