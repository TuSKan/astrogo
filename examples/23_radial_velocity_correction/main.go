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
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
)

func main() {
	// Sirius (α CMa): a well-known bright star with a real published
	// heliocentric radial velocity — re-verifiable against any modern
	// stellar catalog (e.g. SIMBAD/Gaia DR3).
	const (
		siriusRAHours  = 6.7524861
		siriusDecDeg   = -16.7161083
		siriusMeasured = -5.5 // km/s, published heliocentric RV
	)

	target := coord.NewICRS(angle.Hour(siriusRAHours), angle.Deg(siriusDecDeg))

	site, err := plan.NewKnownSite("Mauna Kea")
	if err != nil {
		log.Fatalf("site: %v", err)
	}

	fmt.Println("═══════════════════════════════════════════════════════════════════")
	fmt.Println("  SIRIUS — barycentric/heliocentric radial-velocity correction")
	fmt.Println("  AstroGo | coord.Context | classical projection, ~1 m/s accuracy")
	fmt.Println("═══════════════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Printf("  Target             Sirius (α CMa)\n")
	fmt.Printf("  RA / Dec           %s / %s\n", target.RA().HMSString(2), target.Dec().DMSString(1))
	fmt.Printf("  Measured RV        %.2f km/s (published, heliocentric)\n", siriusMeasured)
	fmt.Printf("  Site               %s\n", site.Name())

	fmt.Println()
	fmt.Println("── Correction across 2026 (Earth's orbital motion drives the swing) ──")
	fmt.Println()
	fmt.Println("  Date          Bary. corr.   Helio. corr.   Bary.-corrected RV")
	fmt.Println("  ──────────    ───────────   ────────────   ───────────────────")

	epoch := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC)

	for m := range 12 {
		t := epoch.AddDays(float64(m) * 30)

		ctx := coord.NewContext(t, site.Location(), atmosphere.StandardRefraction)

		baryCorr := ctx.BarycentricRVCorrection(target)

		helioCorr, err := ctx.HeliocentricRVCorrection(target)
		if err != nil {
			log.Fatalf("heliocentric correction: %v", err)
		}

		fmt.Printf("  %s    %+8.3f km/s   %+8.3f km/s   %+8.3f km/s\n",
			t.Format("2006-01-02"), baryCorr, helioCorr, siriusMeasured+baryCorr)
	}

	fmt.Println()
	fmt.Println("  rvBarycentric = ctx.BarycentricRadialVelocity(target, rvMeasured)")
	fmt.Println("  Not rvMeasured + the correction: redshifts compose by multiplying,")
	fmt.Println("  so the exact form carries a third term rvMeasured*corr/c. For Sirius")
	fmt.Println("  that is 0.55 m/s; for a halo star at 300 km/s it is 30 m/s.")
	fmt.Println("  This is a classical (non-relativistic) velocity projection, accurate")
	fmt.Println("  to ~1 m/s — it does not implement gravitational redshift, light-time")
	fmt.Println("  to the barycenter, or a target's own proper-motion/parallax effects")
	fmt.Println("  on the projection geometry (the full Wright & Eastman 2014 treatment).")
	fmt.Println("═══════════════════════════════════════════════════════════════════")
}
