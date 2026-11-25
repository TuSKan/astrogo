package plan

import (
	"errors"
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/epoch"
	mag "github.com/TuSKan/astrogo/magnitude"
	"github.com/TuSKan/astrogo/unit"
	"github.com/TuSKan/astrogo/vector"
)

// Satellite represents an artificial satellite with an SGP4/TLE-based provider.
type Satellite struct {
	provider   eph.Provider
	name       string
	stdMag     float64
	convention mag.StdMagConvention
	phaseModel mag.SatPhaseModel
	id         eph.ID
	hasStdMag  bool
}

// SatelliteOption configures optional Satellite fields.
type SatelliteOption func(*Satellite)

// WithStdMag sets the satellite's standard magnitude and convention.
func WithStdMag(stdMag float64, conv mag.StdMagConvention) SatelliteOption {
	return func(s *Satellite) {
		s.stdMag = stdMag
		s.convention = conv
		s.hasStdMag = true
	}
}

// WithPhaseModel sets the satellite's phase function model (sphere or cylinder).
func WithPhaseModel(model mag.SatPhaseModel) SatelliteOption {
	return func(s *Satellite) { s.phaseModel = model }
}

// NewSatellite creates a satellite target.
func NewSatellite(name string, id eph.ID, provider eph.Provider, opts ...SatelliteOption) *Satellite {
	s := &Satellite{name: name, id: id, provider: provider}
	for _, opt := range opts {
		opt(s)
	}

	return s
}

// Name returns the satellite's display name.
func (s *Satellite) Name() string { return s.name }

// Provider returns the ephemeris provider for this satellite.
func (s *Satellite) Provider() eph.Provider { return s.provider }

// EphID returns the NAIF/NORAD ID for ephemeris lookups.
func (s *Satellite) EphID() eph.ID { return s.id }

