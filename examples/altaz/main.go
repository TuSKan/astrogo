package main

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/TuSKan/astrogo/catalog"
	"github.com/TuSKan/astrogo/coords"
	"github.com/TuSKan/astrogo/ephem"
	"github.com/TuSKan/astrogo/render"
)

func getWorkspace() string {
	out, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil {
		return ""
	}
	return filepath.Dir(strings.TrimSpace(string(out)))
}

func main() {
	// 1. Define the observer location for São Paulo
	loc := coords.Location{
		Latitude:  -23.55,
		Longitude: -46.63,
		Elevation: 760.0,
	}

	// 2. Look up "M42" using robust normalized catalog.Lookup()
	target, err := catalog.Lookup("M42")
	if err != nil {
		fmt.Printf("Failed to strictly resolve target properties: %v\n", err)
		return
	}

	// 3. Initialize the JPL Ephemeris engine dynamically
	bspPath := filepath.Join(getWorkspace(), "ephem/data/linux_p1550p2650.430")
	bspFile, err := os.Open(bspPath)
	if err != nil {
		fmt.Printf("Failed to open JPL Ephemeris binary file: %s, %v\n", bspPath, err)
		return
	}
	engine, err := ephem.NewEngine(bspFile)
	if err != nil {
		fmt.Printf("Failed to mount JPL Ephemeris geometric engine constraints: %v\n", err)
		return
	}
	defer engine.Close()

	// 4. Define strictly the local start time
	start := time.Date(2026, 3, 19, 18, 0, 0, 0, time.Local)

	var points []render.ObservationPoint

	fmt.Printf("Evaluating precise JPL ephemeris maps compiling %s tonight...\n", target.CommonNames)

	// 5. Build dynamic continuous point maps running over 12 hours cleanly (144 graphical mappings)
	for i := 0; i <= 144; i++ {
		t := start.Add(time.Duration(i*5) * time.Minute)

		// Extract target geometry properties naturally via Astrometric bounds
		altRad, _, err := coords.ICRSToObserved(target.RA, target.Dec, t, loc, 0.0)
		if err != nil {
			fmt.Printf("Error isolating Astrometric geometries bounds natively: %v\n", err)
			return
		}

		targetAltDeg := altRad * (180.0 / math.Pi)

		// Formulate precision Julian mathematical time constraints exactly matching ephemeris boundaries natively
		jd := float64(t.Unix())/86400.0 + 2440587.5

		// Process purely physical Solar vectors inherently tracking standard JPL limits
		x, y, z, err := engine.GetPosition(11, jd) // 11 evaluates natively exactly against the standard Sun ID
		if err != nil {
			fmt.Printf("Ephemeris boundaries failed executing Cartesian boundaries natively: %v\n", err)
			return
		}

		// Remap Cartesian AU explicitly across abstract Angular tracking logic locally
		sunRA, sunDec := ephem.VectorToEquatorial(x, y, z)

		// Evaluate pure solar structural angles cleanly tracking exact astronomical conditions perfectly
		sunAltRad, _, err := coords.ICRSToObserved(sunRA, sunDec, t, loc, 0.0)
		if err != nil {
			fmt.Printf("Error formulating native solar topologies safely: %v\n", err)
			return
		}

		sunAltDeg := sunAltRad * (180.0 / math.Pi)

		// Aggregate explicit data matrices safely structurally mapping time geometries precisely
		points = append(points, render.ObservationPoint{
			Time:      t,
			TargetAlt: targetAltDeg,
			SunAlt:    sunAltDeg,
		})
	}

	// 6. Execute graphical representations cleanly generating native bounds purely natively
	renderer := render.NewRenderer(render.DefaultConfig())
	err = renderer.DrawTransitCurve(points, "m42_transit.png")
	if err != nil {
		fmt.Printf("Renderer completely dropped executing structural layouts smoothly natively: %v\n", err)
	} else {
		fmt.Println("Successfully compiled precise JPL generated altitude boundaries plotting seamlessly natively onto m42_transit.png!")
	}
}
