//go:build integration

package workload_test

import (
	// Observed places depend on UT1 and polar motion, and a benchmark of them
	// without Earth Orientation Parameters measures a different computation:
	// every epoch would silently take zero DUT1.
	_ "github.com/TuSKan/astrogo/remote/eop"

	// eph.NewProvider reaches the SPK backend through a registration.
	_ "github.com/TuSKan/astrogo/ephemeris/jpl"
)
