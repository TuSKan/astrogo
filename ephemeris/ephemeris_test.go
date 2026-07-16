package ephemeris_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

func TestSunAltitudeMovement(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(45), 0)
	site, _ := plan.NewSite("Test", loc)
	p := eph.Default()

	// Noon (roughly) at long 0
	tm := time.FromJD(2460000.0, time.UTC)

	// Get Sun position
	vecStart, err := eph.Position(p, eph.Sun, tm)
	testutil.AssertNoError(t, err)

	posStart, err := eph.ToICRS(vecStart)
	testutil.AssertNoError(t, err)

	ctxStart := coord.NewContext(tm, site.Location(), atmosphere.StandardAtmosphere)
	aaStart, _ := ctxStart.ICRSToAltAz(posStart)

	tmLate := tm.AddDays(0.25) // +6 hours
	vecLate, err := eph.Position(p, eph.Sun, tmLate)
	testutil.AssertNoError(t, err)

	posLate, err := eph.ToICRS(vecLate)
	testutil.AssertNoError(t, err)

	ctxLate := coord.NewContext(tmLate, site.Location(), atmosphere.StandardAtmosphere)
	aaLate, _ := ctxLate.ICRSToAltAz(posLate)

	t.Logf("Sun Alt @ Noon: %.2f", aaStart.Alt().Degrees())
	t.Logf("Sun Alt @ Eve:  %.2f", aaLate.Alt().Degrees())

	if aaStart.Alt().Degrees() == aaLate.Alt().Degrees() {
		t.Error("Sun altitude should change over 6 hours")
	}
}

func TestMoonPosition(t *testing.T) {
	p := eph.Default()
	tm := time.NowUTC()

	vec, err := eph.Position(p, eph.Moon, tm)
	testutil.AssertNoError(t, err)

	pos, err := eph.ToICRS(vec)
	testutil.AssertNoError(t, err)

	t.Logf("Moon ICRS: RA=%.2f Dec=%.2f", pos.RA().Degrees(), pos.Dec().Degrees())

	if pos.Dec().Degrees() > 30 || pos.Dec().Degrees() < -30 {
		t.Error("Moon declination is usually within +/- 30 degrees")
	}
}

func TestStateAndHelpers(t *testing.T) {
	p := eph.Default()
	tm := time.NowUTC()

	st, err := p.State(eph.Sun, tm)
	testutil.AssertNoError(t, err)

	pos, err := eph.Position(p, eph.Sun, tm)
	testutil.AssertNoError(t, err)

	vel, err := eph.Velocity(p, eph.Sun, tm)
	testutil.AssertNoError(t, err)

	if pos != st.Pos {
		t.Error("Position helper result mismatch with State")
	}

	if vel != st.Vel {
		t.Error("Velocity helper result mismatch with State")
	}
}

func TestToICRSZeroVector(t *testing.T) {
	_, err := eph.ToICRS(vector.Vec3{})
	if err == nil {
		t.Error("Expected error for zero vector conversion")
	}
}

func TestUnsupportedBody(t *testing.T) {
	p := eph.Default()
	tm := time.NowUTC()

	_, err := p.State(eph.ID(999999), tm)
	if err == nil {
		t.Error("Expected error for unsupported body")
	}
}

const (
	lightAUPerDay = 173.144632674
	arcsecPerRad  = 206264.80624709636
)

type mockLinearProvider struct {
	baseTime time.Time
	pos      vector.Vec3
	vel      vector.Vec3
}

func (m *mockLinearProvider) State(_ eph.ID, t time.Time) (eph.State, error) {
	jd1Req, jd2Req := t.JDParts()
	jd1Base, jd2Base := m.baseTime.JDParts()
	dtDays := (jd1Req - jd1Base) + (jd2Req - jd2Base)

	p := m.pos.Add(m.vel.MulScalar(dtDays))

	return eph.State{Pos: p, Vel: m.vel}, nil
}

func (m *mockLinearProvider) Close() error { return nil }

func angularSepArcsec(a, b coord.AltAz) float64 {
	az1 := a.Az().Radians()
	alt1 := a.Alt().Radians()
	az2 := b.Az().Radians()
	alt2 := b.Alt().Radians()

	s1 := math.Sin(alt1)
	c1 := math.Cos(alt1)
	s2 := math.Sin(alt2)
	c2 := math.Cos(alt2)

	cosd := s1*s2 + c1*c2*math.Cos(az1-az2)
	if cosd > 1 {
		cosd = 1
	}

	if cosd < -1 {
		cosd = -1
	}

	return math.Acos(cosd) * arcsecPerRad
}

