// Package main demonstrates coordinate system conversions: ICRS to Galactic
// and back, ICRS to the ecliptic of date, ICRS to a site's alt/az, and a
// batch of stars converted through one cached Context.
package main

import (
	"fmt"
	"log"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/examples/internal/show"
	"github.com/TuSKan/astrogo/time"
)

func main() {
	// This example runs offline, so it uses no Earth-orientation data
	// (UT1 = UTC, no polar motion). That can move an alt/az position by up
	// to 13.5 arcsec, 0.004°. For full accuracy, blank-import remote/eop and
	// consent to the IERS bulletin, as 09_geometry_events does.
	time.RegisterModel(time.ZeroModel{})

	// 1. Start with an ICRS coordinate: Sgr A*, the Galactic center
	// (J2000 position from SIMBAD).
	sgrA := coord.NewICRS(angle.Deg(266.416817), angle.Deg(-29.007825))

	// 2. ICRS -> Galactic, and back. The Galactic center sits near l = 0, b = 0.
	galactic := coord.ICRSToGalactic(sgrA)
	back := coord.GalacticToICRS(galactic)

	// 3. ICRS -> the ecliptic of date, which moves with the equinox.
	tz, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		log.Fatalf("tz: %v", err)
	}

	t := time.Date(2026, 7, 15, 22, 0, 0, 0, tz)
	ecliptic := coord.ICRSToEcliptic(sgrA, t)

	// 4. ICRS -> AltAz, which needs a site and an instant. One Context holds
	// everything that depends on them, so build it once and reuse it.
	loc, err := coord.NewEarthLocation(-22.528478, -46.473002, 835.05) // Quinta Calixto
	if err != nil {
		log.Fatalf("loc: %v", err)
	}

	ctx := coord.NewContext(t, loc, atmosphere.StandardRefraction())

	altaz, err := ctx.ICRSToAltAz(sgrA)
	if err != nil {
		log.Fatalf("altaz: %v", err)
	}

	show.Title("Coordinate conversions for Sgr A*",
		"AltAz from Quinta Calixto at "+t.Format("2006-01-02 15:04 MST")+" ("+t.In(time.LocationUTC).Format("15:04 MST")+")")

	deg := func(a angle.Angle) string { return fmt.Sprintf("%.5f°", a.Degrees()) }

	frames := show.NewTable(show.Left("Frame"), show.Left(""), show.Right("Longitude"), show.Left(""), show.Right("Latitude"))
	frames.Row("ICRS (J2000)", "RA", deg(sgrA.RA()), "Dec", deg(sgrA.Dec()))
	frames.Row("Galactic", "l", deg(galactic.L()), "b", deg(galactic.B()))
	frames.Row("  and back to ICRS", "RA", deg(back.RA()), "Dec", deg(back.Dec()))
	frames.Row("Ecliptic of date", "λ", deg(ecliptic.Lon()), "β", deg(ecliptic.Lat()))
	frames.Row("AltAz", "Az", deg(altaz.Az()), "Alt", deg(altaz.Alt()))

	fmt.Println()
	frames.Print()
	fmt.Println()
	show.Note(fmt.Sprintf("The round trip through Galactic lands %.1g arcsec from where it began.",
		coord.Separation(sgrA, back).Arcseconds()))

	// 5. A batch: many stars through the same Context. The expensive part,
	// the Earth's orientation at t, was computed once when ctx was built.
	stars := []struct {
		name    string
		ra, dec float64 // degrees, J2000, from SIMBAD
	}{
		{"Antares", 247.351915, -26.432003},
		{"Vega", 279.234735, 38.783689},
		{"Altair", 297.695827, 8.868321},
		{"Arcturus", 213.915300, 19.182409},
		{"Alpha Centauri", 219.902058, -60.833993},
	}

	in := make([]coord.ICRS, len(stars))
	for i, s := range stars {
		in[i] = coord.NewICRS(angle.Deg(s.ra), angle.Deg(s.dec))
	}

	out := make([]coord.AltAz, len(in))
	ctx.ICRSBatchToAltAz(in, out)

	show.Section("Batch: five stars, one Context")

	batch := show.NewTable(show.Left("Star"), show.Right("Altitude"), show.Right("Azimuth"))
	for i, s := range stars {
		batch.Row(s.name, fmt.Sprintf("%.2f°", out[i].Alt().Degrees()), fmt.Sprintf("%.2f°", out[i].Az().Degrees()))
	}

	batch.Print()
}
