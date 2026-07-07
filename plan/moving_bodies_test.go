package plan

import (
	"errors"
	"fmt"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// R29 regression: plan/asteroid.go, plan/comet.go, and plan/generic.go had
// zero test coverage under default `go test ./...` — every exported
// constructor/method (NewAsteroid, NewComet, NewGenericBody, Position,
// GeocentricVec, GetDetails, ApparentMagnitude(Ctx)) was unexercised.
//
// testEphProvider is a deterministic, offline eph.Provider reproducing the
// same Ceres-opposition geometry already validated live in
// magnitude/imcce_test.go (r=2.77 AU heliocentric, Δ=1.77 AU geocentric,
// α=0°): Sun geocentric position on the -X axis at 1 AU, target on the +X
// axis at 1.77 AU, so target-minus-Sun gives exactly r=2.77 AU with zero
// phase angle.
type testEphProvider struct {
	targetID  eph.ID
	targetPos vector.Vec3
	sunPos    vector.Vec3
}

func newOppositionProvider(targetID eph.ID) *testEphProvider {
	return &testEphProvider{
		targetID:  targetID,
		targetPos: vector.Vec3{X: 1.77},
		sunPos:    vector.Vec3{X: -1.0},
	}
}

var errUnknownTestBody = errors.New("testEphProvider: unknown body id")

func (p *testEphProvider) State(id eph.ID, _ time.Time) (eph.State, error) {
	switch id {
	case eph.Sun:
		return eph.State{Pos: p.sunPos}, nil
	case p.targetID:
		return eph.State{Pos: p.targetPos}, nil
	default:
		return eph.State{}, fmt.Errorf("%w: %d", errUnknownTestBody, id)
	}
}

func (p *testEphProvider) Close() error { return nil }

func testContext(t *testing.T) *coord.Context {
	t.Helper()

	loc, err := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	site, err := NewSite("Test", loc, angle.Zero(), nil)
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}

	return coord.NewContext(time.FromJD(2451545.0, time.UTC), loc, site.Atmosphere())
}

func TestAsteroid_HG_OppositionMagnitude(t *testing.T) {
	const asteroidID eph.ID = 2000001 // Ceres' SPK ID convention (20000+1)

	prov := newOppositionProvider(asteroidID)
	a := NewAsteroid("Ceres", asteroidID, prov, WithHG(3.34, 0.12))

	tm := time.FromJD(2451545.0, time.UTC)

	mag, err := a.ApparentMagnitude(tm)
	if err != nil {
		t.Fatalf("ApparentMagnitude: %v", err)
	}

	// At opposition (α=0°), HG reduces to H + 5·log10(r·Δ) — no phase
	// darkening term. Cross-checked against magnitude/imcce_test.go's live
	// Ceres-opposition case (same r, Δ, H).
	want := 3.34 + 5*math.Log10(2.77*1.77)
	if math.Abs(mag-want) > 0.01 {
		t.Errorf("ApparentMagnitude = %.3f, want %.3f (H + 5log10(r*delta) at opposition)", mag, want)
	}

	magCtx, err := a.ApparentMagnitudeCtx(tm, nil)
	if err != nil {
		t.Fatalf("ApparentMagnitudeCtx: %v", err)
	}

	if magCtx != mag {
		t.Errorf("ApparentMagnitudeCtx = %.3f, want it to match ApparentMagnitude = %.3f", magCtx, mag)
	}

	if a.Name() != "Ceres" {
		t.Errorf("Name() = %q, want Ceres", a.Name())
	}

	if a.EphID() != asteroidID {
		t.Errorf("EphID() = %v, want %v", a.EphID(), asteroidID)
	}
}

func TestAsteroid_HG1G2AndSHG1G2(t *testing.T) {
	const asteroidID eph.ID = 2000002

	prov := newOppositionProvider(asteroidID)
	tm := time.FromJD(2451545.0, time.UTC)

	hg1g2 := NewAsteroid("Test HG1G2", asteroidID, prov, WithHG1G2(10.0, 0.3, 0.2))

	m1, err := hg1g2.ApparentMagnitude(tm)
	if err != nil {
		t.Fatalf("HG1G2 ApparentMagnitude: %v", err)
	}

	if m1 <= 0 || m1 > 30 {
		t.Errorf("HG1G2 magnitude = %.3f, out of plausible range", m1)
	}

	spin := NewAsteroid("Test sHG1G2", asteroidID, prov,
		WithHG1G2(10.0, 0.3, 0.2), WithSpin(45, 30, 0.9))

	m2, err := spin.ApparentMagnitude(tm)
	if err != nil {
		t.Fatalf("sHG1G2 ApparentMagnitude: %v", err)
	}

	// The spin-geometry correction is a modest adjustment on top of HG1G2 —
	// same order of magnitude, not a wildly different value.
	if math.Abs(m2-m1) > 2.0 {
		t.Errorf("sHG1G2 magnitude %.3f diverges too far from HG1G2 %.3f", m2, m1)
	}
}

