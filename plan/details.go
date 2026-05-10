package plan

import (
	"fmt"
	"math"
	"strings"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
)

// TargetDetails holds descriptive and ephemeris properties of an Observable.
type TargetDetails struct {
	ExtraProps   map[string]string
	SetTime      *time.Time
	TransitTime  *time.Time
	RiseTime     *time.Time
	AngularSize  string
	Description  string
	Source       string
	Magnitude    string
	Name         string
	DistanceUnit string
	Distance     float64
	Azimuth      angle.Angle
	RiseAzimuth  angle.Angle
	Altitude     angle.Angle
	MaxElevation angle.Angle
	Dec          angle.Angle
	SetAzimuth   angle.Angle
	Elongation   angle.Angle
	RA           angle.Angle
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

	fmt.Fprintf(&b, "RA (ICRS):\t%s\n", d.RA.HMSString(1))
	fmt.Fprintf(&b, "Dec (ICRS):\t%s\n", d.Dec.DMSString(0))
	fmt.Fprintf(&b, "Altitude:\t%s\n", d.Altitude.DMSString(0))
	fmt.Fprintf(&b, "Azimuth:\t%s\n", d.Azimuth.DMSString(0))
	fmt.Fprintf(&b, "Distance:\t%.4f %s\n", d.Distance, d.DistanceUnit)

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

// ── computeDetails ──────────────────────────────────────────────────────────

func computeDetails(obs Observable, ctx *coord.Context, props ...string) (*TargetDetails, error) {
	d := &TargetDetails{
		Name:       obs.Name(),
		ExtraProps: make(map[string]string),
	}
	t := ctx.Time()

	pos, err := obs.Position(t)
	if err != nil {
		return nil, err
	}

	d.RA = pos.RA()
	d.Dec = pos.Dec()

	// ── Topocentric position + distance ──
	if mb, ok := obs.(MovingBody); ok {
		fillMovingBody(d, mb, t, ctx)
	} else {
		altaz, _ := ctx.ICRSToAltAz(pos)
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

	// ── Type-specific catalog properties ──
	fillTypedProps(d, obs)

	// ── Custom props (override anything above) ──
	applyProps(d, props)

	// ── Rise/Set/Transit events ──
	fillRiseSetTransit(d, obs, ctx)

	return d, nil
}

// ── Moving-body helpers ─────────────────────────────────────────────────────

// fillMovingBody computes topocentric AltAz, RA/Dec, distance, and elongation
// for a target with an ephemeris provider. The observer's ICRS position is
// subtracted to correct for diurnal parallax (~1° for the Moon, ~23″ for Mars).
func fillMovingBody(d *TargetDetails, mb MovingBody, t time.Time, ctx *coord.Context) {
	vec, err := mb.GeocentricVec(t)
	if err != nil {
		return
	}

	// Topocentric vector: geocentric body position minus observer ICRS position.
	topoVec := vec.Sub(ctx.ObsVec())
	topoDist := topoVec.Norm()

	// Topocentric ICRS RA/Dec (astrometric, corrected for diurnal parallax
	// but not for precession-nutation or aberration — matches J2000 star
	// charts, Stellarium default, and GoTo mount coordinate systems).
	d.RA = angle.Rad(math.Atan2(topoVec.Y, topoVec.X)).Wrap360()
	d.Dec = angle.Rad(math.Asin(topoVec.Z / topoDist))

	// AltAz via the full Context pipeline (includes refraction).
	altaz := ctx.GeocentricToObserved(vec)
	d.Altitude = altaz.Alt()
	d.Azimuth = altaz.Az()
	d.Distance = topoDist
	d.DistanceUnit = "a.u."

	// Satellite distances are in km from the Reducer pipeline.
	if _, isSat := mb.(*Satellite); isSat {
		d.Distance = altaz.Dist()
		d.DistanceUnit = "km"

		return
	}

	// Elongation from the Sun (topocentric).
	sunVec, err := eph.Position(mb.Provider(), eph.Sun, t)
	if err == nil {
		sunTopo := sunVec.Sub(ctx.ObsVec())
		sunPos := coord.NewICRS(
			angle.Rad(math.Atan2(sunTopo.Y, sunTopo.X)),
			angle.Rad(math.Asin(sunTopo.Z/sunTopo.Norm())),
		)
		bodyPos := coord.NewICRS(d.RA, d.Dec)
		d.Elongation = coord.Separation(bodyPos, sunPos)
	}
}

// ── Static magnitude ────────────────────────────────────────────────────────

// fillStaticMagnitude handles non-MagnitudeComputer types with catalog magnitudes.
func fillStaticMagnitude(d *TargetDetails, obs Observable) {
	switch v := obs.(type) {
	case *Star:
		if v.hasVMag {
			d.Magnitude = fmt.Sprintf("%.1f mag", v.vMag)
		}
	case *DeepSkyObject:
		if v.hasVMag {
			d.Magnitude = fmt.Sprintf("%.1f mag", v.vMag)
		}
	}
}

// ── Type-specific property extraction ───────────────────────────────────────

// fillTypedProps extracts type-specific properties into ExtraProps.
func fillTypedProps(d *TargetDetails, obs Observable) {
	switch v := obs.(type) {
	case *Star:
		if v.parallax.Radians() > 0 {
			d.Distance = 1.0 / v.parallax.Arcseconds()
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

// applyProps processes key/value pairs that override auto-computed fields.
func applyProps(d *TargetDetails, props []string) {
	for i := 0; i < len(props)-1; i += 2 {
		key := props[i]
		val := props[i+1]

		switch key {
		case "Description":
			d.Description = val
		case "Source":
			d.Source = val
		case "Magnitude":
			d.Magnitude = val
		case "AngularSize":
			d.AngularSize = val
		default:
			d.ExtraProps[key] = val
		}
	}
}

// ── Rise/Set/Transit events ─────────────────────────────────────────────────

// fillRiseSetTransit finds the next rise, set, and transit events within
// ±12/+24 hours of the context time.
func fillRiseSetTransit(d *TargetDetails, obs Observable, ctx *coord.Context) {
	site, _ := NewSite("Observer", ctx.Site(), angle.Deg(0), nil)
	t := ctx.Time()

	start := t.Add(-12 * time.Hour)
	end := t.Add(24 * time.Hour)

	solver := NewEventSolver(15*time.Minute, 1*time.Second)

	spec := EventSpec{
		Family:    EventFamilyVisibility,
		Kind:      EventAnyVisibility,
		Target:    obs,
		Observer:  site,
		Threshold: angle.Deg(0),
	}

	events, err := solver.Find(spec, start, end)
	if err != nil {
		return
	}

	for _, ev := range events {
		if !ev.Time.After(start) {
			continue
		}

		switch ev.Kind {
		case EventRise:
			if d.RiseTime == nil {
				tt := ev.Time
				d.RiseTime = &tt
				d.RiseAzimuth = ev.Azimuth
			}
		case EventSet:
			if d.SetTime == nil {
				tt := ev.Time
				d.SetTime = &tt
				d.SetAzimuth = ev.Azimuth
			}
		case EventTransit:
			if d.TransitTime == nil {
				tt := ev.Time
				d.TransitTime = &tt
				d.MaxElevation = ev.Altitude
			}
		}
	}
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
