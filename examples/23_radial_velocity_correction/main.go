// Package main demonstrates barycentric/heliocentric radial-velocity
// correction — the standard pre-step in spectroscopic work that removes
// the observer's own motion (Earth's orbit plus the site's diurnal
// rotation, and optionally the Sun's own barycentric motion) from a
// measured line-of-sight velocity.
//
// This showcase demonstrates:
//   - coord.Context.BarycentricVelocity: the observer's own barycentric
//     velocity vector, already computed by the SOFA astrometry this
//     library builds per epoch — nothing new to derive
//   - coord.Context.BarycentricRVCorrection / HeliocentricRVCorrection:
//     project that velocity onto a target's direction and report the
//     value to ADD to a measured RV
//   - The correction's annual sinusoid, driven by Earth's own orbital
//     motion, sampled month by month across a year
//
// Run: go run ./examples/23_radial_velocity_correction/
package main

import (
	"fmt"
	"log"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/examples/internal/show"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

func main() {
	// This example runs offline, so it uses no Earth-orientation data
	// (UT1 = UTC, no polar motion). That changes the observatory's
	// rotational velocity toward the star by a few centimeters per second,
	// far below the meters per second printed. For full accuracy,
	// blank-import remote/eop and consent to the IERS bulletin, as
	// 09_geometry_events does.
	time.RegisterModel(time.ZeroModel{})

	// Sirius (α CMa): a well-known bright star with a real published
	// radial velocity — re-verifiable against any modern stellar catalog
	// (here SIMBAD). A catalog RV is already corrected to the barycenter:
	// it is the star's own motion, the same on every night.
	const (
		siriusRADeg      = 101.287155
		siriusDecDeg     = -16.716116
		siriusPublishedV = -5.5 // km/s, barycentric, from SIMBAD
	)

	target := coord.NewICRS(angle.Deg(siriusRADeg), angle.Deg(siriusDecDeg))

	site, err := plan.NewKnownSite("Mauna Kea")
	if err != nil {
		log.Fatalf("site: %v", err)
	}

	show.Title("Radial-velocity correction: Sirius",
		"coord.Context, from "+site.Name()+" across 2026")
	fmt.Println()

	show.Fields(
		show.KV("Target", "Sirius (α CMa)"),
		show.KV("RA / Dec", "%s / %s", target.RA().HMSString(2), target.Dec().DMSString(1)),
		show.KV("Published RV", "%.2f km/s, barycentric", siriusPublishedV),
		show.KV("Site", "%s", site.Name()),
	)

	show.Section("What the site measures, and the correction back")
	show.Note("Earth's orbital motion adds up to ±30 km/s to what a spectrograph",
		"reads. The correction removes it, so every row comes back to the",
		"star's own -5.50 km/s.")
	fmt.Println()

	tab := show.NewTable(show.Left("Date"), show.Right("Barycentric corr."),
		show.Right("Heliocentric corr."), show.Right("Measured"), show.Right("Corrected"))

	epoch := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC)

	for m := range 12 {
		t := epoch.Add(unit.Days(float64(m) * 30))

		ctx := coord.NewContext(t, site.Location(), atmosphere.StandardRefraction())

		baryCorr := ctx.BarycentricRVCorrection(target)

		helioCorr, err := ctx.HeliocentricRVCorrection(target)
		if err != nil {
			log.Fatalf("heliocentric correction: %v", err)
		}

		// What a spectrograph at the site reads on this date: the star's
		// barycentric RV seen from the moving observatory.
		measured, err := ctx.ObservedRadialVelocity(target, unit.KmPerSec(siriusPublishedV))
		if err != nil {
			log.Fatalf("observed radial velocity: %v", err)
		}

		// And the correction back, by the exact composition the note below
		// explains, not measured + baryCorr.
		corrected, err := ctx.BarycentricRadialVelocity(target, measured)
		if err != nil {
			log.Fatalf("barycentric radial velocity: %v", err)
		}

		// unit.Velocity holds m/s; KmPerSec is what the column says.
		tab.Row(t.Format("2006-01-02"),
			fmt.Sprintf("%+.3f km/s", baryCorr.KmPerSec()),
			fmt.Sprintf("%+.3f km/s", helioCorr.KmPerSec()),
			fmt.Sprintf("%+.3f km/s", measured.KmPerSec()),
			fmt.Sprintf("%+.3f km/s", corrected.KmPerSec()))
	}

	tab.Print()

	show.Section("How the correction is applied")
	show.Note("rvBarycentric = ctx.BarycentricRadialVelocity(target, rvMeasured)")
	fmt.Println()
	show.Note("Not rvMeasured + the correction: redshifts compose by multiplying,",
		"so the exact form carries a third term rvMeasured*corr/c. With corr",
		"at most Earth's 30 km/s, that is up to 0.55 m/s for Sirius and 30 m/s",
		"for a halo star at 300 km/s.")
	fmt.Println()
	show.Note("It carries the observer's own clock as well as their motion: the",
		"second-order Doppler shift, the Sun's gravitational potential at the",
		"observer and Earth's own, together about 4.65 m/s. Against Astropy",
		"over 175 cases the two agree within 3 mm/s, half a millimeter per",
		"second typically.")
	fmt.Println()
	show.Note("Still omitted: light-travel time to the barycenter, and the target's",
		"own proper motion over that crossing. Both need the target's distance",
		"and epoch, which a bare ICRS direction does not carry.")
}
