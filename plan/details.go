package plan

import (
	"fmt"
	"maps"
	"strings"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
	"github.com/TuSKan/astrogo/vector"
)

// TargetDetails holds descriptive and ephemeris properties of an Observable.
type TargetDetails struct {
	ExtraProps map[string]string

	// RiseTime, TransitTime and SetTime are the target's pass at the
	// Context's time: the one in progress when the target is up, the next
	// one when it is down. Rise and set are the instants SunEvents,
	// MoonEvents and VisibilityEvents report, on the same horizon. A target
	// that neither rises nor sets has no RiseTime or SetTime, and its
	// TransitTime is its next culmination. RiseAzimuth, SetAzimuth and
	// MaxElevation go with them.
	SetTime        *time.Time
	TransitTime    *time.Time
	RiseTime       *time.Time
	AngularSize    string
	Description    string
	Source         string
	Magnitude      string
	RadialVelocity string
	Name           string

	// DistanceUnit names the unit [TargetDetails.String] renders Distance in:
	// "pc" for a fixed target placed by its parallax, "a.u." for a
	// solar-system body, "km" for a satellite.
	//
	// It is a rendering choice, not the unit Distance is in. It used to be
	// both — Distance was a bare float64 and this string was the only record
	// of what its number meant, so a caller reading Distance without reading
	// this alongside it got parsecs, au or kilometers depending on the target
	// and no way to tell. Distance now carries its own unit and this only
	// decides which one to print.
	DistanceUnit string

	// Distance is the target's distance: from the observer for a
	// solar-system body or satellite, from the Sun for a fixed target
	// placed by its parallax. Zero when nothing supplied one.
	Distance     unit.Length
	Azimuth      angle.Angle
	RiseAzimuth  angle.Angle
	Altitude     angle.Angle
	MaxElevation angle.Angle
	SetAzimuth   angle.Angle
	Elongation   angle.Angle

	// RA is the astrometric topocentric right ascension in the ICRS frame (J2000).
	// For moving bodies this includes diurnal parallax correction but does NOT
	// include precession-nutation or stellar aberration. Matches J2000 star
	// charts, Stellarium default, and GoTo mount coordinate systems.
	RA angle.Angle

	// Dec is the astrometric topocentric declination in the ICRS frame (J2000).
	// Same reference frame caveats as RA — astrometric, not apparent of-date.
	Dec angle.Angle
}

// String formats the details as a textual summary.
func (d TargetDetails) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", strings.ToUpper(d.Name))

	if d.Description != "" {
		fmt.Fprintf(&b, "%s\n\n", d.Description)
	}

	if d.Source != "" {
		fmt.Fprintf(&b, "%s\n\n", d.Source)
	}

	if d.Magnitude != "" {
		fmt.Fprintf(&b, "Magnitude:\t%s\n", d.Magnitude)
	}

	if d.RadialVelocity != "" {
		fmt.Fprintf(&b, "Radial velocity:\t%s\n", d.RadialVelocity)
	}

	fmt.Fprintf(&b, "RA (ICRS):\t%s\n", d.RA.HMSString(1))
	fmt.Fprintf(&b, "Dec (ICRS):\t%s\n", d.Dec.DMSString(0))
	fmt.Fprintf(&b, "Altitude:\t%s\n", d.Altitude.DMSString(0))
	fmt.Fprintf(&b, "Azimuth:\t%s\n", d.Azimuth.DMSString(0))
	fmt.Fprintf(&b, "Distance:\t%.4f %s\n", d.distanceInItsUnit(), d.DistanceUnit)

	if d.AngularSize != "" {
		fmt.Fprintf(&b, "Angular size(s):\t%s\n", d.AngularSize)
	}

	if d.RiseTime != nil {
		fmt.Fprintf(&b, "Rise time:\t%s\n", d.RiseTime.ToGo().Format("03:04 pm"))
		fmt.Fprintf(&b, "Rise azimuth:\t%s\n", d.RiseAzimuth.DMSString(0))
	}

	if d.TransitTime != nil {
		fmt.Fprintf(&b, "Time of maximum elevation:\t%s\n", d.TransitTime.ToGo().Format("03:04 pm"))
		fmt.Fprintf(&b, "Maximum elevation:\t%.1f°\n", d.MaxElevation.Degrees())
	}

	if d.SetTime != nil {
		fmt.Fprintf(&b, "Set time:\t%s\n", d.SetTime.ToGo().Format("03:04 pm"))
		fmt.Fprintf(&b, "Set azimuth:\t%s\n", d.SetAzimuth.DMSString(0))
	}

	if d.Elongation.Radians() != 0 {
		fmt.Fprintf(&b, "Elongation:\t%.1f°\n", d.Elongation.Degrees())
	}

	if len(d.ExtraProps) > 0 {
		b.WriteString("\n")
		// Output known props first if they exist
		if v, ok := d.ExtraProps["Messier number"]; ok {
			fmt.Fprintf(&b, "Messier number:\t%s\n", v)
		}

		if v, ok := d.ExtraProps["NGC/IC number"]; ok {
			fmt.Fprintf(&b, "NGC/IC number:\t%s\n", v)
		}

		for k, v := range d.ExtraProps {
			if k != "Messier number" && k != "NGC/IC number" {
				fmt.Fprintf(&b, "%s:\t%s\n", k, v)
			}
		}
	}

	return b.String()
}

