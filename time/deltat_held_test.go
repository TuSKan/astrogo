package time_test

import (
	"errors"
	"fmt"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/time/internal/iers"
)

// fakeBulletin is a bulletin covering [first, last] MJD, with DUT1 drifting
// 0.5 ms a day and polar motion that is not zero, so a held value and a zero
// one cannot be confused. It reports its coverage, as the real one does.
type fakeBulletin struct{ first, last float64 }

func (b fakeBulletin) EOP(mjd float64) (iers.EOP, error) {
	if mjd < b.first || mjd > b.last {
		return iers.EOP{}, fmt.Errorf("%w: MJD %.1f", iers.ErrOutOfRange, mjd)
	}

	return iers.EOP{DUT1: 0.2 - 0.0005*(mjd-b.first), XP: 1e-6, YP: 2e-6}, nil
}

func (b fakeBulletin) Coverage() (mjdMin, mjdMax float64) { return b.first, b.last }

// bulletin2023 covers 2023-02-25 to 2024-03-31.
var bulletin2023 = fakeBulletin{first: 60000, last: 60400}

// withBulletin registers b for the test and keeps any cached bulletin out.
func withBulletin(t *testing.T, b time.Model) {
	t.Helper()

	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))
	t.Cleanup(func() { remote.SetDataDir("") })

	time.RegisterModel(b)
	t.Cleanup(time.ResetEOP)
}

func atMJD(mjd float64) time.Time { return time.FromJDParts(2400000.5, mjd, time.UTC) }

// labelsApart is TT − UT1 as the conversions put it between the labels of one
// UTC instant, in seconds.
func labelsApart(u time.Time) float64 {
	t1, t2 := u.TT().JDParts()
	u1, u2 := u.UT1Using(u.EOP().DUT1).JDParts()

	return ((t1 - u1) + (t2 - u2)) * 86400
}

// TestDeltaTIsWhatTheConversionsUse is #696: DeltaT used to follow the 2006
// polynomial at every epoch while the conversions used the bulletin, so one
// instant had two ΔT, 6.2 s apart in 2026. From 1960 DeltaT is now the
// difference the conversions themselves put between TT and UT1, inside the
// bulletin, before it and past it; before 1960 it is the model, which is also
// what Time.TT reads a pre-1960 label through.
func TestDeltaTIsWhatTheConversionsUse(t *testing.T) {
	withBulletin(t, bulletin2023)

	for _, mjd := range []float64{60000, 60200.3, 60400, 61000, 70000, 41000, 37500} {
		u := atMJD(mjd)
		if got, want := time.DeltaT(u), labelsApart(u); math.Abs(got-want) > 1e-6 {
			t.Errorf("MJD %.1f: DeltaT %.6f s, the conversions put %.6f s between TT and UT1", mjd, got, want)
		}
	}

	early := time.Date(1900, time.June, 1, 0, 0, 0, 0, time.LocationUTC)
	t1, t2 := early.TT().JDParts()
	u1, u2 := early.JDParts()

	if got, want := time.DeltaT(early), ((t1-u1)+(t2-u2))*86400; math.Abs(got-want) > 1e-6 {
		t.Errorf("1900: DeltaT %.6f s, Time.TT reads the label through %.6f s", got, want)
	}
}

