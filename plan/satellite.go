package plan

import (
	"fmt"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// Satellite represents an artificial satellite with an SGP4/TLE-based provider.
type Satellite struct {
	provider eph.Provider
	name     string
	id       eph.ID
}

// NewSatellite creates a satellite target.
func NewSatellite(name string, id eph.ID, provider eph.Provider) *Satellite {
	return &Satellite{name: name, id: id, provider: provider}
}

// Name returns the satellite's display name.
func (s *Satellite) Name() string { return s.name }

// Provider returns the ephemeris provider for this satellite.
func (s *Satellite) Provider() eph.Provider { return s.provider }

// EphID returns the NAIF/NORAD ID for ephemeris lookups.
func (s *Satellite) EphID() eph.ID { return s.id }

// Position computes the ICRS position of the satellite at time t.
func (s *Satellite) Position(t time.Time) (coord.ICRS, error) {
	pos, err := eph.Position(s.provider, s.id, t)
	if err != nil {
		return coord.ICRS{}, fmt.Errorf("satellite: ephemeris error for %s: %w", s.name, err)
	}

	icrs, err := eph.ToICRS(pos)
	if err != nil {
		return coord.ICRS{}, fmt.Errorf("satellite: coordinate conversion error for %s: %w", s.name, err)
	}

	return icrs, nil
}

// GeocentricVec returns the geocentric position vector of the satellite.
func (s *Satellite) GeocentricVec(t time.Time) (vector.Vec3, error) {
	v, err := eph.Position(s.provider, s.id, t)
	if err != nil {
		return vector.Vec3{}, fmt.Errorf("satellite: geocentric: %w", err)
	}

	return v, nil
}

// GetDetails computes the position and visual magnitude of the satellite.
func (s *Satellite) GetDetails(ctx *coord.Context, props ...string) (*TargetDetails, error) {
	return computeDetails(s, ctx, props...)
}

// defaultAtm is used for satellite pass prediction when no atmosphere is specified.
var defaultAtm = atmosphere.Atmosphere{}

// LookAngle computes the topocentric look angle (altitude, azimuth, distance)
// from an observer to any celestial body at time t.
//
// This works for both satellite and planetary providers — any Provider that
// returns a valid State. Uses the coord.Reducer pipeline which correctly
// handles both nearby objects (LEO satellites) and distant bodies (planets)
// by computing the full topocentric vector (geocentric - observer).
func LookAngle(prov eph.Provider, id eph.ID, ctx *coord.Context) (coord.AltAz, error) {
	st, err := prov.State(id, ctx.Time())
	if err != nil {
		return coord.AltAz{}, fmt.Errorf("satellite: look angle state: %w", err)
	}

	// Use the Reducer pipeline: computes observer GCRS position, subtracts it
	// from the geocentric state, converts to ENU, then az/el. This gives the
	// correct topocentric range for nearby objects (satellites).
	reducer := coord.NewReducer(ctx.Site(), ctx.Time(), ctx.Atmosphere())
	reduction := reducer.Reduce(st.Pos)

	// The Reducer works in AU — convert topocentric distance to km.
	const kmPerAU = 149597870.7
	reduction.Observed.SetDist(reduction.Topocentric.Norm() * kmPerAU)

	return reduction.Observed, nil
}

// SatellitePass represents a single pass of a satellite over an observer.
type SatellitePass struct {
	Name        string        // Satellite name
	Rise        PassEvent     // AOS (Acquisition of Signal)
	Culmination PassEvent     // TCA (Time of Closest Approach / max elevation)
	Set         PassEvent     // LOS (Loss of Signal)
	Duration    time.Duration // Total pass duration
}

// PassEvent captures a time + topocentric coordinates for pass events.
type PassEvent struct {
	Time      time.Time
	Azimuth   angle.Angle
	Elevation angle.Angle
	Range     float64 // km
}

// SatellitePasses computes all passes of a satellite over an observer site
// within the given time window, filtered by minimum elevation.
//
// The function uses a grid-sampling approach with 30-second steps (appropriate
// for LEO satellites with ~90 minute periods) and Chandrupatla root-finding
// for sub-second rise/set boundary refinement.
func SatellitePasses(prov eph.Provider, name string, start, end time.Time,
	observer *coord.Geodetic, minElevation angle.Angle,
) ([]SatellitePass, error) {
	step := 30 * time.Second // 30s steps for LEO
	refineTol := 1 * time.Second

	// lookAt creates a context and computes look angle at time t.
	lookAt := func(t time.Time) (coord.AltAz, error) {
		ctx := coord.NewContext(t, observer, defaultAtm)
		return LookAngle(prov, 0, ctx)
	}

	// Elevation evaluation function.
	evalEl := func(t time.Time) (float64, error) {
		altaz, err := lookAt(t)
		if err != nil {
			return 0, err
		}

		return altaz.Alt().Degrees() - minElevation.Degrees(), nil
	}

	// passEvent builds a PassEvent from a LookAngle call.
	passEvent := func(t time.Time) PassEvent {
		altaz, _ := lookAt(t)

		return PassEvent{
			Time:      t,
			Azimuth:   altaz.Az(),
			Elevation: altaz.Alt(),
			Range:     altaz.Dist(),
		}
	}

	// Sample elevation over the window.
	n := int(end.Sub(start)/step) + 2
	times := make([]time.Time, 0, n)
	vals := make([]float64, 0, n)

	for t := start; !t.After(end); t = t.Add(step) {
		times = append(times, t)

		v, err := evalEl(t)
		if err != nil {
			return nil, err
		}

		vals = append(vals, v)
	}

	if last := times[len(times)-1]; last.Before(end) {
		times = append(times, end)

		v, err := evalEl(end)
		if err != nil {
			return nil, err
		}

		vals = append(vals, v)
	}

	// Find rise/set crossings and build passes.
	solver := NewEventSolver(step, refineTol)

	var (
		passes      []SatellitePass
		currentPass *SatellitePass
	)

	for i := range len(times) - 1 {
		v1, v2 := vals[i], vals[i+1]

		// Rise crossing: elevation goes above minimum.
		if v1 <= 0 && v2 > 0 {
			riseTime, _, err := solver.refineRoot(evalEl, times[i], times[i+1], v1)
			if err != nil {
				continue
			}

			currentPass = &SatellitePass{
				Name: name,
				Rise: passEvent(riseTime),
			}
		}

		// Set crossing: elevation drops below minimum.
		if v1 > 0 && v2 <= 0 && currentPass != nil {
			setTime, _, err := solver.refineRoot(evalEl, times[i], times[i+1], v1)
			if err != nil {
				currentPass = nil
				continue
			}

			currentPass.Set = passEvent(setTime)
			currentPass.Duration = setTime.Sub(currentPass.Rise.Time)

			// Find culmination (max elevation) between rise and set.
			culm, err := findCulmination(prov, observer, currentPass.Rise.Time, setTime)
			if err == nil {
				currentPass.Culmination = culm
			}

			passes = append(passes, *currentPass)
			currentPass = nil
		}
	}

	return passes, nil
}

// findCulmination finds the point of maximum elevation during a pass
// by sampling at 5-second intervals and refining the peak.
func findCulmination(prov eph.Provider, observer *coord.Geodetic,
	start, end time.Time,
) (PassEvent, error) {
	step := 5 * time.Second
	bestTime := start
	bestEl := -90.0

	for t := start; !t.After(end); t = t.Add(step) {
		ctx := coord.NewContext(t, observer, defaultAtm)

		altaz, err := LookAngle(prov, 0, ctx)
		if err != nil {
			continue
		}

		if altaz.Alt().Degrees() > bestEl {
			bestEl = altaz.Alt().Degrees()
			bestTime = t
		}
	}

	// Refine around the peak with 1-second steps.
	refineStart := bestTime.Add(-step)
	if refineStart.Before(start) {
		refineStart = start
	}

	refineEnd := bestTime.Add(step)
	if refineEnd.After(end) {
		refineEnd = end
	}

	for t := refineStart; !t.After(refineEnd); t = t.Add(1 * time.Second) {
		ctx := coord.NewContext(t, observer, defaultAtm)

		altaz, err := LookAngle(prov, 0, ctx)
		if err != nil {
			continue
		}

		if altaz.Alt().Degrees() > bestEl {
			bestEl = altaz.Alt().Degrees()
			bestTime = t
		}
	}

	ctx := coord.NewContext(bestTime, observer, defaultAtm)

	altaz, err := LookAngle(prov, 0, ctx)
	if err != nil {
		return PassEvent{}, err
	}

	return PassEvent{
		Time:      bestTime,
		Azimuth:   altaz.Az(),
		Elevation: altaz.Alt(),
		Range:     altaz.Dist(),
	}, nil
}