// distanceInItsUnit renders Distance in whatever [TargetDetails.DistanceUnit]
// names, so the printed number and the printed unit cannot disagree.
//
// An unrecognized DistanceUnit falls back to meters rather than guessing,
// which is the one answer that is never silently wrong by a factor: it is what
// [unit.Length] stores, so the number is the value itself.
func (d TargetDetails) distanceInItsUnit() float64 {
	switch d.DistanceUnit {
	case "pc":
		return d.Distance.Pc()
	case "a.u.":
		return d.Distance.AU()
	case "km":
		return d.Distance.Km()
	default:
		return d.Distance.Meters()
	}
}

// ── computeDetails ──────────────────────────────────────────────────────────

func computeDetails(obs Observable, ctx *coord.Context, over DetailOverrides) (*TargetDetails, error) {
	d := &TargetDetails{
		Name:       obs.Name(),
		ExtraProps: make(map[string]string),
	}
	t := ctx.Time()

	pos, err := obs.Position(t)
	if err != nil {
		return nil, fmt.Errorf("details: position: %w", err)
	}

	d.RA = pos.RA()
	d.Dec = pos.Dec()

	// ── Topocentric position + distance ──
	if mb, ok := obs.(MovingBody); ok {
		fillMovingBody(d, mb, t, ctx)
	} else {
		altaz, err := ctx.ICRSToAltAz(pos)
		if err != nil {
			return nil, fmt.Errorf("details: alt/az: %w", err)
		}

		d.Altitude = altaz.Alt()
		d.Azimuth = altaz.Az()
		d.DistanceUnit = "pc"
	}

	// ── Magnitude via interface dispatch ──
	if mc, ok := obs.(MagnitudeComputer); ok {
		if v, err := mc.ApparentMagnitudeCtx(t, ctx); err == nil {
			d.Magnitude = fmt.Sprintf("%.1f mag", v)
		}
	} else {
		// Static magnitude for non-MagnitudeComputer types.
		fillStaticMagnitude(d, obs)
	}

	// ── Radial velocity via interface dispatch ──
	fillRadialVelocity(d, obs, ctx)

	// ── Type-specific catalog properties ──
	fillTypedProps(d, obs)

	// ── Caller overrides (replace anything above) ──
	over.apply(d)

	// ── Rise/Set/Transit events ──
	fillRiseSetTransit(d, obs, ctx)

	return d, nil
}

// ── Moving-body helpers ─────────────────────────────────────────────────────

