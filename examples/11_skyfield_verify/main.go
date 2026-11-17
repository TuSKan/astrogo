// Package main cross-checks astrogo's planetary ephemeris against JPL
// Horizons and Skyfield: the geocentric astrometric position of Mars, the
// computation Skyfield's observe() performs, from the built-in analytical
// series and from the JPL DE440 kernel.
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

// reference is one published place of the Mars system barycenter, as seen
// from the geocenter at 2025-07-03 12:00 UTC: right ascension and declination
// in degrees, distance in au.
type reference struct {
	name           string
	raDeg, decDeg  float64
	distAU         float64
	raPlaces, decP int // the digits the source gives, for printing them
}

var (
	// JPL Horizons' astrometric place, retrieved through the Horizons API with
	//
	//	COMMAND='4' CENTER='500@399' EPHEM_TYPE='OBSERVER'
	//	TLIST='2025-07-03 12:00' TIME_TYPE='UT' QUANTITIES='1,20'
	//	ANG_FORMAT='DEG' EXTRA_PREC='YES'
	//
	// Target 4 is the Mars system barycenter, which a DE kernel and the
	// analytical series both hold. Horizons puts Mars's own center, 499, at
	// the same RA and Dec to all nine decimals and 15 cm farther. Its angles
	// have nine decimals of a degree, 3.6 microarcseconds, and its distance
	// fourteen digits.
	horizons = reference{"JPL Horizons (DE441)", 160.844581525, 9.153567896, 1.94245684464294, 5, 4}

	// Skyfield 1.55's place for the same instant, from the DE440 kernel this
	// example also reads, computed with
	//
	//	from skyfield.api import load, load_file
	//
	//	k = load_file('de440.bsp')
	//	ts = load.timescale(builtin=True)
	//	t = ts.utc(2025, 7, 3, 12, 0)
	//	ra, dec, distance = k['earth'].at(t).observe(k['mars barycenter']).radec()
	//
	// with ra._degrees, dec.degrees and distance.au printed in full.
	skyfield = reference{"Skyfield 1.55 (DE440)", 160.844581524633, 9.153567896427, 1.942456844643015, 5, 4}
)

// meters per au, for the distance differences.
var metersPerAU = constants.IAU.AstronomicalUnit.Value

