// Package main demonstrates deep-sky object target details.
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/catalog"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/epoch"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/unit"
)

func main() {
	// OpenNGC is listed alongside SIMBAD below: catalog.NewResolver's first use
	// of catalog.OpenNGC fetches it (content-checked, so a re-run only costs a
	// HEAD probe once cached). Earth orientation, for where the site is on the
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

	resolver := catalog.NewResolver(catalog.OpenNGC, catalog.SIMBAD)

	catTarget, err := resolver.Resolve(context.Background(), "M31")
	if err != nil {
		log.Fatalf("failed to resolve M31: %v", err)
	}

	m31, err := plan.FromCatalog(catTarget, nil)
	if err != nil {
		log.Fatalf("build target: %v", err)
	}

	details, err := m31.GetDetails(ctx, plan.DetailOverrides{Description: "Andromeda Galaxy", Source: "OpenNGC"})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(details)
}
