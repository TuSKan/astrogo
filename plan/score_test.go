package plan

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/constraint"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/earth"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/observatory"
	"github.com/TuSKan/astrogo/target"
	"github.com/TuSKan/astrogo/time"
)

func TestScoreObservable(t *testing.T) {
	loc, _ := earth.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	site, _ := observatory.NewSite("Test", loc, angle.Zero(), nil)
	tm := time.FromJD(2451545.0, time.UTC) // J2000 Noon (LST ~18.69h)

	t.Run("AltitudeScoring", func(t *testing.T) {
		// Target 1: Near zenith (Alt ~90)
		obj1 := target.Custom{Coord: coord.ICRS{RA: angle.Hour(18.69), Dec: angle.Deg(0)}}
		// Target 2: Lower (Alt ~45)
		obj2 := target.Custom{Coord: coord.ICRS{RA: angle.Hour(18.69), Dec: angle.Deg(45)}}
		
		s1, _ := ScoreObservable(obj1, tm, site)
		s2, _ := ScoreObservable(obj2, tm, site)
		
		if s1 <= s2 {
			t.Errorf("Expected higher altitude to have higher score: %f <= %f", s1, s2)
		}
	})

	t.Run("FailingConstraint", func(t *testing.T) {
		obj := target.Custom{Coord: coord.ICRS{RA: angle.Hour(18.69), Dec: angle.Deg(0)}}
		// Force fail with extreme altitude threshold
		c := constraint.Altitude{Threshold: angle.Deg(95)}
		
		s, err := ScoreObservable(obj, tm, site, c)
		testutil.AssertNoError(t, err)
		if s != 0 {
			t.Errorf("Expected score 0 for failing constraint, got %f", s)
		}
	})
}

type prioritizedTarget struct {
	target.Observable
	priority float64
}

func (p prioritizedTarget) Priority() float64 { return p.priority }

func TestRankObservables(t *testing.T) {
	loc, _ := earth.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	site, _ := observatory.NewSite("Test", loc, angle.Zero(), nil)
	tm := time.FromJD(2451545.0, time.UTC)

	t.Run("RankingStability", func(t *testing.T) {
		objs := []target.Observable{
			prioritizedTarget{
				Observable: target.Custom{Coord: coord.ICRS{RA: angle.Hour(18.69), Dec: angle.Deg(45)}},
				priority:   2.0, // High priority but lower altitude
			},
			target.Custom{Coord: coord.ICRS{RA: angle.Hour(18.69), Dec: angle.Deg(0)}}, // Zenith but priority 1.0
		}
		
		// Score 1: ~45 * 2.0 = 90
		// Score 2: ~90 * 1.0 = 90
		// (Actually depends on exact math, let's adjust to be sure)
		
		objs[0] = prioritizedTarget{
			Observable: target.Custom{Coord: coord.ICRS{RA: angle.Hour(18.69), Dec: angle.Deg(45)}},
			priority:   3.0, // Score ~135
		}
		
		ranked, err := RankObservables(objs, tm, site)
		testutil.AssertNoError(t, err)
		
		if len(ranked) != 2 {
			t.Errorf("Expected 2 ranked targets, got %d", len(ranked))
		}
		if ranked[0].Object.Name() != objs[0].Name() {
			t.Error("Priority should have pushed lower altitude target to first place")
		}
	})
}