func TestAsteroid_PositionAndGetDetails(t *testing.T) {
	const asteroidID eph.ID = 2000003

	prov := newOppositionProvider(asteroidID)
	a := NewAsteroid("Test Asteroid", asteroidID, prov, WithHG(15.0, 0.15))

	tm := time.FromJD(2451545.0, time.UTC)

	icrs, err := a.Position(tm)
	if err != nil {
		t.Fatalf("Position: %v", err)
	}

	// Target is on the +X axis with no Y/Z component: RA=Dec=0.
	if math.Abs(icrs.RA().Degrees()) > 1e-9 || math.Abs(icrs.Dec().Degrees()) > 1e-9 {
		t.Errorf("Position = (RA=%v, Dec=%v), want (0,0)", icrs.RA().Degrees(), icrs.Dec().Degrees())
	}

	vec, err := a.GeocentricVec(tm)
	if err != nil {
		t.Fatalf("GeocentricVec: %v", err)
	}

	if math.Abs(vec.Norm()-1.77) > 1e-9 {
		t.Errorf("GeocentricVec norm = %v, want 1.77 AU", vec.Norm())
	}

	d, err := a.GetDetails(testContext(t))
	if err != nil {
		t.Fatalf("GetDetails: %v", err)
	}

	if d.Name != "Test Asteroid" {
		t.Errorf("GetDetails Name = %q, want %q", d.Name, "Test Asteroid")
	}
}

func TestComet_ApparentMagnitudeAndDetails(t *testing.T) {
	const cometID eph.ID = 1000001

	prov := newOppositionProvider(cometID)
	c := NewComet("Test Comet", cometID, prov, 6.0, 10.0, WithNuclearMagnitude(11.0, 5.0))

	tm := time.FromJD(2451545.0, time.UTC)

	m, err := c.ApparentMagnitude(tm)
	if err != nil {
		t.Fatalf("ApparentMagnitude: %v", err)
	}

	// IAU standard: m = M1 + 5*log10(delta) + K1*log10(r).
	want := 6.0 + 5*math.Log10(1.77) + 10.0*math.Log10(2.77)
	if math.Abs(m-want) > 0.01 {
		t.Errorf("ApparentMagnitude = %.3f, want %.3f", m, want)
	}

	mCtx, err := c.ApparentMagnitudeCtx(tm, nil)
	if err != nil {
		t.Fatalf("ApparentMagnitudeCtx: %v", err)
	}

	if mCtx != m {
		t.Errorf("ApparentMagnitudeCtx = %.3f, want it to match ApparentMagnitude = %.3f", mCtx, m)
	}

	if c.Name() != "Test Comet" {
		t.Errorf("Name() = %q, want %q", c.Name(), "Test Comet")
	}

	if c.EphID() != cometID {
		t.Errorf("EphID() = %v, want %v", c.EphID(), cometID)
	}

	icrs, err := c.Position(tm)
	if err != nil {
		t.Fatalf("Position: %v", err)
	}

	if math.Abs(icrs.RA().Degrees()) > 1e-9 {
		t.Errorf("Position RA = %v, want 0", icrs.RA().Degrees())
	}

	vec, err := c.GeocentricVec(tm)
	if err != nil {
		t.Fatalf("GeocentricVec: %v", err)
	}

	if math.Abs(vec.Norm()-1.77) > 1e-9 {
		t.Errorf("GeocentricVec norm = %v, want 1.77 AU", vec.Norm())
	}

	d, err := c.GetDetails(testContext(t))
	if err != nil {
		t.Fatalf("GetDetails: %v", err)
	}

	if d.Name != "Test Comet" {
		t.Errorf("GetDetails Name = %q, want %q", d.Name, "Test Comet")
	}
}

func TestGenericBody_PositionAndDetails(t *testing.T) {
	const bodyID eph.ID = 3000001

	prov := newOppositionProvider(bodyID)
	g := NewGenericBody("Unknown Body", bodyID, prov)

	if g.Name() != "Unknown Body" {
		t.Errorf("Name() = %q, want %q", g.Name(), "Unknown Body")
	}

	if g.EphID() != bodyID {
		t.Errorf("EphID() = %v, want %v", g.EphID(), bodyID)
	}

	if g.Provider() != prov {
		t.Error("Provider() did not return the constructor-supplied provider")
	}

	tm := time.FromJD(2451545.0, time.UTC)

	icrs, err := g.Position(tm)
	if err != nil {
		t.Fatalf("Position: %v", err)
	}

	if math.Abs(icrs.RA().Degrees()) > 1e-9 {
		t.Errorf("Position RA = %v, want 0", icrs.RA().Degrees())
	}

	vec, err := g.GeocentricVec(tm)
	if err != nil {
		t.Fatalf("GeocentricVec: %v", err)
	}

	if math.Abs(vec.Norm()-1.77) > 1e-9 {
		t.Errorf("GeocentricVec norm = %v, want 1.77 AU", vec.Norm())
	}

	// GenericBody deliberately does NOT implement MagnitudeComputer — confirm
	// GetDetails doesn't report a spurious magnitude for it.
	d, err := g.GetDetails(testContext(t))
	if err != nil {
		t.Fatalf("GetDetails: %v", err)
	}

	if d.Name != "Unknown Body" {
		t.Errorf("GetDetails Name = %q, want %q", d.Name, "Unknown Body")
	}

	if _, ok := any(g).(MagnitudeComputer); ok {
		t.Error("GenericBody must not implement MagnitudeComputer")
	}
}

func TestGenericBody_PositionError(t *testing.T) {
	const bodyID eph.ID = 3000002

	prov := newOppositionProvider(9999999) // provider only knows a different ID
	g := NewGenericBody("Broken Body", bodyID, prov)

	if _, err := g.Position(time.FromJD(2451545.0, time.UTC)); err == nil {
		t.Error("expected an error when the provider doesn't recognize the body's ID")
	}
}
