// Package main cross-checks astrogo's planetary ephemeris against JPL
// Horizons: the geocentric astrometric position of Mars, the computation
// Skyfield's observe() performs, from the built-in analytical series and
// from the JPL DE440 kernel.
package main

import (
	"context"
	"fmt"
	"log"
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/examples/internal/show"
	"github.com/TuSKan/astrogo/remote"
	_ "github.com/TuSKan/astrogo/remote/eop"
	"github.com/TuSKan/astrogo/time"

	// The kernel-backed sources register their backend rather than being
	// imported by the root package, so a build that wants them says so (#112).
	_ "github.com/TuSKan/astrogo/ephemeris/jpl"
)

// The same computation in Python, with Skyfield:
//
//	from skyfield.api import load
//
//	planets = load('de440.bsp')
//	earth, mars = planets['earth'], planets['mars barycenter']
//
//	ts = load.timescale()
//	t = ts.utc(2025, 7, 3, 12, 0)
//	ra, dec, distance = earth.at(t).observe(mars).radec()
//
// The reference is JPL Horizons' astrometric place for the same instant,
// retrieved through the Horizons API with:
//
//	COMMAND='4' CENTER='500@399' EPHEM_TYPE='OBSERVER'
//	TLIST='2025-07-03 12:00' TIME_TYPE='UT' QUANTITIES='1,20'
//	ANG_FORMAT='DEG' EXTRA_PREC='YES'
//
// Target 4 is the Mars system barycenter, which a DE kernel and the
// analytical series both hold. Horizons puts Mars's own center, 499, at the
// same RA and Dec to all nine decimals and 15 cm farther.
const (
	horizonsRADeg  = 160.844581525
	horizonsDecDeg = 9.153567896
	horizonsDistAU = 1.94245684464294
)

// kmPerAU is the astronomical unit in kilometers.
var kmPerAU = constants.IAU.AstronomicalUnit.Value / 1e3

func main() {
	// Earth orientation: like every online example, this one imports
	// remote/eop and consents to the IERS bulletin. A geocentric astrometric
	// place does not use it, having no site and no Earth rotation in it.
	remote.EnableDownloads(0, remote.IERSFinals2000A)

	t := time.Date(2025, 7, 3, 12, 0, 0, 0, time.LocationUTC)
	ref := coord.NewICRS(angle.Deg(horizonsRADeg), angle.Deg(horizonsDecDeg))

	show.Title("Mars, geocentric astrometric place", "astrogo against JPL Horizons")
	fmt.Println()
	show.Fields(
		show.KV("Epoch", "%s  (TDB JD %.6f)", t.Format("2006-01-02 15:04:05 MST"), t.TDB().JD()),
		show.KV("Horizons", "RA %s  Dec %s  %.9f AU  (DE441)",
			ref.RA().HMSString(4), ref.Dec().DMSString(3), horizonsDistAU),
	)

	places := show.NewTable(show.Left("Source"), show.Left("RA"), show.Left("Dec"), show.Right("Distance"))
	offsets := show.NewTable(show.Left("Source"), show.Right("Off"), show.Right("Range"), show.Right("Documented"))

	// 1. The built-in provider: SOFA's Plan94 analytical series, no kernel.
	// Its documented worst case for Mars over 1800-2100 is 25,000 km
	// (docs/VALIDATION.md), 17.7 arcsec at this distance.
	compare(places, offsets, "Built-in (Plan94)", eph.Default(), t, ref, 17.7)

	// 2. The JPL DE440 kernel. Downloads are opt-in, see README "Data
	// downloads & offline usage": de440 is ~114 MB; naif0012.tls (leap
	// seconds) ~5 KB. Its astrometric place is contracted at 0.01 arcsec
	// against Horizons (docs/VALIDATION.md).
	remote.EnableDownloads(200<<20, remote.NAIFSPK)
	remote.EnableDownloads(0, remote.NAIFLSK)

	jplProv, err := eph.NewProvider(context.Background(), eph.Planets, "de440")
	if err != nil {
		printResults(places, offsets)
		fmt.Println()
		show.Note(fmt.Sprintf("JPL DE440 unavailable, skipped: %v", err))

		return
	}
	defer func() {
		if err := jplProv.Close(); err != nil {
			log.Printf("failed to close JPL provider: %v", err)
		}
	}()

	compare(places, offsets, "JPL DE440", jplProv, t, ref, 0.01)

	printResults(places, offsets)
	fmt.Println()
	show.Note("Off is the separation from Horizons on the sky, Range the",
		"difference in distance, and Documented the provider's own stated",
		"accuracy (docs/VALIDATION.md). A * marks an offset outside it.")
}

// printResults prints the two tables compare fills.
func printResults(places, offsets *show.Table) {
	show.Section("Astrometric place")
	places.Print()
	show.Section("Against Horizons")
	offsets.Print()
}

// compare adds a row to each table for prov's astrometric place of the Mars
// barycenter at t: the place to places, and to offsets its offset from the
// Horizons reference beside the provider's documented accuracy, maxArcsec,
// starred if the offset on the sky is outside it.
func compare(places, offsets *show.Table, name string, prov eph.Provider, t time.Time, ref coord.ICRS, maxArcsec float64) {
	// The astrometric place: light time applied, no aberration or light
	// deflection, which is what Horizons and Skyfield's radec() report.
	st, err := eph.At(prov, t).Astrometric(eph.MarsBarycenter, eph.Geocenter())
	if err != nil {
		log.Fatalf("%s: %v", name, err)
	}

	icrs, err := eph.ToICRS(st.Pos)
	if err != nil {
		log.Fatalf("%s: %v", name, err)
	}

	dist := st.Pos.Norm()
	sep := coord.Separation(icrs, ref).Arcseconds()
	dKm := math.Abs(dist-horizonsDistAU) * kmPerAU

	limit := fmt.Sprintf(`%g"`, maxArcsec)
	if sep > maxArcsec {
		limit += " *"
	}

	places.Row(name, icrs.RA().HMSString(4), icrs.Dec().DMSString(3), fmt.Sprintf("%.9f AU", dist))
	offsets.Row(name, fmt.Sprintf(`%.4f"`, sep), fmt.Sprintf("%.3f km", dKm), limit)
}
