package ephemeris_test

import (
	"testing"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
)

// TestProvidersNameTheirCenterAndAxes is the point of the Frame and Center
// fields.
//
// Before them, the contract on State was a comment reading "Geocentric
// position in AU (ICRS-like)" — hedged, on a struct several providers fill in
// differently. A provider now locates a body about the center it computes it
// from and says which, and an Instant composes those, so a label that lied
// would put a body an AU from where it is. The analytical provider computes
// the planets about the Sun and the Moon about the Earth; a state composed
// from them is about the center it was asked for.
func TestProvidersNameTheirCenterAndAxes(t *testing.T) {
	t.Parallel()

	when := time.Date(2026, 1, 1, 0, 0, 0, 0, time.LocationUTC)
	p := eph.Default()

	for _, c := range []struct {
		id, center eph.ID
	}{
		{eph.MarsBarycenter, eph.Sun},
		{eph.Earth, eph.Sun},
		{eph.Moon, eph.Earth},
		{eph.Sun, eph.SolarSystemBarycenter},
	} {
		st, err := p.Locate(c.id, when)
		if err != nil {
			t.Fatalf("Locate(%v): %v", c.id, err)
		}

		if st.Frame != eph.FrameICRS || st.Center != c.center {
			t.Errorf("the analytical provider locates %v about %v on %s axes, want about %v on the ICRS's", c.id, st.Center, st.Frame, c.center)
		}
	}

	st, err := eph.At(p, when).Geometric(eph.MarsBarycenter, eph.Earth)
	if err != nil {
		t.Fatal(err)
	}

	if st.Frame != eph.FrameICRS || st.Center != eph.Earth {
		t.Errorf("Mars composed about the Earth is about %v on %s axes", st.Center, st.Frame)
	}
}

// The String forms appear in the errors the Instant reports for axes it cannot
// compose, so they have to read.
func TestFrameStrings(t *testing.T) {
	t.Parallel()

	for got, want := range map[string]string{
		eph.FrameUnspecified.String(): "unspecified",
		eph.FrameICRS.String():        "ICRS",
		eph.FrameGCRS.String():        "GCRS",
		eph.FrameTEME.String():        "TEME",
		eph.FrameITRS.String():        "ITRS",
	} {
		if got != want {
			t.Errorf("String() = %q, want %q", got, want)
		}
	}
}
