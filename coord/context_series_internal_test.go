package coord

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/time"
)

// TestNewContextKeepsItsMatrix holds the reuse in NewContext to the matrix it
// replaces: the celestial-to-intermediate matrix a Context holds must be
// C2i06a at t.TT(), bit for bit, which is what NewContext computed before it
// learned to take Apco13's (#473).
//
// From 1972 on Apco13 built that matrix at the same TT and the reuse changes
// nothing. Before 1972 Apco13's TT, from SOFA's leap-second table, differs
// from t.TT(), which follows ΔT: reused there unconditionally, the matrix moves
// by up to 2.4e-10 rad and with it every ObsVec and GeocentricToObserved. The
// epochs below cross that boundary, and every leap-second day, at both midday
// and half an hour before the extra second.
func TestNewContextKeepsItsMatrix(t *testing.T) {
	t.Parallel()

	loc, err := NewGeodetic(angle.Deg(-70.4), angle.Deg(-24.6), 2635)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	atm := atmosphere.AtAltitude(2635)

	var epochs []time.Time

	// 1900 to 2100 in steps of 37.3 days, so every phase of the year and of
	// the day is visited.
	for jd := 2415020.5; jd < 2488069.5; jd += 37.3 {
		epochs = append(epochs, time.FromJD(jd, time.UTC))
	}

	for _, ym := range [][2]int{
		{1972, 6}, {1972, 12}, {1973, 12}, {1974, 12}, {1975, 12}, {1976, 12}, {1977, 12},
		{1978, 12}, {1979, 12}, {1981, 6}, {1982, 6}, {1983, 6}, {1985, 6}, {1987, 12},
		{1989, 12}, {1990, 12}, {1992, 6}, {1993, 6}, {1994, 6}, {1995, 12}, {1997, 6},
		{1998, 12}, {2005, 12}, {2008, 12}, {2012, 6}, {2015, 6}, {2016, 12},
	} {
		day := 30
		if ym[1] == 12 {
			day = 31
		}

		for _, hour := range []int{12, 23} {
			epochs = append(epochs, time.Date(ym[0], time.Month(ym[1]), day, hour, 30, 0, 0, time.LocationUTC))
		}
	}

	for _, epoch := range epochs {
		ctx := NewContext(epoch, loc, atm)

		tt1, tt2 := epoch.UTC().TT().JDParts()
		if want := gofaext.C2i06a(tt1, tt2); ctx.rc2i != want {
			t.Fatalf("%v: NewContext's celestial-to-intermediate matrix is not C2i06a at t.TT():\n"+
				"  got  %v\n  want %v", epoch, ctx.rc2i, want)
		}
	}
}