// fillMovingBody computes topocentric AltAz, RA/Dec, distance, and elongation
// for a target with an ephemeris provider, all from ctx's site.
func fillMovingBody(d *TargetDetails, mb MovingBody, t time.Time, ctx *coord.Context) {
	in := eph.At(mb.Provider(), t)

	topoVec, err := astrometricFromSite(in, mb, ctx)
	if err != nil {
		return
	}

	topoDist := topoVec.Norm()

	ra, dec := topoVec.ToSpherical()
	d.RA, d.Dec = angle.Rad(ra), angle.Rad(dec)

	// AltAz observed from the site itself (includes refraction).
	altaz, err := observedPlace(mb, t, ctx)
	if err != nil {
		return
	}

	d.Altitude = altaz.Alt()
	d.Azimuth = altaz.Az()
	d.Distance = unit.AU(topoDist)
	d.DistanceUnit = "a.u."

	// A satellite's distance is the same topocentric one, conventionally read
	// in km. This used to replace it with altaz.Dist(), on the claim that it
	// came from "the Reducer pipeline"; GeocentricToObserved set no distance,
	// so every satellite's details read 0 km (#495).
	if _, isSat := mb.(*Satellite); isSat {
		d.DistanceUnit = "km"

		return
	}

	// Apparent/angular diameter — silently skipped for bodies with no
	// known physical radius (asteroids, comets), same best-effort
	// pattern as Elongation below. DetailOverrides.AngularSize, applied
	// after computeDetails' fillMovingBody, still overrides this.
	if diam, err := AngularDiameter(mb, t, ctx); err == nil {
		d.AngularSize = formatAngularSize(diam)
	}

	// Elongation from the Sun, placed the same way as the body.
	sunVec, err := sunFromSite(in, mb, ctx)
	if err == nil {
		sunRA, sunDec := sunVec.ToSpherical()
		sunPos := coord.NewICRS(angle.Rad(sunRA), angle.Rad(sunDec))
		bodyPos := coord.NewICRS(d.RA, d.Dec)
		d.Elongation = coord.Separation(bodyPos, sunPos)
	}
}

// astrometricFromSite is mb's astrometric place from ctx's site, the vector
// TargetDetails' RA, Dec and distance describe: light time to the site, but
// no deflection and no aberration. in is at ctx's time.
//
// A body seenFromTheSite takes it from in with the site as the observer. It
// used to be the geocentric apparent place less the site's position, which
// carried the annual aberration the RA and Dec are documented without: up to
// 21.0″ from the astrometric place for the Moon, the Sun and the planets
// across 2026 (#843). Any other MovingBody still offers only its
// GeocentricVec, and has the site's position taken off that.
func astrometricFromSite(in *eph.Instant, mb MovingBody, ctx *coord.Context) (vector.Vec3, error) {
	if !seenFromTheSite(mb) {
		vec, err := mb.GeocentricVec(in.Time())
		if err != nil {
			return vector.Vec3{}, fmt.Errorf("plan: geocentric vector: %w", err)
		}

		return vec.Sub(ctx.ObsVec()), nil
	}

	st, err := in.Astrometric(mb.EphID(), eph.Site(ctx))
	if err != nil {
		return vector.Vec3{}, fmt.Errorf("plan: %s: astrometric state from the site: %w", mb.Name(), err)
	}

	return st.Pos, nil
}

// sunFromSite is the Sun placed from ctx's site the way astrometricFromSite
// places mb, so an elongation compares two places of one kind. A body
// seenFromTheSite has the Sun's astrometric place from the same Instant; any
// other has its geometric one, less the site's position.
func sunFromSite(in *eph.Instant, mb MovingBody, ctx *coord.Context) (vector.Vec3, error) {
	if !seenFromTheSite(mb) {
		vec, err := geocentric(mb.Provider(), eph.Sun, in.Time())
		if err != nil {
			return vector.Vec3{}, err
		}

		return vec.Sub(ctx.ObsVec()), nil
	}

	st, err := in.Astrometric(eph.Sun, eph.Site(ctx))
	if err != nil {
		return vector.Vec3{}, fmt.Errorf("plan: the Sun's astrometric state from the site: %w", err)
	}

	return st.Pos, nil
}

