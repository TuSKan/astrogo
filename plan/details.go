package plan

import (
	"fmt"
	"math"
	"strings"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	mag "github.com/TuSKan/astrogo/magnitude"
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
	tgt, isTarget := obs.(Target)
	if isTarget && tgt.Provider != nil {
		fillMovingBody(d, tgt, t, ctx)
	} else {
		altaz, _ := ctx.ICRSToAltAz(pos)
		d.Altitude = altaz.Alt()
		d.Azimuth = altaz.Az()
		d.DistanceUnit = "pc"
	}

	// ── Catalog properties + magnitude ──
	if isTarget {
		fillCatalogProps(d, tgt)
		if m := tgt.computeMagnitude(t); m != "" {
			d.Magnitude = m
		}
	}

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
func fillMovingBody(d *TargetDetails, tgt Target, t time.Time, ctx *coord.Context) {
	vec, err := tgt.GeocentricVec(t)
	if err != nil {
		return
	}

	// Topocentric vector: geocentric body position minus observer ICRS position.
	topoVec := vec.Sub(ctx.ObsVec())
	topoDist := topoVec.Norm()

	// Topocentric RA/Dec (corrected for diurnal parallax).
	d.RA = angle.Rad(math.Atan2(topoVec.Y, topoVec.X)).Wrap360()
	d.Dec = angle.Rad(math.Asin(topoVec.Z / topoDist))

	// AltAz via the full Context pipeline (includes refraction).
	altaz := ctx.GeocentricToObserved(vec)
	d.Altitude = altaz.Alt()
	d.Azimuth = altaz.Az()
	d.Distance = topoDist
	d.DistanceUnit = "a.u."

	if tgt.Catalog.Kind == "Satellite" {
		d.Distance = altaz.Dist()
		d.DistanceUnit = "km"
		return
	}

	// Elongation from the Sun (topocentric).
	sunVec, err := eph.Position(tgt.Provider, eph.Sun, t)
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

// ── Magnitude computation ───────────────────────────────────────────────────

// computeMagnitude returns the formatted apparent magnitude string using the
// highest-priority model available for the target.
//
// Priority: Planet > Comet (M1) > Asteroid (sHG1G2 > HG1G2 > HG) > Catalog VMag.
func (tgt Target) computeMagnitude(t time.Time) string {
	cat := tgt.Catalog

	// Planet / Moon / Sun — ephemeris-based Mallama & Hilton (2018).
	if tgt.Provider != nil && cat.Kind != "Satellite" {
		if id, ok := tgt.ephID(); ok {
			if v, err := mag.PlanetApparent(tgt.Provider, id, t); err == nil {
				return fmt.Sprintf("%.1f mag", v)
			}
		}
	}

	// Comet — M1/k1 total magnitude.
	if cat.HasM1 {
		return tgt.cometMagnitude(t)
	}

	// Asteroid — sHG1G2 / HG1G2 / HG phase-curve.
	if cat.HasH {
		return tgt.asteroidMagnitude(t)
	}

	// Star / DSO — catalog V-band magnitude.
	if cat.HasVMag {
		return fmt.Sprintf("%.1f mag", cat.VMag)
	}

	return ""
}

// cometMagnitude computes the apparent magnitude for a comet using M1/k1.
func (tgt Target) cometMagnitude(t time.Time) string {
	r, delta, _, _, ok := tgt.helioGeometry(t)
	if ok {
		return fmt.Sprintf("%.1f mag", mag.CometApparent(tgt.Catalog.M1, tgt.Catalog.K1, r, delta))
	}
	// No ephemeris — return raw parameters.
	return fmt.Sprintf("M1=%.1f, k1=%.1f", tgt.Catalog.M1, tgt.Catalog.K1)
}

// asteroidMagnitude computes the apparent magnitude for an asteroid using the
// best available model: sHG1G2 (Carry 2024) → HG1G2 → HG.
func (tgt Target) asteroidMagnitude(t time.Time) string {
	cat := tgt.Catalog
	r, delta, alpha, st, ok := tgt.helioGeometry(t)
	if !ok {
		G := cat.G
		if G == 0 {
			G = 0.15
		}
		return fmt.Sprintf("H=%.1f, G=%.2f", cat.H, G)
	}

	switch {
	case cat.HasG1G2 && cat.HasSpin && cat.HasOblateness:
		// Full sHG1G2 (Carry et al. 2024) with spin correction.
		ra := angle.Rad(math.Atan2(st.Pos.Y, st.Pos.X))
		dec := angle.Rad(math.Asin(st.Pos.Z / delta))
		cosL := mag.CosAspectAngle(ra, dec, angle.Deg(cat.SpinRA), angle.Deg(cat.SpinDec))
		v := mag.AsteroidSHG1G2(cat.H, cat.G1, cat.G2, r, delta, alpha, cat.Oblateness, cosL)
		return fmt.Sprintf("%.1f mag", v)

	case cat.HasG1G2:
		// HG1G2 without spin correction.
		v := mag.AsteroidHG1G2(cat.H, cat.G1, cat.G2, r, delta, alpha)
		return fmt.Sprintf("%.1f mag", v)

	default:
		// Classic HG model.
		G := cat.G
		if G == 0 {
			G = 0.15
		}
		v := mag.AsteroidHG(cat.H, G, r, delta, alpha)
		return fmt.Sprintf("%.1f mag", v)
	}
}

// helioGeometry computes heliocentric distance r, geocentric distance Δ,
// and phase angle α for a small body at time t.
func (tgt Target) helioGeometry(t time.Time) (r, delta float64, alpha angle.Angle, st eph.State, ok bool) {
	id, valid := tgt.ephID()
	if !valid || tgt.Provider == nil {
		return
	}
	var err error
	st, err = tgt.Provider.State(id, t)
	if err != nil {
		return
	}
	sunSt, err := tgt.Provider.State(eph.Sun, t)
	if err != nil {
		return
	}

	delta = st.Distance()
	hx := st.Pos.X - sunSt.Pos.X
	hy := st.Pos.Y - sunSt.Pos.Y
	hz := st.Pos.Z - sunSt.Pos.Z
	r = math.Sqrt(hx*hx + hy*hy + hz*hz)
	R := sunSt.Distance()

	cosA := (r*r + delta*delta - R*R) / (2 * r * delta)
	cosA = clamp(cosA, -1, 1)
	alpha = angle.Rad(math.Acos(cosA))
	ok = true
	return
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

// ── Catalog property extraction ─────────────────────────────────────────────

// fillCatalogProps populates parallax-derived distance, proper motion,
// and catalog alias identifiers (Messier, NGC/IC).
func fillCatalogProps(d *TargetDetails, tgt Target) {
	cat := tgt.Catalog
	if cat.Parallax.Radians() > 0 {
		d.Distance = 1.0 / cat.Parallax.Arcseconds() // pc
	}
	if cat.PmRA.Radians() != 0 {
		d.ExtraProps["Proper motion (RA)"] = fmt.Sprintf("%.2f mas/yr", cat.PmRA.Arcseconds()*1000.0)
	}
	if cat.PmDec.Radians() != 0 {
		d.ExtraProps["Proper motion (Dec)"] = fmt.Sprintf("%.2f mas/yr", cat.PmDec.Arcseconds()*1000.0)
	}
	for _, alias := range cat.Aliases {
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