// Position computes the ICRS position of the satellite at time t.
func (s *Satellite) Position(t epoch.Time) (coord.ICRS, error) {
	pos, err := geocentric(s.provider, s.id, t)
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
func (s *Satellite) GeocentricVec(t epoch.Time) (vector.Vec3, error) {
	v, err := geocentric(s.provider, s.id, t)
	if err != nil {
		return vector.Vec3{}, fmt.Errorf("satellite: geocentric: %w", err)
	}

	return v, nil
}

// GetDetails computes the position and visual magnitude of the satellite.
func (s *Satellite) GetDetails(ctx *coord.Context, over DetailOverrides) (*TargetDetails, error) {
	return computeDetails(s, ctx, over)
}

// errNoObserverCtx is returned when satellite magnitude is called without a context.
var errNoObserverCtx = errors.New("satellite: apparent magnitude requires observer context (use ApparentMagnitudeCtx)")

// errNoStdMag is returned when satellite magnitude is requested without a standard magnitude.
var errNoStdMag = errors.New("satellite: no standard magnitude set")

// errDegenerateGeometry is returned when satellite magnitude geometry is degenerate.
var errDegenerateGeometry = errors.New("satellite magnitude: degenerate geometry")

// ApparentMagnitude cannot be computed for a satellite without observer context.
// Use [Satellite.ApparentMagnitudeCtx] instead.
func (s *Satellite) ApparentMagnitude(_ epoch.Time) (float64, error) {
	return 0, errNoObserverCtx
}

// ApparentMagnitudeCtx computes the satellite's apparent visual magnitude
// using the topocentric range from [LookAngle] and the Sun–Satellite–Observer
// phase angle. Requires that [WithStdMag] was set at construction.
func (s *Satellite) ApparentMagnitudeCtx(t epoch.Time, ctx *coord.Context) (float64, error) {
	if !s.hasStdMag {
		return 0, fmt.Errorf("%w: %s", errNoStdMag, s.name)
	}

	if ctx == nil {
		return 0, errNoObserverCtx
	}

	// Get topocentric range via LookAngle.
	altaz, err := LookAngle(s.provider, s.id, ctx)
	if err != nil {
		return 0, fmt.Errorf("satellite magnitude: look angle: %w", err)
	}

	observerRange := altaz.Dist()

	// Compute phase angle: Sun–Satellite–Observer.
	// The Sun's position always comes from the analytic SOFA provider, not
	// s.provider: a bare SGP4/TLE provider (satellite.NewFromTLE) tracks
	// exactly one body and ignores the requested ID, so querying it for
	// eph.Sun would silently return the satellite's own state again, making
	// sunToSat the zero vector below and this always fail with
	// errDegenerateGeometry.
	sunSt, err := eph.At(eph.Default(), t).Geometric(eph.Sun, eph.Earth)
	if err != nil {
		return 0, fmt.Errorf("satellite magnitude: sun state: %w", err)
	}

	satSt, err := eph.At(s.provider, t).Geometric(s.id, eph.Earth)
	if err != nil {
		return 0, fmt.Errorf("satellite magnitude: sat state: %w", err)
	}

	// Sun→Satellite vector (heliocentric satellite position).
	sunToSat := satSt.Pos.Sub(sunSt.Pos)
	// Observer→Satellite vector (topocentric, already in the reducer).
	obsToSat := satSt.Pos.Sub(ctx.ObsVec())

	// Phase angle = angle between Sun→Sat and Obs→Sat vectors.
	// cos(α) = dot(sunToSat, obsToSat) / (|sunToSat| · |obsToSat|)
	dot := sunToSat.X*obsToSat.X + sunToSat.Y*obsToSat.Y + sunToSat.Z*obsToSat.Z
	norm1 := sunToSat.Norm()
	norm2 := obsToSat.Norm()

	if norm1 == 0 || norm2 == 0 {
		return 0, errDegenerateGeometry
	}

	// An eclipsed satellite reflects nothing and has no visual magnitude.
	// Until #562 it was given one anyway: the ISS on a midnight pass,
	// entirely inside Earth's shadow, came out at −3.9.
	if satelliteEclipsed(satSt.Pos, sunSt.Pos) {
		return 0, fmt.Errorf("%w: %s at %v", ErrSatelliteEclipsed, s.name, t)
	}

	cosAlpha := math.Max(-1, math.Min(1, dot/(norm1*norm2)))
	alpha := angle.Rad(math.Acos(cosAlpha))

	return mag.SatelliteApparent(s.stdMag, s.convention, observerRange, alpha, s.phaseModel), nil
}

// satelliteEclipsed reports whether a satellite at satPos is in Earth's
// shadow: whether Earth's sphere lies on the line from the satellite toward
// the Sun's center at sunPos, both geocentric and in AU.
//
// It is Skyfield's is_sunlit definition — the Sun's center, a spherical Earth
// of the WGS 84 equatorial radius, no penumbra and no atmosphere — chosen so
// the two can be compared directly; the Sun's 16′ disk and refraction in the
// atmosphere shift the real shadow edge by seconds, not minutes.
func satelliteEclipsed(satPos, sunPos vector.Vec3) bool {
	radius := earthEquatorialRadiusKm / auKm

	if satPos.Norm() <= radius {
		return true
	}

	// Along toSun from the satellite, where the line comes closest to Earth's
	// center. Behind the satellite, the line only moves away from the sphere.
	toSun := sunPos.Sub(satPos)
	along := -satPos.Dot(toSun) / toSun.Dot(toSun)

	if along <= 0 {
		return false
	}

	closest := satPos.Add(toSun.MulScalar(along))

	return closest.Norm() < radius
}

// StaticMagnitude returns the catalog standard magnitude if set.
func (s *Satellite) StaticMagnitude() (float64, bool) {
	return s.stdMag, s.hasStdMag
}

// kmPerAU is the number of kilometers in one Astronomical Unit.
//
// var, not const: constants.IAU.AstronomicalUnit is a struct, and Go does not
// permit selecting a struct field inside a constant expression.
var kmPerAU = constants.IAU.AstronomicalUnit.Value / 1e3

// defaultAtm is used for satellite pass prediction when no atmosphere is specified.
var defaultAtm = atmosphere.Refraction{}

// contextLocator is a provider that can place its body about the Earth with
// the precession-nutation of a Context in hand, rather than evaluating it
// again at the Context's instant: [eph.Satellite], whose Locate is otherwise
// 99% that evaluation (#812).
type contextLocator interface {
	LocateCtx(id eph.ID, ctx *coord.Context) (eph.State, error)
}

// LookAngle computes the topocentric look angle (altitude, azimuth, distance)
// from an observer to any celestial body at the Context's time.
//
// This works for both satellite and planetary providers — any Provider that
// returns a valid State. The observer's geocentric position is subtracted from
// the body's, so the range is right for a LEO satellite as well as a planet.
//
// It observes through ctx as it is, a Context moved with Context.SetTime
// included, and builds no other: the altitude and azimuth are ctx's
// refracted GeocentricToObserved, and the range subtracts ctx's observer
// vector.
func LookAngle(prov eph.Provider, id eph.ID, ctx *coord.Context) (coord.AltAz, error) {
	var (
		st  eph.State
		err error
	)

	if c, ok := prov.(contextLocator); ok {
		st, err = c.LocateCtx(id, ctx)
	} else {
		st, err = eph.At(prov, ctx.Time()).Geometric(id, eph.Earth)
	}

	if err != nil {
		return coord.AltAz{}, fmt.Errorf("satellite: look angle state: %w", err)
	}

	observed := ctx.GeocentricToObserved(st.Pos)

	// Context works in AU; the distance is stored as a typed length, so the
	// unit it is read back in is the reader's choice rather than a convention
	// this line has to be remembered for.
	observed.SetDist(unit.Km(st.Pos.Sub(ctx.ObsVec()).Norm() * kmPerAU))

	return observed, nil
}

// SatellitePass represents a single pass of a satellite over an observer.
//
// A pass already above the minimum elevation when the search window opens,
// or still above it when the window closes, is cut off at that edge rather
// than dropped: its Rise or Set is then the window's start or end, not a
// crossing, and RiseCutOff or SetCutOff says so. Its Culmination is the
// highest point inside the window, which on a cut-off pass can be the edge
// itself, and its Duration is the time inside the window.
type SatellitePass struct {
	Name        string        // Satellite name
	Rise        PassEvent     // AOS (Acquisition of Signal), or the window's start
	Culmination PassEvent     // TCA (Time of Closest Approach / max elevation)
	Set         PassEvent     // LOS (Loss of Signal), or the window's end
	Duration    unit.Duration // Total pass duration, inside the window

	// RiseCutOff reports that the satellite was already above the minimum
	// elevation when the window opened, so Rise is the window's start and the
	// real rise is earlier.
	RiseCutOff bool

	// SetCutOff reports that the satellite was still above the minimum
	// elevation when the window closed, so Set is the window's end and the
	// real set is later.
	SetCutOff bool
}

// PassEvent captures a time + topocentric coordinates for pass events, and
// the light the satellite is in.
//
// Seen by eye, a satellite is a point of reflected sunlight against a dark
// sky: it is visible where Sunlit is true while the Sun is below the
// horizon, best past civil twilight, SunAltitude below −6°. A pass can be
// visible at one event and not another, entering Earth's shadow before it
// sets.
type PassEvent struct {
	Time      epoch.Time
	Azimuth   angle.Angle
	Elevation angle.Angle
	Range     unit.Length

	// Sunlit reports whether the satellite is in sunlight at Time, by
	// Skyfield's is_sunlit definition: the Sun's center, a spherical Earth of
	// the WGS 84 equatorial radius, no penumbra.
	Sunlit bool

	// SunAltitude is the Sun's geometric altitude at the site at Time, its
	// center with no refraction.
	SunAltitude angle.Angle
}

// PassOption configures [SatellitePasses].
type PassOption func(*passSearch)

// passSearch is what a pass search is configured with.
type passSearch struct {
	minElevation angle.Angle
}

// WithMinElevation sets the elevation the satellite must clear for its time
// above it to count as a pass. The default is 0°, the geometric horizon.
func WithMinElevation(e angle.Angle) PassOption {
	return func(p *passSearch) { p.minElevation = e }
}

// SatellitePasses computes all passes of a satellite over an observer site
// within the given time window, filtered by minimum elevation. A pass in
// progress when the window opens or closes is cut off at that edge and
// marked, as [SatellitePass] describes, so a window opened during a pass, or
// lying wholly inside one, still returns it.
//
// The function uses a grid-sampling approach with 30-second steps (appropriate
// for LEO satellites with ~90 minute periods) and Chandrupatla root-finding
// for sub-second rise/set boundary refinement. A pass above minElevation for
// less than one step can lie wholly between two samples, so every sampled
// maximum below it is refined to its true peak, which finds such a pass
// wherever the window starts.
//
// sat is the satellite and the provider that places it, by its NAIF code, as
// [NewSatellite] builds it; its name labels the returned passes. A pass is
// the time the satellite spends above the minimum elevation, 0°, the
// geometric horizon, unless [WithMinElevation] sets another; the elevations
// are geometric, without refraction, and the site's own horizon does not
// change them.
func SatellitePasses(sat *Satellite, site *Site, w Window, opts ...PassOption) ([]SatellitePass, error) {
	if sat == nil {
		return nil, ErrNilTarget
	}

	if site == nil {
		return nil, ErrNilSite
	}

	start, end := w.Start, w.End
	if err := checkInterval(start, end); err != nil {
		return nil, err
	}

	search := passSearch{}
	for _, opt := range opts {
		opt(&search)
	}

	prov, id, name := sat.provider, sat.id, sat.name
	observer, minElevation := site.Location(), search.minElevation

	step := unit.Seconds(30) // 30s steps for LEO
	refineTol := unit.Seconds(1)

	// lookAt computes the look angle at t.
	//
	// The Context comes from a cache rather than a fresh coord.NewContext per
	// call. Decomposed on the ISS at Paranal, a rebuild is 145.5 µs against
	// 94.8 µs for the satellite's State and 24 ns for the transform itself, so
	// 61% of every 30-second sample went on rebuilding state that barely
	// changes — see movingContext and #166.
	//
	// That State was itself 99% the same thing: a fresh precession-nutation
	// evaluation, at every sample. LookAngle now places the satellite with
	// the Context's own (#812), and the sample is SGP4 and a rotation. It
	// also makes the reuse free of error: the moving Context's interpolated
	// precession-nutation turns the satellite and the observer alike, and
	// cancels from the look angle, under 0.01 mas from a fresh Context's
	// (TestMovingContextStaysInsideItsStatedBound).
	//
	// The satellite is asked for by its own NAIF code. It used to be asked
	// for as 0, which a satellite provider answered and which is the Solar
	// System barycenter's code now that an ID is a NAIF code (#809).
	ctxAt, err := movingContext(start, observer, defaultAtm)
	if err != nil {
		return nil, err
	}

	lookAt := func(t epoch.Time) (coord.AltAz, error) {
		return LookAngle(prov, id, ctxAt(t))
	}

	// Elevation evaluation function.
	evalEl := func(t epoch.Time) (float64, error) {
		altaz, err := lookAt(t)
		if err != nil {
			return 0, err
		}

		return altaz.Alt().Degrees() - minElevation.Degrees(), nil
	}

	// passEvent builds a PassEvent from a LookAngle call.
	passEvent := func(t epoch.Time) (PassEvent, error) {
		altaz, err := lookAt(t)
		if err != nil {
			return PassEvent{}, err
		}

		sunlit, sunAlt, err := sunlight(prov, id, t, ctxAt(t))
		if err != nil {
			return PassEvent{}, err
		}

		return PassEvent{
			Time:        t,
			Azimuth:     altaz.Az(),
			Elevation:   altaz.Alt(),
			Range:       altaz.Dist(),
			Sunlit:      sunlit,
			SunAltitude: sunAlt,
		}, nil
	}

	// Sample elevation over the window.
	n := int(end.Sub(start)/step) + 2
	times := make([]epoch.Time, 0, n)
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

	// A pass that clears minElevation for less than one step can fall
	// between two samples that are both below it. Until #897 it was not
	// found at all, or was, depending on where the window started. Its peak
	// now joins the samples.
	times, vals, err = withSteppedOverPeaks(times, vals, evalEl, func(from, to epoch.Time) (epoch.Time, error) {
		return peakTime(prov, id, ctxAt, from, to)
	})
	if err != nil {
		return nil, fmt.Errorf("satellite passes: %w", err)
	}

	// Find rise/set crossings and build passes.
	solver := NewEventSolver(step, refineTol)

	var (
		passes      []SatellitePass
		currentPass *SatellitePass
	)

	// closePass ends the pass in progress at setTime, a set crossing or the
	// window's end, and finds its culmination between its two ends.
	closePass := func(setTime epoch.Time) error {
		var err error

		if currentPass.Set, err = passEvent(setTime); err != nil {
			return fmt.Errorf("set: %w", err)
		}

		currentPass.Duration = setTime.Sub(currentPass.Rise.Time)

		if currentPass.Culmination, err = findCulmination(prov, id, observer, ctxAt, currentPass.Rise.Time, setTime); err != nil {
			return fmt.Errorf("culmination: %w", err)
		}

		passes = append(passes, *currentPass)
		currentPass = nil

		return nil
	}

	// A pass already above the threshold when the window opens has no rise
	// crossing inside it. Until #896 its set was then ignored and the pass
	// dropped, so asking during a pass found none. It is opened at the
	// window's start instead, and marked.
	if vals[0] > 0 {
		rise, err := passEvent(times[0])
		if err != nil {
			return nil, fmt.Errorf("satellite passes: window start: %w", err)
		}

		currentPass = &SatellitePass{Name: name, Rise: rise, RiseCutOff: true}
	}

	// The samples bracket each crossing, so every failure from here on is the
	// propagation failing, and is returned: a pass dropped, or reported
	// without its culmination, is indistinguishable from a sky without one
	// (#454).
	for i := range len(times) - 1 {
		v1, v2 := vals[i], vals[i+1]

		// Rise crossing: elevation goes above minimum.
		if v1 <= 0 && v2 > 0 {
			riseTime, _, err := solver.refineRoot(evalEl, times[i], times[i+1], v1, v2)
			if err != nil {
				return nil, fmt.Errorf("satellite passes: rise near %v: %w", times[i], err)
			}

			rise, err := passEvent(riseTime)
			if err != nil {
				return nil, fmt.Errorf("satellite passes: rise: %w", err)
			}

			currentPass = &SatellitePass{Name: name, Rise: rise}
		}

		// Set crossing: elevation drops below minimum.
		if v1 > 0 && v2 <= 0 && currentPass != nil {
			setTime, _, err := solver.refineRoot(evalEl, times[i], times[i+1], v1, v2)
			if err != nil {
				return nil, fmt.Errorf("satellite passes: set near %v: %w", times[i], err)
			}

			if err := closePass(setTime); err != nil {
				return nil, fmt.Errorf("satellite passes: %w", err)
			}
		}
	}

	// And one still above it when the window closes is closed at the
	// window's end, which the last sample always is.
	if currentPass != nil {
		currentPass.SetCutOff = true

		if err := closePass(times[len(times)-1]); err != nil {
			return nil, fmt.Errorf("satellite passes: window end: %w", err)
		}
	}

	return passes, nil
}

// findCulmination finds the point of maximum elevation during a pass
// by sampling at 5-second intervals and refining the peak.
//
// Samples are looked up through ctxAt, the moving Context the pass scan
// itself uses, rather than a full coord.NewContext at each one: it built
// about 130 per pass, each a full evaluation of the precession-nutation
// series, where one Context moved with Context.SetTime serves them all
// (#476). The culmination it reports is computed from a full Context at the
// chosen instant, as rise and set are, so the moving Context decides only
// which sample wins.
func findCulmination(prov eph.Provider, id eph.ID, observer *coord.Geodetic,
	ctxAt func(epoch.Time) *coord.Context, start, end epoch.Time,
) (PassEvent, error) {
	bestTime, err := peakTime(prov, id, ctxAt, start, end)
	if err != nil {
		return PassEvent{}, err
	}

	ctx, err := coord.NewContext(bestTime, observer, defaultAtm)
	if err != nil {
		return PassEvent{}, fmt.Errorf("plan: culmination: %w", err)
	}

	altaz, err := LookAngle(prov, id, ctx)
	if err != nil {
		return PassEvent{}, err
	}

	sunlit, sunAlt, err := sunlight(prov, id, bestTime, ctx)
	if err != nil {
		return PassEvent{}, err
	}

	return PassEvent{
		Time:        bestTime,
		Azimuth:     altaz.Az(),
		Elevation:   altaz.Alt(),
		Range:       altaz.Dist(),
		Sunlit:      sunlit,
		SunAltitude: sunAlt,
	}, nil
}

// sunlight returns whether the satellite id is in sunlight at t, by
// satelliteEclipsed, and the Sun's geometric altitude from ctx's site, which
// ctx must be at t. The Sun comes from the analytic SOFA provider rather than
// prov, for the reason ApparentMagnitudeCtx gives: a satellite provider
// tracks one body and answers every ID with it.
func sunlight(prov eph.Provider, id eph.ID, t epoch.Time, ctx *coord.Context) (sunlit bool, sunAlt angle.Angle, err error) {
	sunSt, err := eph.At(eph.Default(), t).Geometric(eph.Sun, eph.Earth)
	if err != nil {
		return false, 0, fmt.Errorf("satellite pass: sun state: %w", err)
	}

	satSt, err := eph.At(prov, t).Geometric(id, eph.Earth)
	if err != nil {
		return false, 0, fmt.Errorf("satellite pass: satellite state: %w", err)
	}

	return !satelliteEclipsed(satSt.Pos, sunSt.Pos), ctx.GeocentricToObserved(sunSt.Pos).Alt(), nil
}

// peakTime is the instant of highest elevation between start and end, by
// sampling at 5-second intervals and refining the best of them at 1 second,
// through ctxAt. It is findCulmination without the full Context its reported
// event is computed from, which is all withSteppedOverPeaks needs.
func peakTime(prov eph.Provider, id eph.ID,
	ctxAt func(epoch.Time) *coord.Context, start, end epoch.Time,
) (epoch.Time, error) {
	step := unit.Seconds(5)
	bestTime := start
	bestEl := -90.0

	for t := start; !t.After(end); t = t.Add(step) {
		altaz, err := LookAngle(prov, id, ctxAt(t))
		if err != nil {
			return epoch.Time{}, err
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

	for t := refineStart; !t.After(refineEnd); t = t.Add(unit.Seconds(1)) {
		altaz, err := LookAngle(prov, id, ctxAt(t))
		if err != nil {
			return epoch.Time{}, err
		}

		if altaz.Alt().Degrees() > bestEl {
			bestEl = altaz.Alt().Degrees()
			bestTime = t
		}
	}

	return bestTime, nil
}

// withSteppedOverPeaks returns the samples with the peak of every pass the
// grid stepped over inserted among them, in time order.
//
// A sampled maximum that is not above zero is where such a pass would be: the
// true peak lies between the samples on either side of it, and peakAt finds
// it. A peak that clears zero is added, and the samples on either side of it
// then bracket its rise and set; one that does not is the answer, that
// nothing reached the threshold there. A maximum already above zero has a
// sign change on each side and needs nothing.
func withSteppedOverPeaks(
	times []epoch.Time, vals []float64,
	eval func(epoch.Time) (float64, error),
	peakAt func(from, to epoch.Time) (epoch.Time, error),
) ([]epoch.Time, []float64, error) {
	// The peaks found, each beside the sample it was found around. The
	// samples are copied only when there is one to insert, which a window
	// with no grazing pass never has.
	type steppedOver struct {
		i int
		t epoch.Time
		v float64
	}

	var found []steppedOver

	for i := 1; i < len(times)-1; i++ {
		if vals[i] > 0 || vals[i] <= vals[i-1] || vals[i] < vals[i+1] {
			continue
		}

		peak, err := peakAt(times[i-1], times[i+1])
		if err != nil {
			return nil, nil, fmt.Errorf("peak near %v: %w", times[i], err)
		}

		before := peak.Before(times[i]) && peak.After(times[i-1])
		after := peak.After(times[i]) && peak.Before(times[i+1])

		if !before && !after {
			continue
		}

		v, err := eval(peak)
		if err != nil {
			return nil, nil, fmt.Errorf("peak at %v: %w", peak, err)
		}

		if v > 0 {
			found = append(found, steppedOver{i: i, t: peak, v: v})
		}
	}

	if len(found) == 0 {
		return times, vals, nil
	}

	outT := make([]epoch.Time, 0, len(times)+len(found))
	outV := make([]float64, 0, len(vals)+len(found))

	k := 0

	for i := range times {
		var p *steppedOver
		if k < len(found) && found[k].i == i {
			p = &found[k]
			k++
		}

		if p != nil && p.t.Before(times[i]) {
			outT, outV = append(outT, p.t), append(outV, p.v)
		}

		outT, outV = append(outT, times[i]), append(outV, vals[i])

		if p != nil && p.t.After(times[i]) {
			outT, outV = append(outT, p.t), append(outV, p.v)
		}
	}

	return outT, outV, nil
}
