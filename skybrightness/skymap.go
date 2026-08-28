package skybrightness

import (
	"context"
	"errors"
	"fmt"
	"math"
	"runtime"

	"golang.org/x/sync/errgroup"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/unit"
)

// ErrRingCount is returned when a sky map is asked for a non-positive
// number of altitude rings.
var ErrRingCount = errors.New("skybrightness: sky map needs at least one altitude ring")

// Zenith evaluates the sky straight up.
func (m *Model) Zenith(ctx context.Context, q Query) (*Estimate, error) {
	q.Direction = coord.NewAltAz(angle.Deg(90), angle.Zero())

	return m.Estimate(ctx, q)
}

// Direction evaluates the sky in one altitude/azimuth direction. It is
// Estimate with the direction spelled out, for callers who find that
// clearer at a call site.
func (m *Model) Direction(ctx context.Context, q Query, alt, az angle.Angle) (*Estimate, error) {
	q.Direction = coord.NewAltAz(alt, az)

	return m.Estimate(ctx, q)
}

// SkyPoint is one sample of an all-sky map.
type SkyPoint struct {
	// Direction is where this sample was evaluated.
	Direction coord.AltAz

	// Estimate is the sky state there.
	Estimate *Estimate

	// SolidAngleSR is the solid angle this sample represents, used when
	// integrating the map over the hemisphere.
	SolidAngleSR float64
}

// SkyMap evaluates the sky over the visible hemisphere on a ring grid:
// rings of constant altitude from the horizon to the zenith, each with
// enough azimuth samples to keep the angular spacing roughly uniform.
//
// The map is built by evaluating the physical model in each direction. It
// is emphatically not a zenith value spread outward by an assumed falloff
// curve: directional structure — a light dome on one horizon, the Moon,
// the Galactic plane — is produced by the solver, because that structure
// is most of what an all-sky prediction is for.
func (m *Model) SkyMap(ctx context.Context, q Query, rings int) ([]SkyPoint, error) {
	if rings < 1 {
		return nil, fmt.Errorf("%w: got %d", ErrRingCount, rings)
	}

	if err := q.Scene.Validate(); err != nil {
		return nil, err
	}

	// Before the hemisphere is sampled, not per direction: a mismatch is a
	// property of the query, so failing here costs one check instead of one
	// per direction, and reports the whole map as unevaluated rather than
	// half-built.
	if err := m.checkPreset(q); err != nil {
		return nil, err
	}

	// The incoming field, once for the whole map.
	//
	// Eq. 11's L_0 is the radiance above the atmosphere: it depends on where
	// the light comes from and not on where the observer looks, so it is the
	// same field for every direction of this map. Sampling it once turns the
	// expensive half of a reference-fidelity sky from tens of thousands of
	// component evaluations into a few hundred, and leaves each direction with
	// arithmetic that carries no coordinate transform and no transcendental.
	//
	// At any other fidelity there is no scattering integral, so nothing is
	// sampled and nothing is spent.
	if q.Fidelity == Reference {
		grid := q.grid()
		if err := grid.Validate(); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrNoGrid, err)
		}

		field, err := m.sampleHemisphere(ctx, q.Scene, grid, 0)
		if err != nil {
			return nil, err
		}

		q.field = field
	}

	// Every direction the map will hold, laid out before any is evaluated.
	//
	// Ring centres sit at half-steps so no sample lands exactly on the
	// horizon, where airmass diverges and most models leave their stated
	// validity domain.
	step := 90.0 / float64(rings)

	var out []SkyPoint

	for r := range rings {
		altDeg := step * (float64(r) + 0.5)

		// Azimuth samples scale with cos(alt) so the sky is covered at
		// roughly constant angular density: a ring near the zenith is
		// short and needs few samples, one near the horizon is long.
		count := max(int(math.Round(4*float64(rings)*math.Cos(altDeg*math.Pi/180))), 1)

		// Each sample owns a patch: one ring's altitude band divided by
		// the number of azimuth samples around it.
		lo := (altDeg - step/2) * math.Pi / 180
		hi := (altDeg + step/2) * math.Pi / 180
		ringSR := 2 * math.Pi * (math.Sin(hi) - math.Sin(lo))

		for a := range count {
			out = append(out, SkyPoint{
				Direction: coord.NewAltAz(
					angle.Deg(altDeg), angle.Deg(360*float64(a)/float64(count))),
				SolidAngleSR: ringSR / float64(count),
			})
		}
	}

	// Evaluated across cores.
	//
	// The directions are independent: each reads the scene, the components and
	// the shared incoming field, and writes only its own estimate. The
	// components hold their own caches behind mutexes — TestComponentsAreConcurrencySafe
	// is what says so — and the incoming field is written once before any
	// worker starts and only read after.
	//
	// A sky is thousands of directions and a reference-fidelity one is
	// milliseconds each, so this is the difference between a map a user waits
	// for and one they give up on. The order of out is unchanged: each worker
	// writes to its own index.
	workers := max(min(runtime.GOMAXPROCS(0), len(out)), 1)

	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(workers)

	for i := range out {
		group.Go(func() error {
			local := q
			local.Direction = out[i].Direction

			est, err := m.Estimate(groupCtx, local)
			if err != nil {
				return err
			}

			out[i].Estimate = est

			return nil
		})
	}

	if err := group.Wait(); err != nil {
		return nil, fmt.Errorf("skybrightness: sky map: %w", err)
	}

	return out, nil
}

// IntegratedHemisphere sums a sky map's radiance over the hemisphere,
// giving the total spectral flux arriving from the sky on a horizontal
// surface facing up, in W m^-2 nm^-1.
//
// Each sample is weighted by its own solid angle and by sin(alt), which is
// the cosine of the zenith angle: a patch near the horizon presents itself
// obliquely to a horizontal surface and contributes less than the same patch
// overhead. The code has always done this; the comment used to say cos(alt),
// which is that factor upside down - one at the horizon and zero at the
// zenith - and so described the opposite of both the physics and the
// sentence it introduced.
func IntegratedHemisphere(points []SkyPoint, grid unit.SpectralGrid) (SpectralRadiance, error) {
	out := NewSpectralRadiance(grid)

	for _, p := range points {
		if p.Estimate == nil {
			continue
		}

		if !p.Estimate.grid.Equal(grid) {
			return nil, fmt.Errorf("%w: sample grid %s, want %s",
				unit.ErrGridMismatch, p.Estimate.grid, grid)
		}

		w := p.SolidAngleSR * p.Direction.Alt().Sin()
		for i, v := range p.Estimate.total {
			out[i] += v * w
		}
	}

	return out, nil
}

// HorizontalIlluminance integrates a sky map into the spectrally
// integrated irradiance on an upward-facing horizontal surface, in W m^-2.
//
// This is a radiometric quantity. Photopic illuminance in lux is a
// different projection of the same spectrum, obtained by weighting with
// the CIE V(lambda) response through [magnitude], and the two must not be
// confused: the ratio between them depends on the sky's colour.
func HorizontalIlluminance(points []SkyPoint, grid unit.SpectralGrid) (unit.Irradiance, error) {
	spectrum, err := IntegratedHemisphere(points, grid)
	if err != nil {
		return 0, err
	}

	v, err := grid.Integrate(spectrum)
	if err != nil {
		return 0, fmt.Errorf("skybrightness: horizontal illuminance: %w", err)
	}

	return unit.Irradiance(v), nil
}