func iteratedApparentVector(st eph.State) vector.Vec3 {
	v := st.Pos

	tauDays := v.Norm() / lightAUPerDay
	for range 5 {
		iterPos := v.Sub(st.Vel.MulScalar(tauDays))
		tauDays = iterPos.Norm() / lightAUPerDay
	}

	return v.Sub(st.Vel.MulScalar(tauDays))
}

func TestApparentState_ZeroVelocityReducesToGeometric(t *testing.T) {
	tm := time.Date(2026, 4, 5, 0, 0, 0, 0, time.LocationUTC)

	mock := &mockLinearProvider{
		baseTime: tm,
		pos:      vector.V3(1.2, 0.4, 0.3),
		vel:      vector.Zero(),
	}

	appState, err := eph.ApparentState(mock, eph.Sun, tm)
	if err != nil {
		t.Fatalf("ApparentState failed: %v", err)
	}

	// With zero velocity, the retarded position must be exactly the geometric position.
	diff := appState.Pos.Sub(mock.pos)

	dist := diff.Norm()
	if dist > 1e-15 {
		t.Fatalf("zero-velocity case should return identical position; diff = %e AU", dist)
	}
}

func TestApparentState_MatchesManualLightTimeIteration(t *testing.T) {
	tm := time.Date(2026, 4, 5, 0, 0, 0, 0, time.LocationUTC)

	st := eph.State{
		Pos: vector.V3(1.0, 0.8, 0.2),
		Vel: vector.V3(-0.012, 0.009, 0.0015),
	}

	mock := &mockLinearProvider{
		baseTime: tm,
		pos:      st.Pos,
		vel:      st.Vel,
	}

	appState, _ := eph.ApparentState(mock, eph.Mars, tm)
	expected := iteratedApparentVector(st)

	// Compare retarded position vectors directly.
	diff := appState.Pos.Sub(expected)

	dist := diff.Norm()
	if dist > 1e-15 {
		t.Fatalf("ApparentState does not match explicit light-time reduction; diff = %e AU", dist)
	}
}

//nolint:dupl // shares structure with DistantObject test but uses different mock parameters.
func TestApparentState_LightTimeActuallyChangesResult(t *testing.T) {
	tm := time.Date(2026, 4, 5, 0, 0, 0, 0, time.LocationUTC)

	site, err := coord.NewGeodetic(angle.Deg(-155.4700), angle.Deg(19.8261), 4205)
	testutil.AssertNoError(t, err)

	atm := atmosphere.Atmosphere{}
	atm.Model = atmosphere.RefractionNone{}

	mock := &mockLinearProvider{
		baseTime: tm,
		pos:      vector.V3(4.0, 1.5, 0.2),
		vel:      vector.V3(-0.006, 0.010, 0.0008),
	}

	ctx := coord.NewContext(tm, site, atm)
	appState, _ := eph.ApparentState(mock, eph.Jupiter, tm)

	got := ctx.GeocentricToObserved(appState.Pos)
	geom := ctx.GeocentricToObserved(mock.pos)

	sep := angularSepArcsec(got, geom)
	if sep <= 0 {
		t.Fatalf("expected light-time correction to produce a non-zero angular shift")
	}

	if sep < 0.001 {
		t.Fatalf("expected measurable apparent shift, got only %.9f arcsec", sep)
	}
}

//nolint:dupl // shares structure with LightTime test but uses different mock parameters.
func TestApparentState_DistantObjectHasTinyCorrection(t *testing.T) {
	tm := time.Date(2026, 4, 5, 0, 0, 0, 0, time.LocationUTC)

	site, err := coord.NewGeodetic(angle.Deg(-17.8890), angle.Deg(28.7606), 2390)
	testutil.AssertNoError(t, err)

	atm := atmosphere.Atmosphere{}
	atm.Model = atmosphere.RefractionNone{}

	mock := &mockLinearProvider{
		baseTime: tm,
		pos:      vector.V3(40.0, 10.0, 2.0),
		vel:      vector.V3(-0.001, 0.0008, 0.0001),
	}

	ctx := coord.NewContext(tm, site, atm)
	appState, _ := eph.ApparentState(mock, eph.Jupiter, tm)

	got := ctx.GeocentricToObserved(appState.Pos)
	geom := ctx.GeocentricToObserved(mock.pos)

	sep := angularSepArcsec(got, geom)

	if sep < 0 {
		t.Fatalf("invalid negative separation")
	}

	if sep > 30 {
		t.Fatalf("distant slow object should not shift absurdly; got %.6f arcsec", sep)
	}
}