// TestDeltaTIsHeldPastTheBulletin: past the bulletin's last day ΔT keeps its
// value on that day, where it used to fall back to DUT1 = 0, UT1 pinned to
// UTC. Time.UT1 answers there instead of failing, since the value is a stated
// forecast and every other conversion uses it; before the bulletin's start,
// nothing changed.
func TestDeltaTIsHeldPastTheBulletin(t *testing.T) {
	withBulletin(t, bulletin2023)

	end := time.DeltaT(atMJD(bulletin2023.last))

	for _, mjd := range []float64{60400.5, 61000, 70000, 88000} {
		if got := time.DeltaT(atMJD(mjd)); math.Abs(got-end) > 1e-6 {
			t.Errorf("MJD %.0f: DeltaT %.6f s, want it held at the bulletin's last %.6f s", mjd, got, end)
		}
	}

	last, _ := bulletin2023.EOP(bulletin2023.last)
	past := atMJD(70000)

	if eop := past.EOP(); eop.DUT1 != last.DUT1 || eop.XP != 0 || eop.YP != 0 {
		t.Errorf("EOP past the bulletin = %+v, want DUT1 held at %g s and zero polar motion", eop, last.DUT1)
	}

	ut1, err := past.UT1()
	if err != nil {
		t.Fatalf("UT1 past the bulletin: %v, want the held value", err)
	}

	want := past.UT1Using(last.DUT1)
	if !ut1.Equal(want) {
		t.Errorf("UT1 past the bulletin = %v, want %v", ut1, want)
	}

	if _, err := atMJD(59000).UT1(); !errors.Is(err, iers.ErrOutOfRange) {
		t.Errorf("UT1 before the bulletin's start: %v, want the out-of-range error, as before", err)
	}
}

// TestDeltaTStaysHeldAcrossALeapSecond: a leap second registered past the
// bulletin moves ΔAT by one second, and DUT1 has to move with it, as it does
// at a real one, or ΔT would jump by a second there.
func TestDeltaTStaysHeldAcrossALeapSecond(t *testing.T) {
	withBulletin(t, bulletin2023)

	defer time.ResetLeapSeconds()

	if err := time.RegisterLeapSeconds(
		extended(time.LeapSecond{Year: 2030, Month: 1, Day: 1, DeltaAT: 38}),
		"deltat-held-test",
	); err != nil {
		t.Fatalf("registering a leap second past the bulletin: %v", err)
	}

	before, after := utc(2029, time.June, 1), utc(2030, time.June, 1)

	if d := time.DeltaT(after) - time.DeltaT(before); math.Abs(d) > 1e-6 {
		t.Errorf("ΔT moved by %.6f s across a registered leap second; it is held", d)
	}

	if d := after.EOP().DUT1 - before.EOP().DUT1; math.Abs(d-1) > 1e-9 {
		t.Errorf("DUT1 moved by %.9f s across the leap second, want +1 s", d)
	}
}

// TestDeltaTUncertaintyGrowsFromTheBulletinsEnd: σ is zero through the
// measured record and grows from its end, by Huber's random walk. It used to
// start the walk at 2005 for every caller, so a ΔT the bulletin measured in
// 2023 came with a 4.7 s uncertainty.
func TestDeltaTUncertaintyGrowsFromTheBulletinsEnd(t *testing.T) {
	withBulletin(t, bulletin2023)

	for _, mjd := range []float64{39000, 60000, 60400} {
		if s := time.DeltaTUncertainty(atMJD(mjd)); s != 0 {
			t.Errorf("MJD %.0f, inside the measured record: σ = %g s, want 0", mjd, s)
		}
	}

	// Huber (2000): 365.25·N·√(N·Q/3·(1 + N/M)) ms, Q = 0.058, M = 2500.
	huber := func(n float64) float64 { return 365.25 * n * math.Sqrt(n*0.058/3*(1+n/2500)) / 1000 }

	prev := 0.0

	for _, years := range []float64{1, 10, 70} {
		got := time.DeltaTUncertainty(atMJD(bulletin2023.last + years*365.25))
		testutil.AssertNear(t, fmt.Sprintf("σ %g years past the bulletin", years), got, huber(years), 1e-9)

		if got <= prev {
			t.Errorf("σ %g years past the bulletin = %g s, not above %g s nearer it", years, got, prev)
		}

		prev = got
	}

	if s := time.DeltaTUncertainty(time.Date(1750, time.June, 15, 0, 0, 0, 0, time.LocationUTC)); s != 2 {
		t.Errorf("1750: σ = %g s, want the historical model's 2 s", s)
	}
}
