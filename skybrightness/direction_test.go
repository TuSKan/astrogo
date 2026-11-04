package skybrightness_test

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/skybrightness"
)

// nonFiniteDirections are the directions a caller can build that no component
// can evaluate: a NaN or infinite altitude, or azimuth.
func nonFiniteDirections() map[string]coord.AltAz {
	nan, inf := math.NaN(), math.Inf(1)

	return map[string]coord.AltAz{
		"altitude NaN":  coord.NewAltAz(angle.Deg(nan), angle.Deg(90)),
		"azimuth NaN":   coord.NewAltAz(angle.Deg(45), angle.Deg(nan)),
		"altitude +Inf": coord.NewAltAz(angle.Deg(inf), angle.Deg(90)),
		"azimuth -Inf":  coord.NewAltAz(angle.Deg(45), angle.Deg(-inf)),
	}
}

// The direction is the caller's input, so Model.Estimate refuses a non-finite
// one itself, by name. Left to the components, it came back as one of them
// failing, under a message about radiance, airmass or optical depth (#1000).
// The model here holds a component that ignores the direction altogether, so
// only Estimate's own check can refuse it.
func TestEstimateRefusesANonFiniteDirection(t *testing.T) {
	t.Parallel()

	m, err := skybrightness.NewModel("test", constantComponent{id: skybrightness.Starlight, value: 1e-9})
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}

	scene := testScene(t)

	for name, dir := range nonFiniteDirections() {
		if _, err := m.Estimate(context.Background(), skybrightness.Query{Scene: scene, Direction: dir}); !errors.Is(err, skybrightness.ErrDirection) {
			t.Errorf("%s: err %v, want ErrDirection", name, err)
		}
	}

	if _, err := m.Estimate(context.Background(), skybrightness.Query{
		Scene: scene, Direction: coord.NewAltAz(angle.Deg(45), angle.Deg(90)),
	}); err != nil {
		t.Errorf("a finite direction: %v", err)
	}
}

// CloudySkyglow's only look at the direction was its horizon test, which a NaN
// altitude passes, so a non-finite direction came out as NaN in every bin with
// no error, clear or overcast (#1000). Called directly, it must refuse one.
func TestCloudySkyglowRefusesANonFiniteDirection(t *testing.T) {
	t.Parallel()

	c, err := skybrightness.NewCloudySkyglow([]skybrightness.GroundEmitter{cloudyCity(t, 0, 5)})
	if err != nil {
		t.Fatalf("NewCloudySkyglow: %v", err)
	}

	grid := skybrightness.DefaultOpticalGrid()

	for _, sky := range []struct {
		name           string
		base, fraction float64
	}{{"clear", 0, 0}, {"overcast", 1000, 0.7}} {
		scene := cloudyScene(t, sky.base, sky.fraction)

		for name, dir := range nonFiniteDirections() {
			_, err := c.AddRadiance(context.Background(), skybrightness.NewSpectralRadiance(grid), grid, dir, scene)
			if !errors.Is(err, skybrightness.ErrDirection) {
				t.Errorf("%s sky, %s: err %v, want ErrDirection", sky.name, name, err)
			}
		}

		dst := skybrightness.NewSpectralRadiance(grid)
		if _, err := c.AddRadiance(context.Background(), dst, grid, coord.NewAltAz(angle.Deg(45), angle.Deg(90)), scene); err != nil {
			t.Errorf("%s sky, a finite direction: %v", sky.name, err)
		} else if err := dst.Validate(); err != nil {
			t.Errorf("%s sky, a finite direction: %v", sky.name, err)
		}
	}
}
