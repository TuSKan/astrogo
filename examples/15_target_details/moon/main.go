// Package main demonstrates moon target details.
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/epoch"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/unit"

	// The kernel-backed sources register their backend rather than being
	// imported by the root package, so a build that wants them says so (#112).
	_ "github.com/TuSKan/astrogo/ephemeris/jpl"
)

func main() {
	// de442 is ~115 MB and naif0012.tls (leap seconds) ~5 KB; both download on
	// first run and are cached. Earth orientation, for where the site is on the
	// turning Earth: the IERS bulletin, which downloads on first need and is
	// cached.
	loc, err := coord.NewGeodetic(angle.LatDeg(-23.5505), angle.LonDeg(-46.6333), unit.Meters(760.0))
	if err != nil {
		log.Fatalf("failed to create location: %v", err)
	}

	tz, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		log.Fatalf("tz: %v", err)
	}

	t := epoch.Date(2026, 4, 25, 17, 0, 0, 0, tz) // 20:00 UTC

	ctx, err := coord.NewContext(t, loc, atmosphere.StandardRefraction())
	if err != nil {
		log.Fatalf("NewContext: %v", err)
	}

	prov, err := eph.NewProvider(context.Background(), eph.Planets, "de442")
	if err != nil {
		log.Fatalf("failed to create provider: %v", err)
	}

	moon := plan.NewMoon(prov)

	details, err := moon.GetDetails(ctx, plan.DetailOverrides{Description: "Earth's natural satellite", Source: "JPL DE442"})
	if err != nil {
		log.Fatalf("failed to get details: %v", err)
	}

	fmt.Println(details)
}