// ── Static magnitude ────────────────────────────────────────────────────────

// fillStaticMagnitude handles non-MagnitudeComputer types with catalog magnitudes.
func fillStaticMagnitude(d *TargetDetails, obs Observable) {
	if sm, ok := obs.(StaticMagnitude); ok {
		if v, has := sm.StaticMagnitude(); has {
			d.Magnitude = fmt.Sprintf("%.1f mag", v)
		}
	}
}

// ── Radial velocity ─────────────────────────────────────────────────────────

// fillRadialVelocity records the radial velocity an observer at ctx would
// measure right now, for any target that has one.
//
// Silently skipped rather than a hard error when the target has none — the
// same best-effort convention AngularDiameter and Elongation already use in
// fillMovingBody.
//
// It used to handle only MeasuredRadialVelocity, and so only *Star. That
// missed both halves of the sky: a galaxy's catalog RV was being fetched and
// discarded, and a solar-system body — whose radial velocity is the one that
// actually moves, by tens of km/s in a night — had none at all. [RadialVelocity]
// dispatches on what the target is.
func fillRadialVelocity(d *TargetDetails, obs Observable, ctx *coord.Context) {
	rv, err := RadialVelocity(obs, ctx)
	if err != nil {
		return
	}

	// A catalog value is worth showing beside the measurement, because the
	// two differ by up to ~30 km/s and a reader comparing against a
	// published number needs to know which they are looking at. A moving
	// body has no catalog value to show.
	if mrv, ok := obs.(MeasuredRadialVelocity); ok {
		if barycentric, has := mrv.MeasuredRadialVelocity(); has {
			d.RadialVelocity = fmt.Sprintf("%+.2f km/s topocentric (%+.2f km/s barycentric)",
				rv.KmPerSec(), barycentric.KmPerSec())

			return
		}
	}

	d.RadialVelocity = fmt.Sprintf("%+.2f km/s topocentric", rv.KmPerSec())
}

// ── Type-specific property extraction ───────────────────────────────────────

// fillTypedProps extracts type-specific properties into ExtraProps.
func fillTypedProps(d *TargetDetails, obs Observable) {
	switch v := obs.(type) {
	case *Star:
		if v.parallax.Radians() > 0 {
			d.Distance = coord.ParallaxDistance(v.parallax)
		}

		if v.pmRA.Radians() != 0 {
			d.ExtraProps["Proper motion (RA)"] = fmt.Sprintf("%.2f mas/yr", v.pmRA.Arcseconds()*1000.0)
		}

		if v.pmDec.Radians() != 0 {
			d.ExtraProps["Proper motion (Dec)"] = fmt.Sprintf("%.2f mas/yr", v.pmDec.Arcseconds()*1000.0)
		}

		fillAliasProps(d, v.aliases)
	case *DeepSkyObject:
		fillAliasProps(d, v.aliases)
	}
}

// fillAliasProps extracts Messier and NGC/IC identifiers from alias lists.
func fillAliasProps(d *TargetDetails, aliases []string) {
	for _, alias := range aliases {
		if strings.HasPrefix(alias, "M ") || strings.HasPrefix(alias, "M") {
			if len(alias) > 1 && alias[1] >= '0' && alias[1] <= '9' || (len(alias) > 2 && alias[0:2] == "M ") {
				d.ExtraProps["Messier number"] = strings.ReplaceAll(alias, " ", "")
			}
		}

		if strings.HasPrefix(alias, "NGC") || strings.HasPrefix(alias, "IC") {
			d.ExtraProps["NGC/IC number"] = alias
		}
	}
}

