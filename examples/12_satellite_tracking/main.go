// Package main demonstrates real-time satellite tracking.
package main

import (
	"context"
	"fmt"
	"log"
	"strconv"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/catalog"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/ephemeris/satellite"
	"github.com/TuSKan/astrogo/examples/internal/show"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/remote"
	_ "github.com/TuSKan/astrogo/remote/eop"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// This example demonstrates astrogo's unified Provider API for satellite tracking:
//
//  1. Resolve satellite from catalog (NORAD → CelestTrak)
//  2. SGP4 orbit propagation via Provider.Locate — same as JPL planets
//  3. Height above the WGS84 ellipsoid
//  4. Topocentric look angle via coord.Context (same API for ISS (Zarya) and Mars)
//  5. Pass prediction
//
// CelestTrak API: https://celestrak.org/NORAD/elements/gp.php

func main() {
	// Earth orientation, for where the site is on the turning Earth: the
	// IERS bulletin, which the remote/eop import above fetches.
	remote.EnableDownloads(0, remote.IERSFinals2000A)

	// ═══════════════════════════════════════════════════════════════════════
	// 1. Resolve ISS (Zarya) from NORAD Catalog
	// ═══════════════════════════════════════════════════════════════════════
	resolver := catalog.NewResolver(catalog.NORAD)

	target, err := resolver.Resolve(context.Background(), "ISS (Zarya)")
	if err != nil {
		log.Fatalf("Failed to resolve ISS (Zarya): %v", err)
	}

	// A TLE describes an orbit well for a few days either side of its
	// epoch, and CelesTrak serves the current one, so this example works
	// from the present moment rather than a fixed date.
	now := time.NowUTC()

	show.Title("Tracking the ISS", "From CelesTrak's current element set, through SGP4")

	show.Section("The element set")
	show.Fields(
		show.KV("Name", "%s", target.Name),
		show.KV("NORAD ID", "%s", target.ID),
		show.KV("International designator", "%s", target.Designation),
		show.KV("Epoch", "%s, %.1f days ago", target.Epoch.Format("2006-01-02 15:04:05 UTC"), now.Sub(target.Epoch).Days()),
	)

	// ═══════════════════════════════════════════════════════════════════════
	// 2. Create Provider from Target TLE
	// ═══════════════════════════════════════════════════════════════════════
	prov, err := eph.NewProvider(context.Background(), eph.Satellites, target.Name,
		eph.WithTLE(target.TLELine1, target.TLELine2))
	if err != nil {
		log.Fatalf("Failed to create satellite provider: %v", err)
	}
	defer func() {
		err := prov.Close()
		if err != nil {
			log.Printf("failed to close provider: %v", err)
		}
	}()

	// A Satellites provider is a *satellite.Satellite. Its ID is its NAIF
	// code, -100000 minus the NORAD catalog number, and the one id it
	// answers for.
	sat, ok := prov.(*satellite.Satellite)
	if !ok {
		log.Fatalf("a Satellites provider should be a *satellite.Satellite, got %T", prov)
	}

	// The same composition as for JPL planets: the satellite about the Earth,
	// at one instant.
	state, err := eph.At(prov, now).Geometric(sat.ID(), eph.Earth)
	if err != nil {
		log.Fatalf("Geometric failed: %v", err)
	}

	// Derive RA/Dec from GCRS position.
	icrs, err := eph.ToICRS(state.Pos)
	if err != nil {
		log.Fatalf("icrs: %v", err)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// 3. Height Above the WGS84 Ellipsoid
	// ═══════════════════════════════════════════════════════════════════════
	// The *satellite.Satellite knows the ellipsoid; a generic Provider only
	// knows a geocentric distance.
	alt, err := sat.Altitude(now)
	if err != nil {
		log.Fatalf("Altitude failed: %v", err)
	}

	show.Section("Where it is now, through eph.At(...).Geometric")
	show.Fields(
		show.KV("At", "%s", now.Format("2006-01-02 15:04:05 UTC")),
		show.KV("RA", "%s", icrs.RA().HMSString(1)),
		show.KV("Dec", "%s", icrs.Dec().DMSString(0)),
		show.KV("From Earth's center", "%.1f km", state.DistanceKm()),
		show.KV("Above the WGS84 ellipsoid", "%.1f km", alt.Km()),
	)

	// ═══════════════════════════════════════════════════════════════════════
	// 4. Look Angle via coord.Context (same API for ISS (Zarya) and Mars)
	// ═══════════════════════════════════════════════════════════════════════
	// Observer: Quinta Calixto (-22.528478°, -46.473002°, 835.05m)
	observer, err := coord.NewEarthLocation(-22.528478, -46.473002, 835.05)
	if err != nil {
		log.Fatalf("NewEarthLocation failed: %v", err)
	}

	// Build the observation context (caches SOFA matrices for this time+site).
	ctx := coord.NewContext(now, observer, atmosphere.Refraction{})

	altaz, err := plan.LookAngle(prov, sat.ID(), ctx)
	if err != nil {
		log.Fatalf("LookAngle failed: %v", err)
	}

	status := "below the horizon"
	if altaz.Alt().Degrees() > 0 {
		status = "above the horizon"
	}

	show.Section("From Quinta Calixto, Brazil, through coord.Context")
	show.Fields(
		show.KV("Azimuth", "%.2f°", altaz.Az().Degrees()),
		show.KV("Elevation", "%+.2f°, %s", altaz.Alt().Degrees(), status),
		show.KV("Range", "%.1f km", altaz.Dist().Km()),
	)

	// ═══════════════════════════════════════════════════════════════════════
	// 5. Pass Prediction
	// ═══════════════════════════════════════════════════════════════════════
	start := now
	end := now.Add(unit.Days(1.0))
	minEl := angle.Deg(20.0)

	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		log.Fatalf("loc: %v", err)
	}

	passes, err := plan.SatellitePasses(prov, sat.ID(), target.Name, start, end, observer, minEl)
	if err != nil {
		log.Fatalf("SatellitePasses failed: %v", err)
	}

	show.Section(fmt.Sprintf("Passes above %.0f° in the next 24 hours", minEl.Degrees()))

	if len(passes) == 0 {
		show.Note("None.")

		return
	}

	tab := show.NewTable(show.Right("Pass"), show.Left("Event"), show.Left("Time"),
		show.Right("Azimuth"), show.Right("Elevation"), show.Right("Range"))

	for i, pass := range passes {
		// A pass here runs from where the satellite climbs past the minimum
		// elevation to where it drops below it, not from rise to set. One in
		// progress when the 24 hours begin or end is cut off there: its
		// first or last event is the window's edge.
		riseLabel := fmt.Sprintf("above %.0f°", minEl.Degrees())
		setLabel := fmt.Sprintf("below %.0f°", minEl.Degrees())

		if pass.RiseCutOff {
			riseLabel = "window start"
		}

		if pass.SetCutOff {
			setLabel = "window end"
		}

		n := strconv.Itoa(i + 1)
		tab.Row(passRow(n, riseLabel, pass.Rise, loc)...)
		tab.Row(passRow("", "highest", pass.Culmination, loc)...)
		tab.Row(passRow("", setLabel, pass.Set, loc)...)
		tab.Row("", "lasting "+fmtDuration(pass.Duration), "", "", "", "")
	}

	tab.Print()
}

// ── Formatting helpers ────────────────────────────────────────────────────────

func passRow(pass, label string, ev plan.PassEvent, loc *time.Location) []string {
	return []string{
		pass, label,
		ev.Time.ToGo().In(loc).Format("2006-01-02 15:04:05 MST"),
		fmt.Sprintf("%.1f°", ev.Azimuth.Degrees()),
		fmt.Sprintf("%.1f°", ev.Elevation.Degrees()),
		fmt.Sprintf("%.0f km", ev.Range.Km()),
	}
}

func fmtDuration(d time.Duration) string {
	total := int(d.Seconds())
	mins := total / 60
	sec := total % 60

	return fmt.Sprintf("%dm %02ds", mins, sec)
}