func main() {
	// Earth orientation: like every online example, this one imports
	// remote/eop and consents to the IERS bulletin. A geocentric astrometric
	// place does not use it, having no site and no Earth rotation in it.
	remote.EnableDownloads(0, remote.IERSFinals2000A)

	t := time.Date(2025, 7, 3, 12, 0, 0, 0, time.LocationUTC)

	show.Title("Mars, geocentric astrometric place", "astrogo against JPL Horizons and Skyfield")
	fmt.Println()
	show.Fields(
		show.KV("Epoch", "%s  (TDB JD %.6f)", t.Format("2006-01-02 15:04:05 MST"), t.TDB().JD()),
		show.KV("Target", "%s", "the Mars system barycenter, from the Earth's center"),
	)

	places := show.NewTable(show.Left("Source"), show.Left("RA"), show.Left("Dec"), show.Right("Distance"))
	againstHorizons := show.NewTable(show.Left("Source"), show.Right("On the sky"), show.Right("In distance"),
		show.Right("Allowed"))

	for _, ref := range []reference{horizons, skyfield} {
		c := ref.icrs()
		places.Row(ref.name, c.RA().HMSString(ref.raPlaces), c.Dec().DMSString(ref.decP),
			fmt.Sprintf("%.12f au", ref.distAU))
	}

	againstHorizons.Row(skyfield.name, onTheSky(skyfield.icrs()), inDistance(skyfield.distAU), "a reference")

	// 1. The built-in provider: SOFA's Plan94 analytical series, no kernel.
	// Its worst case for Mars over 1800-2100 is 25,000 km (docs/VALIDATION.md),
	// 17.7 arcsec at this distance.
	builtIn := place(eph.Default(), t, "astrogo, built-in Plan94", 17.7)

	// 2. The JPL DE440 kernel. Downloads are opt-in, see README "Data
	// downloads & offline usage": de440 is ~114 MB; naif0012.tls (leap
	// seconds) ~5 KB.
	remote.EnableDownloads(200<<20, remote.NAIFSPK)
	remote.EnableDownloads(0, remote.NAIFLSK)

	var kernel *result

	jplProv, err := eph.NewProvider(context.Background(), eph.Planets, "de440")
	if err == nil {
		defer func() {
			if err := jplProv.Close(); err != nil {
				log.Printf("failed to close JPL provider: %v", err)
			}
		}()

		// The kernel's astrometric place is contracted at 0.01 arcsec against
		// Horizons (docs/VALIDATION.md).
		kernel = place(jplProv, t, "astrogo, JPL DE440", 0.01)
	}

	for _, r := range []*result{kernel, builtIn} {
		if r == nil {
			continue
		}

		places.Row(r.name, r.icrs.RA().HMSString(5), r.icrs.Dec().DMSString(4), fmt.Sprintf("%.12f au", r.dist))
		againstHorizons.Row(r.name, onTheSky(r.icrs), inDistance(r.dist), r.allowed)
	}

	show.Section("Astrometric place")
	places.Print()

	show.Section("Against JPL Horizons")
	againstHorizons.Print()

	if kernel == nil {
		fmt.Println()
		show.Note(fmt.Sprintf("JPL DE440 unavailable, skipped: %v", err))

		return
	}

	show.Section("Against Skyfield, the same kernel")

	againstSkyfield := show.NewTable(show.Left("Source"), show.Right("On the sky"), show.Right("In distance"))
	againstSkyfield.Row(kernel.name, mas(coord.Separation(kernel.icrs, skyfield.icrs())),
		fmt.Sprintf("%.3f m", math.Abs(kernel.dist-skyfield.distAU)*metersPerAU))
	againstSkyfield.Print()

	fmt.Println()
	show.Note(
		"The kernel. Horizons, Skyfield and astrogo's DE440 agree on the sky",
		"to a few microarcseconds, which is as far as Horizons' nine decimals",
		"of a degree resolve (3.6). They read DE441 and DE440, which put Mars",
		"1.6 cm apart on this date. astrogo's distance is 15 cm from both,",
		"five parts in ten trillion.",
		"",
		"The built-in series. Plan94 (Simon et al. 1994) is analytical, so it",
		"needs no download, at an accuracy of thousands of kilometers. SOFA",
		"documents its Mars position as 7,690 km RMS over 1960-2025, about",
		`5.5" at this distance, and astrogo holds it to 25,000 km at worst`,
		`over 1800-2100 (docs/VALIDATION.md), 17.7" here. Its 2.76" on this`,
		"date is typical, not a fault.",
		"",
		"Allowed is the provider's documented accuracy, and a * marks an",
		"offset outside it.",
	)
}

// mas formats an angle in milliarcseconds, to a tenth of a microarcsecond.
func mas(a angle.Angle) string {
	return fmt.Sprintf("%.4f mas", a.Arcseconds()*1000)
}

// result is one astrogo provider's place: its ICRS direction, its distance
// in au, and the accuracy its documentation allows, for the tables.
type result struct {
	name    string
	icrs    coord.ICRS
	dist    float64
	allowed string
}

// place computes prov's astrometric place of the Mars barycenter at t: light
// time applied, no aberration or light deflection, which is what Horizons and
// Skyfield's radec() report. limitArcsec is the provider's documented
// accuracy, starred when the offset from Horizons is outside it.
func place(prov eph.Provider, t time.Time, name string, limitArcsec float64) *result {
	st, err := eph.At(prov, t).Astrometric(eph.MarsBarycenter, eph.Geocenter())
	if err != nil {
		log.Fatalf("%s: %v", name, err)
	}

	icrs, err := eph.ToICRS(st.Pos)
	if err != nil {
		log.Fatalf("%s: %v", name, err)
	}

	allowed := fmt.Sprintf(`%g"`, limitArcsec)
	if coord.Separation(icrs, horizons.icrs()).Arcseconds() > limitArcsec {
		allowed += " *"
	}

	return &result{name: name, icrs: icrs, dist: st.Pos.Norm(), allowed: allowed}
}

func (r reference) icrs() coord.ICRS {
	return coord.NewICRS(angle.Deg(r.raDeg), angle.Deg(r.decDeg))
}

// onTheSky is c's separation from Horizons' place.
func onTheSky(c coord.ICRS) string {
	return mas(coord.Separation(c, horizons.icrs()))
}

// inDistance is the difference from Horizons' distance, in meters to the
// millimeter.
func inDistance(au float64) string {
	return fmt.Sprintf("%.3f m", math.Abs(au-horizons.distAU)*metersPerAU)
}