// ── Custom property overrides ───────────────────────────────────────────────

// DetailOverrides carries what this package cannot compute — a human
// description, where the numbers came from — and replaces what it can, for a
// caller who has a better answer.
//
// # Why a struct and not the key/value pairs it replaces
//
// GetDetails used to take `props ...string` read two at a time. Three ways to
// get that wrong, all silent:
//
//   - An odd count dropped the last argument, because the loop ran to
//     len(props)-1.
//   - A misspelled key did not fail; it landed in ExtraProps, and the field it
//     meant to override kept its computed value. A key one letter away from
//     "Description" was just as valid, with an entirely different effect.
//   - A key and a value could be swapped with nothing to notice.
//
// The zero value overrides nothing, which is the common case and what a caller
// with nothing to add should pass.
type DetailOverrides struct {
	// Description is prose about the target — what it is, why it is on the
	// list. Nothing computes this.
	Description string

	// Source records where the target's data came from: a catalog name, a
	// kernel, an element set.
	Source string

	// Magnitude, RadialVelocity and AngularSize replace the computed strings
	// when a caller holds a better value than this package can derive. Empty
	// leaves the computed one.
	Magnitude      string
	RadialVelocity string
	AngularSize    string

	// Extra is anything else, verbatim, in TargetDetails.ExtraProps. This is
	// where a genuinely open-ended key belongs — and it being a separate field
	// is what makes a typo in one of the named ones a compile error rather
	// than a silent demotion to here.
	Extra map[string]string
}

// apply writes the non-empty overrides onto d.
func (o DetailOverrides) apply(d *TargetDetails) {
	if o.Description != "" {
		d.Description = o.Description
	}

	if o.Source != "" {
		d.Source = o.Source
	}

	if o.Magnitude != "" {
		d.Magnitude = o.Magnitude
	}

	if o.RadialVelocity != "" {
		d.RadialVelocity = o.RadialVelocity
	}

	if o.AngularSize != "" {
		d.AngularSize = o.AngularSize
	}

	maps.Copy(d.ExtraProps, o.Extra)
}

// ── Rise/Set/Transit events ─────────────────────────────────────────────────

// fillRiseSetTransit fills the rise, transit and set of the target's pass
// at the Context's time: the pass in progress when the target is up then,
// and the next one when it is down. A target that neither rises nor sets
// across the search gets its next culmination alone.
//
// Rise and set are on the horizon riseSetSpec gives SunEvents, MoonEvents
// and VisibilityEvents, so they are those functions' instants. This used to
// search a geometric 0° horizon of its own, which put rise and set 4 to 8
// minutes from every other rise and set in the package, and it reported the
// first rise after twelve hours before the Context's time, which was often
// a pass already over (#704).
func fillRiseSetTransit(d *TargetDetails, obs Observable, ctx *coord.Context) {
	site, err := NewSite("Observer", ctx.Site())
	if err != nil {
		// No valid location to compute rise/set/transit against; leave
		// those fields unset rather than proceed with a broken Observer.
		return
	}

	t := ctx.Time()

	// A day back finds the rise of a pass in progress; two ahead find the
	// next pass's rise and its set.
	events, err := NewEventSolver(unit.Minutes(15), unit.Seconds(1)).
		Find(riseSetSpec(obs, site), t.Add(unit.Hours(-24)), t.Add(unit.Hours(48)))
	if err != nil {
		return
	}

	rise, transit, set := passAt(events, t)

	if rise != nil {
		d.RiseTime, d.RiseAzimuth = &rise.Time, rise.Azimuth
	}

	if transit != nil {
		d.TransitTime, d.MaxElevation = &transit.Time, transit.Altitude
	}

	if set != nil {
		d.SetTime, d.SetAzimuth = &set.Time, set.Azimuth
	}
}

