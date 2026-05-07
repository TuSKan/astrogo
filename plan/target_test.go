package plan

import (
	"errors"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

func TestTargetFixed(t *testing.T) {
	obj := catalog.Target{
		Name:     "Orion Nebula",
		Coord:    coord.NewICRS(angle.Hour(5.5), angle.Deg(-5.5)),
		HasCoord: true,
	}
	f := NewTarget(obj, nil)

	if f.Name() != "Orion Nebula" {
		t.Errorf("expected name Orion Nebula, got %s", f.Name())
	}

	pos, err := f.Position(time.NowUTC())
	testutil.AssertNoError(t, err)

	if pos.RA().Radians() != obj.Coord.RA().Radians() || pos.Dec().Radians() != obj.Coord.Dec().Radians() {
		t.Errorf("expected position %+v, got %+v", obj.Coord, pos)
	}
}

func TestTargetZeroValueSafety(t *testing.T) {
	// Target with zero-value catalog.Target
	f := NewTarget(catalog.Target{}, nil)
	if f.Name() != "" {
		t.Errorf("expected empty name for zero-value Target")
	}
	pos, err := f.Position(time.NowUTC())
	testutil.AssertNoError(t, err)
	if pos.RA().Radians() != 0 || pos.Dec().Radians() != 0 {
		t.Errorf("expected zero position for zero-value Target")
	}
}

func TestTargetBody(t *testing.T) {
	p := eph.Default()
	now := time.NowUTC()

	// Sun
	sun := NewTarget(catalog.Target{ID: "11", Name: "Sun", Kind: resolve.KindStar}, p)
	if sun.Name() != "Sun" {
		t.Errorf("expected name Sun, got %s", sun.Name())
	}
	pos, err := sun.Position(now)
	testutil.AssertNoError(t, err)
	if pos.RA().Radians() == 0 && pos.Dec().Radians() == 0 {
		t.Error("expected non-zero position for Sun")
	}

	// Moon
	moon := NewTarget(catalog.Target{ID: "10", Name: "Moon", Kind: resolve.KindMoon}, p)
	if moon.Name() != "Moon" {
		t.Errorf("expected name Moon, got %s", moon.Name())
	}
	pos2, err := moon.Position(now)
	testutil.AssertNoError(t, err)
	if pos2.RA().Radians() == 0 && pos2.Dec().Radians() == 0 {
		t.Error("expected non-zero position for Moon")
	}

	// Deterministic repeated calls
	pos3, _ := sun.Position(now)
	if pos.RA().Radians() != pos3.RA().Radians() || pos.Dec().Radians() != pos3.Dec().Radians() {
		t.Error("repeated calls for same time should return same position")
	}
}

type mockMarsProvider struct{}

func (m mockMarsProvider) State(id eph.ID, _ time.Time) (eph.State, error) {
	if id == eph.Mars {
		return eph.State{Pos: vector.V3(1.5, 0, 0)}, nil
	}
	return eph.State{}, errors.New("unsupported body")
}

func (m mockMarsProvider) Close() error { return nil }

func TestTargetMars(t *testing.T) {
	mars := NewTarget(catalog.Target{ID: "4", Name: "Mars", Kind: resolve.KindPlanet}, mockMarsProvider{})
	pos, err := mars.Position(time.NowUTC())
	testutil.AssertNoError(t, err)

	// X=1.5, Y=0, Z=0 in AU -> RA=0, Dec=0
	if pos.RA().Degrees() != 0 || pos.Dec().Degrees() != 0 {
		t.Errorf("expected RA=0 Dec=0 for Mars at X=1.5, got %v", pos)
	}
}

func TestTargetErrors(t *testing.T) {
	// Moving object but nil provider
	b1 := Target{Catalog: catalog.Target{ID: "11", Kind: resolve.KindStar}, Provider: nil}
	_, err := b1.GeocentricVec(time.NowUTC())
	testutil.AssertError(t, err)

	// Unsupported body
	b2 := NewTarget(catalog.Target{ID: "999999", Kind: resolve.KindOther}, eph.Default())
	_, err = b2.Position(time.NowUTC())
	testutil.AssertError(t, err)
}
