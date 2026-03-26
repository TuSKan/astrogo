package ephemeris_test

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/body"
	"github.com/TuSKan/astrogo/earth"
	"github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/observatory"
	"github.com/TuSKan/astrogo/sky"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

func TestSunAltitudeMovement(t *testing.T) {
	loc, _ := earth.NewGeodetic(angle.Deg(0), angle.Deg(45), 0)
	site, _ := observatory.NewSite("Test", loc, angle.Zero(), nil)
	p := ephemeris.Default()

	// Noon (roughly) at long 0
	tm := time.FromJD(2460000.0, time.UTC)

	// Get Sun position
	vecStart, err := ephemeris.Position(p, body.Sun, tm)
	testutil.AssertNoError(t, err)

	posStart, err := ephemeris.ToICRS(vecStart)
	testutil.AssertNoError(t, err)

	aaStart, _ := sky.AltAz(posStart, tm, site)

	tmLate := tm.AddDays(0.25) // +6 hours
	vecLate, err := ephemeris.Position(p, body.Sun, tmLate)
	testutil.AssertNoError(t, err)

	posLate, err := ephemeris.ToICRS(vecLate)
	testutil.AssertNoError(t, err)

	aaLate, _ := sky.AltAz(posLate, tmLate, site)

	t.Logf("Sun Alt @ Noon: %.2f", aaStart.Alt.Degrees())
	t.Logf("Sun Alt @ Eve:  %.2f", aaLate.Alt.Degrees())

	if aaStart.Alt.Degrees() == aaLate.Alt.Degrees() {
		t.Error("Sun altitude should change over 6 hours")
	}
}

func TestMoonPosition(t *testing.T) {
	p := ephemeris.Default()
	tm := time.NowUTC()

	vec, err := ephemeris.Position(p, body.Moon, tm)
	testutil.AssertNoError(t, err)

	pos, err := ephemeris.ToICRS(vec)
	testutil.AssertNoError(t, err)

	t.Logf("Moon ICRS: RA=%.2f Dec=%.2f", pos.RA.Degrees(), pos.Dec.Degrees())

	if pos.Dec.Degrees() > 30 || pos.Dec.Degrees() < -30 {
		t.Error("Moon declination is usually within +/- 30 degrees")
	}
}

func TestStateAndHelpers(t *testing.T) {
	p := ephemeris.Default()
	tm := time.NowUTC()

	st, err := p.State(body.Sun, tm)
	testutil.AssertNoError(t, err)

	pos, err := ephemeris.Position(p, body.Sun, tm)
	testutil.AssertNoError(t, err)

	vel, err := ephemeris.Velocity(p, body.Sun, tm)
	testutil.AssertNoError(t, err)

	if pos != st.Pos {
		t.Error("Position helper result mismatch with State")
	}
	if vel != st.Vel {
		t.Error("Velocity helper result mismatch with State")
	}
}

func TestToICRSZeroVector(t *testing.T) {
	_, err := ephemeris.ToICRS(vector.Vec3{})
	if err == nil {
		t.Error("Expected error for zero vector conversion")
	}
}

func TestUnsupportedBody(t *testing.T) {
	p := ephemeris.Default()
	tm := time.NowUTC()

	_, err := p.State(body.Mars, tm)
	if err == nil {
		t.Error("Expected error for unsupported body (Mars) in sofa provider")
	}
}
