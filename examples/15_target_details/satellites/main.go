// Package main demonstrates satellite target details.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/catalog"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/remote"
	_ "github.com/TuSKan/astrogo/remote/eop"
	"github.com/TuSKan/astrogo/time"
)

func main() {
	// Earth orientation, for where the site is on the turning Earth: the
	// IERS bulletin, which the remote/eop import above fetches.
	remote.EnableDownloads(0, remote.IERSFinals2000A)

	loc, err := coord.NewEarthLocation(-23.5505, -46.6333, 760.0) // São Paulo
	if err != nil {
		log.Fatalf("failed to create location: %v", err)
	}

	// The ISS's current element set, from CelesTrak.
	resolver := catalog.NewResolver(catalog.NORAD)

	catTarget, err := resolver.Resolve(context.Background(), "ISS (Zarya)")
	if err != nil {
		log.Fatalf("failed to resolve ISS: %v", err)
	}

	// For satellites, we create an ephemeris provider using TLE strings
	prov, err := eph.NewProvider(context.Background(), eph.Satellites, catTarget.Name, eph.WithTLE(catTarget.TLELine1, catTarget.TLELine2))
	if err != nil {
		log.Fatalf("failed to create satellite provider: %v", err)
	}
	defer func() {
		err := prov.Close()
		if err != nil {
			log.Printf("failed to close provider: %v", err)
		}
	}()

	iss, err := plan.FromCatalog(catTarget, prov)
	if err != nil {
		log.Fatalf("build target: %v", err)
	}

	// The ISS right now, in São Paulo's time zone. Unlike the other
	// examples this one cannot use a fixed date: a TLE describes an orbit
	// well for a few days either side of its epoch, and CelesTrak serves
	// the current one.
	tz, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		log.Fatalf("tz: %v", err)
	}

	t := time.NowUTC().In(tz)
	ctx := coord.NewContext(t, loc, atmosphere.StandardRefraction())

	fmt.Printf("Element set epoch: %s (%.1f days ago)\n\n",
		catTarget.Epoch.In(tz).Format("2006-01-02 15:04 MST"), t.Sub(catTarget.Epoch).Days())

	details, err := iss.GetDetails(ctx, plan.DetailOverrides{Description: "International Space Station", Source: "NORAD TLE"})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(details)
}
