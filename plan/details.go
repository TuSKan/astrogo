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
	Name         string
	Description  string
	Source       string
	Magnitude    string
	RA           angle.Angle
	Dec          angle.Angle
	Altitude     angle.Angle
	Azimuth      angle.Angle
	Distance     float64 // in AU or pc depending on object
	DistanceUnit string
	AngularSize  string
	RiseTime     *time.Time
	RiseAzimuth  angle.Angle
	TransitTime  *time.Time
	MaxElevation angle.Angle
	SetTime      *time.Time
	SetAzimuth   angle.Angle
	Elongation   angle.Angle

	ExtraProps map[string]string
}

// String formats the details as a textual summary.
func (d TargetDetails) String() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s\n", strings.ToUpper(d.Name)))
	if d.Description != "" {
		b.WriteString(fmt.Sprintf("%s\n\n", d.Description))
	}
	if d.Source != "" {
		b.WriteString(fmt.Sprintf("%s\n\n", d.Source))
	}

	if d.Magnitude != "" {
		b.WriteString(fmt.Sprintf("Magnitude:\t%s\n", d.Magnitude))
	}
	b.WriteString(fmt.Sprintf("RA:\t%s\n", d.RA.HMSString(1)))
	b.WriteString(fmt.Sprintf("Dec:\t%s\n", d.Dec.DMSString(0)))
	b.WriteString(fmt.Sprintf("Altitude:\t%s\n", d.Altitude.DMSString(0)))
	b.WriteString(fmt.Sprintf("Azimuth:\t%s\n", d.Azimuth.DMSString(0)))
	b.WriteString(fmt.Sprintf("Distance:\t%.4f %s\n", d.Distance, d.DistanceUnit))
	if d.AngularSize != "" {
		b.WriteString(fmt.Sprintf("Angular size(s):\t%s\n", d.AngularSize))
	}

	if d.RiseTime != nil {
		b.WriteString(fmt.Sprintf("Rise time:\t%s\n", d.RiseTime.ToGo().Format("03:04 pm")))
		b.WriteString(fmt.Sprintf("Rise azimuth:\t%s\n", d.RiseAzimuth.DMSString(0)))
	}
	if d.TransitTime != nil {
		b.WriteString(fmt.Sprintf("Time of maximum elevation:\t%s\n", d.TransitTime.ToGo().Format("03:04 pm")))
		b.WriteString(fmt.Sprintf("Maximum elevation:\t%.1f°\n", d.MaxElevation.Degrees()))
	}
	if d.SetTime != nil {
		b.WriteString(fmt.Sprintf("Set time:\t%s\n", d.SetTime.ToGo().Format("03:04 pm")))
		b.WriteString(fmt.Sprintf("Set azimuth:\t%s\n", d.SetAzimuth.DMSString(0)))
	}
	if d.Elongation.Radians() != 0 {
		b.WriteString(fmt.Sprintf("Elongation:\t%.1f°\n", d.Elongation.Degrees()))
	}

	if len(d.ExtraProps) > 0 {
		b.WriteString("\n")
		// Output known props first if they exist
		if v, ok := d.ExtraProps["Messier number"]; ok {
			b.WriteString(fmt.Sprintf("Messier number:\t%s\n", v))
		}
		if v, ok := d.ExtraProps["NGC/IC number"]; ok {
			b.WriteString(fmt.Sprintf("NGC/IC number:\t%s\n", v))
		}
		for k, v := range d.ExtraProps {
			if k != "Messier number" && k != "NGC/IC number" {
				b.WriteString(fmt.Sprintf("%s:\t%s\n", k, v))
			}
		}
	}
	return b.String()
}

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

	// Compute Topocentric AltAz and Distance
	var altaz coord.AltAz
	if tTarget, ok := obs.(Target); ok && tTarget.Provider != nil {
		vec, err := tTarget.GeocentricVec(t)
		if err != nil {
			return nil, err
		}
		altaz = ctx.GeocentricToObserved(vec)
		d.Distance = vec.Norm() // AU for planets, or maybe km for satellites
		d.DistanceUnit = "a.u."

		// If it's a satellite check
		if tTarget.Catalog.Kind == "Satellite" {
			d.Distance = altaz.Dist() // km
			d.DistanceUnit = "km"
		} else {
			// Elongation for planets
			sunVec, err := eph.Position(tTarget.Provider, eph.Sun, t)
			if err == nil {
				sunPos := coord.NewICRS(angle.Rad(math.Atan2(sunVec.Y, sunVec.X)), angle.Rad(math.Asin(sunVec.Z/sunVec.Norm())))
				sep := coord.Separation(pos, sunPos)
				d.Elongation = sep
			}
		}

	} else {
		altaz, _ = ctx.ICRSToAltAz(pos)
		d.DistanceUnit = "pc"
		d.Distance = 0
	}

	d.Altitude = altaz.Alt()
	d.Azimuth = altaz.Az()

	// Fill catalog properties
	if tTarget, ok := obs.(Target); ok {
		if tTarget.Catalog.Parallax.Radians() > 0 {
			d.Distance = 1.0 / tTarget.Catalog.Parallax.Arcseconds() // pc
		}
		if tTarget.Catalog.PmRA.Radians() != 0 {
			d.ExtraProps["Proper motion (RA)"] = fmt.Sprintf("%.2f mas/yr", tTarget.Catalog.PmRA.Arcseconds()*1000.0)
		}
		if tTarget.Catalog.PmDec.Radians() != 0 {
			d.ExtraProps["Proper motion (Dec)"] = fmt.Sprintf("%.2f mas/yr", tTarget.Catalog.PmDec.Arcseconds()*1000.0)
		}
		for _, alias := range tTarget.Catalog.Aliases {
			if strings.HasPrefix(alias, "M ") || strings.HasPrefix(alias, "M") {
				// Avoid "Mars" or other M words, check if next char is digit
				if len(alias) > 1 && alias[1] >= '0' && alias[1] <= '9' || (len(alias) > 2 && alias[0:2] == "M ") {
					d.ExtraProps["Messier number"] = strings.Replace(alias, " ", "", -1)
				}
			}
			if strings.HasPrefix(alias, "NGC") || strings.HasPrefix(alias, "IC") {
				d.ExtraProps["NGC/IC number"] = alias
			}
		}
	}

	// Process custom flexible props
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

	site, _ := NewSite("Observer", ctx.Site(), angle.Deg(0), nil)

	// Start looking for events from slightly before now to capture today's rise/set
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
	if err == nil {
		for _, ev := range events {
			if ev.Kind == EventRise && ev.Time.After(start) && d.RiseTime == nil {
				tt := ev.Time
				d.RiseTime = &tt
				d.RiseAzimuth = ev.Azimuth
			} else if ev.Kind == EventSet && ev.Time.After(start) && d.SetTime == nil {
				tt := ev.Time
				d.SetTime = &tt
				d.SetAzimuth = ev.Azimuth
			} else if ev.Kind == EventTransit && ev.Time.After(start) && d.TransitTime == nil {
				tt := ev.Time
				d.TransitTime = &tt
				d.MaxElevation = ev.Altitude
			}
		}
	}

	return d, nil
}
