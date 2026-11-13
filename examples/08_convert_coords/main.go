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
	"github.com/TuSKan/astrogo/time"
)

func main() {
	// This example runs offline, so it uses no Earth-orientation data
	// (UT1 = UTC, no polar motion). That can move an alt/az position by up
	// to 13.5 arcsec, 0.004°. For full accuracy, blank-import remote/eop and
	// consent to the IERS bulletin, as 05_resolve_name does.
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

	fmt.Println("Coordinate conversions for Sgr A*")
	fmt.Println("=================================")
	fmt.Printf("ICRS (J2000)       RA %9.5f°   Dec %9.5f°\n", sgrA.RA().Degrees(), sgrA.Dec().Degrees())
	fmt.Printf("Galactic           l  %9.5f°   b   %9.5f°\n", galactic.L().Degrees(), galactic.B().Degrees())
	fmt.Printf("  and back         RA %9.5f°   Dec %9.5f°   (%.1g arcsec off)\n",
		back.RA().Degrees(), back.Dec().Degrees(), coord.Separation(sgrA, back).Arcseconds())
	fmt.Printf("Ecliptic of date   λ  %9.5f°   β   %9.5f°\n", ecliptic.Lon().Degrees(), ecliptic.Lat().Degrees())
	fmt.Printf("AltAz              Alt %8.4f°   Az  %9.4f°\n", altaz.Alt().Degrees(), altaz.Az().Degrees())
	fmt.Printf("  from Quinta Calixto at %s (%s)\n\n",
		t.Format("2006-01-02 15:04 MST"), t.In(time.LocationUTC).Format("15:04 MST"))

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

	fmt.Println("Batch: five stars, one Context")
	fmt.Println("==============================")

	for i, s := range stars {
		fmt.Printf("%-15s  Alt %6.2f°   Az %6.2f°\n", s.name, out[i].Alt().Degrees(), out[i].Az().Degrees())
	}
}
