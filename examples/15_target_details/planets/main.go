// Package main demonstrates planet target details.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/time"
)

func main() {
	loc, err := coord.NewEarthLocation(-23.5505, -46.6333, 760.0)
	if err != nil {
		log.Fatalf("failed to create location: %v", err)
	}

	tz, _ := time.LoadLocation("UTC")
	t := time.Date(2026, 4, 25, 20, 0, 0, 0, tz)
	ctx := coord.NewContext(t, loc, atmosphere.StandardAtmosphere)

	// JPL kernel downloads are opt-in — see README "Data downloads &
	// offline usage". de442 is ~115 MB; naif0012.tls (leap seconds) ~5 KB.
	remote.EnableDownloads(remote.NAIFSPK, 200<<20)
	remote.EnableDownloads(remote.NAIFLSK, 0)

	prov, err := eph.NewProvider(context.Background(), eph.Planets, "de442")
	if err != nil {
		log.Fatalf("failed to create provider: %v", err)
	}

	mars := plan.NewMars(prov)

	details, err := mars.GetDetails(ctx, "Description", "The Red Planet", "Source", "JPL DE442")
	if err != nil {
		log.Fatalf("failed to get details: %v", err)
	}

	fmt.Println(details)
}
