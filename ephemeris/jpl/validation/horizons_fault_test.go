//go:build network

package jpl_test

import (
	"errors"
	"testing"

	"github.com/TuSKan/astrogo/internal/horizons"
	"github.com/TuSKan/astrogo/internal/testutil"
)

// TestNoDataBlockTellsAFaultFromAMissingTable: an answer without a table is a
// fault on Horizons' server when it says so, which the suites skip on, and a
// failure otherwise. On 2026-10-26 the first case reached six tests here as
// "no ephemeris data in the response" and failed them all (#858).
func TestNoDataBlockTellsAFaultFromAMissingTable(t *testing.T) {
	t.Parallel()

	// Horizons' text answer on 2026-10-26, header and all, as the kepler
	// suite logged it.
	fault := "API VERSION: 1.2\nAPI SOURCE: NASA/JPL Horizons API\n\n" +
		"wldini(): missing required file LTKERNL\n\n ERROR in VLRDC: Var not declared: IP_ADDR\n"

	err := noDataBlock(fault)
	if !errors.Is(err, horizons.ErrServerFault) {
		t.Errorf("noDataBlock(the 2026-10-26 fault) = %v, want horizons.ErrServerFault", err)
	}

	if _, upstream := testutil.UpstreamFailure(err); !upstream {
		t.Errorf("testutil.UpstreamFailure(%v) = false: the suites would fail on JPL's outage", err)
	}

	// A well-formed answer that simply holds no table is not an outage.
	empty := "API VERSION: 1.2\nAPI SOURCE: NASA/JPL Horizons API\n\nNo ephemeris for target \"Ceres\" prior to A.D. 1600-JAN-01\n"

	err = noDataBlock(empty)
	if !errors.Is(err, errNoEphemerisData) || errors.Is(err, horizons.ErrServerFault) {
		t.Errorf("noDataBlock(an answer without a table) = %v, want errNoEphemerisData alone", err)
	}

	if _, upstream := testutil.UpstreamFailure(err); upstream {
		t.Errorf("testutil.UpstreamFailure(%v) = true: a missing table would be skipped", err)
	}
}
