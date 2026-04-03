package target

import (
	"errors"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/body"
	"github.com/TuSKan/astrogo/catalog"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

func TestFixed(t *testing.T) {
	obj := catalog.Target{
		Name:  "Orion Nebula",
		Coord: coord.ICRS{RA: angle.Hour(5.5), Dec: angle.Deg(-5.5)},
	}
	f := Fixed{Object: obj}

	if f.Name() != "Orion Nebula" {
		t.Errorf("expected name Orion Nebula, got %s", f.Name())
	}

	pos, err := f.Position(time.Now())
	testutil.AssertNoError(t, err)

	if pos.RA.Radians() != obj.Coord.RA.Radians() || pos.Dec.Radians() != obj.Coord.Dec.Radians() {
		t.Errorf("expected position %+v, got %+v", obj.Coord, pos)
	}
}

func TestCustom(t *testing.T) {
	c1 := coord.ICRS{RA: angle.Deg(10), Dec: angle.Deg(20)}

	// Named custom
	target1 := Custom{Label: "My Point", Coord: c1}
	if target1.Name() != "My Point" {
		t.Errorf("expected name My Point, got %s", target1.Name())
	}
	pos1, err := target1.Position(time.Now())
	testutil.AssertNoError(t, err)
	if pos1.RA.Radians() != c1.RA.Radians() {
		t.Errorf("expected position %+v, got %+v", c1, pos1)
	}

	// Default custom
	target2 := Custom{Coord: c1}
	if target2.Name() != "Custom" {
		t.Errorf("expected name Custom, got %s", target2.Name())
	}
}

func TestInterface(t *testing.T) {
	var _ Observable = Fixed{}
	var _ Observable = Custom{}
}

func TestZeroValueSafety(t *testing.T) {
	// Fixed with zero-value catalog.Target
	f := Fixed{}
	if f.Name() != "" {
		t.Errorf("expected empty name for zero-value Fixed")
	}
	pos, err := f.Position(time.Now())
	testutil.AssertNoError(t, err)
	if pos.RA.Radians() != 0 || pos.Dec.Radians() != 0 {
		t.Errorf("expected zero position for zero-value Fixed")
	}

	// Custom with zero-value
	c := Custom{}
	if c.Name() != "Custom" {
		t.Errorf("expected name Custom for zero-value Custom")
	}
	pos2, err := c.Position(time.Now())
	testutil.AssertNoError(t, err)
	if pos2.RA.Radians() != 0 || pos2.Dec.Radians() != 0 {
		t.Errorf("expected zero position for zero-value Custom")
	}
}

func TestBody(t *testing.T) {
	p := ephemeris.Default()
	now := time.Now()

	// Sun
	sun := Body{ID: body.Sun, Provider: p}
	if sun.Name() != "Sun" {
		t.Errorf("expected name Sun, got %s", sun.Name())
	}
	pos, err := sun.Position(now)
	testutil.AssertNoError(t, err)
	if pos.RA.Radians() == 0 && pos.Dec.Radians() == 0 {
		t.Error("expected non-zero position for Sun")
	}

	// Moon
	moon := Body{ID: body.Moon, Provider: p}
	if moon.Name() != "Moon" {
		t.Errorf("expected name Moon, got %s", moon.Name())
	}
	pos2, err := moon.Position(now)
	testutil.AssertNoError(t, err)
	if pos2.RA.Radians() == 0 && pos2.Dec.Radians() == 0 {
		t.Error("expected non-zero position for Moon")
	}

	// Deterministic repeated calls
	pos3, _ := sun.Position(now)
	if pos.RA.Radians() != pos3.RA.Radians() || pos.Dec.Radians() != pos3.Dec.Radians() {
		t.Error("repeated calls for same time should return same position")
	}
}

type mockMarsProvider struct{}

func (m mockMarsProvider) State(id body.ID, _ time.Time) (ephemeris.State, error) {
	if id == body.Mars {
		return ephemeris.State{Pos: vector.V3(1.5, 0, 0)}, nil
	}
	return ephemeris.State{}, errors.New("unsupported body")
}

func TestBodyMars(t *testing.T) {
	mars := Body{ID: body.Mars, Provider: mockMarsProvider{}}
	pos, err := mars.Position(time.Now())
	testutil.AssertNoError(t, err)

	// X=1.5, Y=0, Z=0 in AU -> RA=0, Dec=0
	if pos.RA.Degrees() != 0 || pos.Dec.Degrees() != 0 {
		t.Errorf("expected RA=0 Dec=0 for Mars at X=1.5, got %v", pos)
	}
}

func TestBodyErrors(t *testing.T) {
	// Nil provider
	b1 := Body{ID: body.Sun}
	_, err := b1.Position(time.Now())
	testutil.AssertError(t, err)

	// Unsupported body (using sofaProvider for Mars)
	b2 := Body{ID: body.Mars, Provider: ephemeris.Default()}
	_, err = b2.Position(time.Now())
	testutil.AssertError(t, err)
}
