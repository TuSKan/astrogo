package ephemeris_test

import (
	"errors"
	"testing"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
)

// TestProvidersLabelTheirFrame is the point of the Frame and Center fields.
//
// Before them, the frame contract on State was a comment reading "Geocentric
// position in AU (ICRS-like)" — hedged, on a struct several providers fill in
// differently. The analytical provider produces ICRS; ephemeris/satellite
// converts SGP4's TEME output to GCRS and produces that. Those differ by frame
// bias, about 23 milliarcseconds, and nothing in the type distinguished them.
//
// A value from one used where the other was meant stays mathematically valid
// and becomes physically wrong, which is the failure mode this whole class of
// bug shares with the four time-scale defects: an invariant that lived in
// prose instead of in something a compiler or a test can see.
func TestProvidersLabelTheirFrame(t *testing.T) {
	t.Parallel()

	when := time.Date(2026, 1, 1, 0, 0, 0, 0, time.LocationUTC)

	st, err := eph.Default().State(eph.Mars, when)
	if err != nil {
		t.Fatalf("State: %v", err)
	}

	if st.Frame != eph.FrameICRS {
		t.Errorf("the analytical provider labels its frame %s, want ICRS", st.Frame)
	}

	if st.Center != eph.CenterGeocentre {
		t.Errorf("the analytical provider labels its origin %s, want geocentre", st.Center)
	}
}

// TestRequireCatchesAFrameMismatch checks the guard the labels exist for.
func TestRequireCatchesAFrameMismatch(t *testing.T) {
	t.Parallel()

	icrs := eph.State{Frame: eph.FrameICRS, Center: eph.CenterGeocentre}
	gcrs := eph.State{Frame: eph.FrameGCRS, Center: eph.CenterGeocentre}

	if err := icrs.Require(eph.FrameICRS, eph.CenterGeocentre); err != nil {
		t.Errorf("a matching state was refused: %v", err)
	}

	// The case that used to be invisible: a satellite state where a planetary
	// one was meant.
	err := gcrs.Require(eph.FrameICRS, eph.CenterGeocentre)
	if !errors.Is(err, eph.ErrWrongFrame) {
		t.Errorf("GCRS passed as ICRS: err = %v, want ErrWrongFrame", err)
	}

	if err := icrs.Require(eph.FrameICRS, eph.CenterBarycentre); !errors.Is(err, eph.ErrWrongCenter) {
		t.Errorf("geocentric passed as barycentric: err = %v, want ErrWrongCenter", err)
	}
}

// An unlabelled state passes rather than failing.
//
// The zero value asserts nothing, and Require checks against a *wrong* label
// rather than demanding every producer carry one. Making unspecified an error
// would turn "we do not know" into a failure at every call site not yet
// updated, which is a migration rather than a safeguard — and would punish
// third-party providers implementing the interface for astrogo's own change.
func TestRequireAcceptsAnUnlabelledState(t *testing.T) {
	t.Parallel()

	var unlabelled eph.State

	if err := unlabelled.Require(eph.FrameICRS, eph.CenterBarycentre); err != nil {
		t.Errorf("an unlabelled state was refused: %v", err)
	}

	// And a labelled state is not constrained by a caller who does not care.
	gcrs := eph.State{Frame: eph.FrameGCRS, Center: eph.CenterGeocentre}
	if err := gcrs.Require(eph.FrameUnspecified, eph.CenterUnspecified); err != nil {
		t.Errorf("a caller with no requirement got an error: %v", err)
	}
}

// The String forms appear in the error messages above, so they have to read.
func TestFrameAndCenterStrings(t *testing.T) {
	t.Parallel()

	for got, want := range map[string]string{
		eph.FrameUnspecified.String():  "unspecified",
		eph.FrameICRS.String():         "ICRS",
		eph.FrameGCRS.String():         "GCRS",
		eph.FrameTEME.String():         "TEME",
		eph.FrameITRS.String():         "ITRS",
		eph.CenterUnspecified.String(): "unspecified",
		eph.CenterGeocentre.String():   "geocentre",
		eph.CenterBarycentre.String():  "barycentre",
		eph.CenterHeliocentre.String(): "heliocentre",
	} {
		if got != want {
			t.Errorf("String() = %q, want %q", got, want)
		}
	}
}