// passAt picks, from time-ordered visibility events, the rise, transit and
// set of the pass at t. The target is up at t when the last rise or set
// before t is a rise; that rise starts the pass. When it is down, the pass
// is the next one, from its rise. With no rise or set before t, the first
// one after says which: a set means a pass in progress whose rise is out of
// reach. With none at all, the target neither rises nor sets, and the pass
// is its next culmination. Any of the three may be nil.
func passAt(events []Event, t time.Time) (rise, transit, set *Event) {
	var last *Event

	for i := range events {
		e := &events[i]
		if e.Time.Before(t) && (e.Kind == EventRise || e.Kind == EventSet) {
			last = e
		}
	}

	from := t
	if last != nil && last.Kind == EventRise {
		rise, from = last, last.Time
	}

	// waiting is a target down at t whose next rise is still ahead: a
	// culmination before that rise belongs to no pass.
	waiting := last != nil && last.Kind == EventSet

	for i := range events {
		e := &events[i]
		if e.Time.Before(from) {
			continue
		}

		switch e.Kind { //nolint:exhaustive // a pass is a rise, a transit and a set
		case EventRise:
			if e == rise {
				continue
			}

			if rise != nil || set != nil {
				return rise, transit, set
			}

			// A culmination before the rise was below the horizon, on a
			// day the target did not rise; it is not this pass's.
			rise, transit, waiting = e, nil, false
		case EventTransit:
			if transit == nil && !waiting {
				transit = e
			}
		case EventSet:
			if !waiting {
				return rise, transit, e
			}
		}
	}

	return rise, transit, set
}

// clamp restricts v to the range [lo, hi].
func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}

	if v > hi {
		return hi
	}

	return v
}

// RadialVelocity returns the radial velocity, in km/s, an observer at ctx
// measures for obs right now — positive when the target is receding.
//
// # One function, two entirely different quantities
//
// A fixed target's radial velocity is a measurement somebody made and wrote
// in a catalog: it is a property of the object, constant on any timescale an
// observer cares about, and the only work here is referring it from the
// barycenter to the observer. That is [MeasuredRadialVelocity], carried by
// *Star and *DeepSkyObject.
//
// A solar-system body has no such number, and could not have one. Its radial
// velocity is a consequence of where it and the observer happen to be, and it
// swings by tens of km/s over a single night. It is computed from the
// ephemeris state the provider already returns — see
// [coord.Context.TopocentricRadialVelocity].
//
// Both are radial velocities and a caller wanting "how fast is this
// separating from me" should not have to know which kind it has, which is why
// this dispatches rather than exposing two functions.
//
// Returns ErrNoRadialVelocity for a target that is neither: a fixed target
// whose catalog carried no RV, or one of the target kinds with no position
// model at all.
func RadialVelocity(obs Observable, ctx *coord.Context) (unit.Velocity, error) {
	// Moving body first. A body with an ephemeris always has a radial
	// velocity, and a catalog value for one would be meaningless — the
	// ordering matters if a type ever implements both.
	if mb, ok := obs.(MovingBody); ok {
		state, err := eph.At(mb.Provider(), ctx.Time()).Geometric(mb.EphID(), eph.Earth)
		if err != nil {
			return 0, fmt.Errorf("radial velocity for %s: %w", mb.Name(), err)
		}

		return ctx.TopocentricRadialVelocity(state.Pos, state.Vel), nil
	}

	if mrv, ok := obs.(MeasuredRadialVelocity); ok {
		if rvBarycentric, has := mrv.MeasuredRadialVelocity(); has {
			pos, err := obs.Position(ctx.Time())
			if err != nil {
				return 0, fmt.Errorf("radial velocity for %s: %w", obs.Name(), err)
			}

			rv, err := ctx.ObservedRadialVelocity(pos, rvBarycentric)
			if err != nil {
				return 0, fmt.Errorf("radial velocity for %s: %w", obs.Name(), err)
			}

			return rv, nil
		}
	}

	return 0, fmt.Errorf("%w: %s", ErrNoRadialVelocity, obs.Name())
}
