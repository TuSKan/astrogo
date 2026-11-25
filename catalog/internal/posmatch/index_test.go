package posmatch

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/epoch"
	"github.com/TuSKan/astrogo/unit"
)

// pairSet is CandidatePairs's answer as a set.
func pairSet(pairs [][2]int) map[[2]int]bool {
	set := make(map[[2]int]bool, len(pairs))
	for _, p := range pairs {
		set[p] = true
	}

	return set
}

// epochs a random target is given at: none, J2000.0, Gaia DR2's and DR3's,
// Hipparcos's, and 2MASS-like observation dates.
var randomEpochs = []epoch.Time{
	{}, epoch.J2000(),
	epoch.FromJD(2457206.375, epoch.TT), epoch.FromJD(2457389.0, epoch.TT),
	epoch.FromJD(2448349.0625, epoch.TT),
	epoch.FromJD(2450500.5, epoch.TT), epoch.FromJD(2451800.5, epoch.TT),
}

// randomTarget is a target near (raDeg, decDeg): moving or still, with or
// without errors, at one of randomEpochs.
func randomTarget(rng *rand.Rand, raDeg, decDeg float64) resolve.Target {
	t := resolve.Target{
		HasCoord: true,
		Coord: coord.NewICRS(angle.Deg(raDeg+(rng.Float64()-0.5)*40.0/3600),
			angle.Deg(decDeg+(rng.Float64()-0.5)*40.0/3600)),
		Epoch: randomEpochs[rng.IntN(len(randomEpochs))],
	}

	if rng.Float64() < 0.5 {
		speed := 3.0
		if rng.Float64() < 0.1 {
			speed = 10
		}

		t.PmRA = angle.Arcsec((rng.Float64()*2 - 1) * speed)
		t.PmDec = angle.Arcsec((rng.Float64()*2 - 1) * speed)
		t.Parallax = angle.Arcsec(rng.Float64() * 0.5)

		if rng.Float64() < 0.5 {
			t.RadialVelocity = unit.KmPerSec((rng.Float64()*2 - 1) * 100)
		}
	}

	if rng.Float64() < 0.6 {
		sigma := math.Pow(10, rng.Float64()*5-2) // 0.01 to 1000 mas
		t.AstrometricError = resolve.AstrometricError{
			RA: mas(sigma * (0.5 + rng.Float64())), Dec: mas(sigma * (0.5 + rng.Float64())),
			RADec: (rng.Float64()*2 - 1) * 0.6,
		}

		if rng.Float64() < 0.7 {
			pmSigma := math.Pow(10, rng.Float64()*3-2) // 0.01 to 10 mas/yr
			t.AstrometricError.PmRA, t.AstrometricError.PmDec = mas(pmSigma), mas(pmSigma*(0.5+rng.Float64()))
			t.AstrometricError.DecPmDec = (rng.Float64()*2 - 1) * 0.6
			t.AstrometricError.RAPmRA = (rng.Float64()*2 - 1) * 0.6
		}

		if rng.Float64() < 0.3 {
			t.AstrometricError.Epoch = randomEpochs[2+rng.IntN(len(randomEpochs)-2)]
		}
	}

	return t
}

// seenBy is a still row that saw t where t was at rowEpoch, a few mas off: the
// pair Compare compares at the row's epoch, away from where t is indexed.
func seenBy(rng *rand.Rand, t resolve.Target, rowEpoch epoch.Time) (resolve.Target, bool) {
	at, err := coord.PropagateEpoch(Coord(t), rowEpoch)
	if err != nil {
		return resolve.Target{}, false
	}

	row := resolve.Target{
		HasCoord: true, Epoch: rowEpoch,
		Coord: coord.NewICRS(at.RA().Add(mas(rng.Float64()*5)), at.Dec().Add(mas(rng.Float64()*5))),
	}

	// Half of them publish errors of a few mas, so that D², not a
	// resolution, decides the pair, at the row's epoch.
	if rng.Float64() < 0.5 {
		sigma := 1 + rng.Float64()*20
		row.AstrometricError = resolve.AstrometricError{RA: mas(sigma), Dec: mas(sigma)}
	}

	return row, true
}

// TestCandidatePairsHoldEveryCompatiblePair: over random catalogs crowded
// into a 40″ field, mixing moving and still targets, errors and none, and
// epochs from 1991 to 2016, every pair Compare finds compatible is among the
// candidates, at no resolution, a fine one and a coarse one. Each moving
// target of one side is also seen by a still row of the other at that row's
// own epoch.
func TestCandidatePairsHoldEveryCompatiblePair(t *testing.T) {
	t.Parallel()

	for seed := range uint64(8) {
		rng := rand.New(rand.NewPCG(seed, 1183))

		a, b := make([]resolve.Target, 0, 120), make([]resolve.Target, 0, 180)
		for range 120 {
			a = append(a, randomTarget(rng, 150, 20))
			b = append(b, randomTarget(rng, 150, 20))
		}

		for _, star := range a[:60] {
			if star.PmRA == 0 && star.PmDec == 0 {
				continue
			}

			if row, ok := seenBy(rng, star, randomEpochs[3+rng.IntN(len(randomEpochs)-3)]); ok {
				b = append(b, row)
			}
		}

		for _, res := range []angle.Angle{0, angle.Arcsec(0.3), angle.Arcsec(3)} {
			cands := pairSet(CandidatePairs(a, b, res))
			compatible := 0

			for i := range a {
				for j := range b {
					if _, ok, err := Compare(a[i], b[j], res); err != nil || !ok {
						continue
					}

					compatible++

					if !cands[[2]int{i, j}] {
						t.Errorf("seed %d, resolution %v: compatible pair (%d, %d) is no candidate: %+v and %+v",
							seed, res, i, j, a[i], b[j])
					}
				}
			}

			t.Logf("seed %d, resolution %v: %d compatible pairs among %d candidates, of %d", seed, res, compatible, len(cands), len(a)*len(b))
		}
	}
}

// TestCandidatePairsFindAMovingStarWhereTheRowSawIt: Barnard's star from
// Gaia DR3 is indexed at J2000.0, 166″ from where a still row observed at
// J2016.0 saw it. The pair is compared at the row's epoch, so it is a
// candidate within a 1″ resolution.
func TestCandidatePairsFindAMovingStarWhereTheRowSawIt(t *testing.T) {
	t.Parallel()

	j2016 := epoch.FromJD(2457389.0, epoch.TT)
	barnard := resolve.Target{
		HasCoord: true, Epoch: j2016,
		Coord: coord.NewICRS(angle.Deg(269.44850252543836), angle.Deg(4.739420051112412)),
		PmRA:  mas(-801.5509783684709), PmDec: mas(10362.394206546573), Parallax: mas(546.9759),
	}
	row := resolve.Target{HasCoord: true, Epoch: j2016, Coord: barnard.Coord}

	if _, ok, err := Compare(barnard, row, angle.Arcsec(1)); err != nil || !ok {
		t.Fatalf("test setup: Compare = %v, %v; want compatible", ok, err)
	}

	for _, pairs := range [][][2]int{
		CandidatePairs([]resolve.Target{barnard}, []resolve.Target{row}, angle.Arcsec(1)),
		CandidatePairs([]resolve.Target{row}, []resolve.Target{barnard}, angle.Arcsec(1)),
	} {
		if len(pairs) != 1 {
			t.Errorf("candidates %v, want the one pair", pairs)
		}
	}
}

// TestCandidatePairsReachTheErrors: with no resolution, two still targets
// 50 mas apart, each known to 30 mas, are compatible by D² = 1.39 and so are
// a candidate.
func TestCandidatePairsReachTheErrors(t *testing.T) {
	t.Parallel()

	a := still(20, 0, errorsOf(30, 0))
	b := still(20, 50, errorsOf(30, 0))

	if _, ok, err := Compare(a, b, 0); err != nil || !ok {
		t.Fatalf("test setup: Compare = %v, %v; want compatible", ok, err)
	}

	if pairs := CandidatePairs([]resolve.Target{a}, []resolve.Target{b}, 0); len(pairs) != 1 {
		t.Errorf("candidates %v, want the one pair", pairs)
	}
}

// TestCandidatePairsAreTheNearPairsOfASparseSky: two skies of 3000 still
// positions without errors, with 100 pairs planted 0.5″ apart, give at a 1″
// resolution exactly the pairs within 1″, found without comparing the nine
// million others.
func TestCandidatePairsAreTheNearPairsOfASparseSky(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewPCG(7, 1183))
	sky := func() coord.ICRS {
		return coord.NewICRS(angle.Rad(rng.Float64()*2*math.Pi), angle.Rad(math.Asin(rng.Float64()*2-1)))
	}

	a, b := make([]resolve.Target, 3000), make([]resolve.Target, 3000)
	for k := range a {
		a[k] = resolve.Target{HasCoord: true, Coord: sky()}
		b[k] = resolve.Target{HasCoord: true, Coord: sky()}
	}

	for k := range 100 {
		b[k*30].Coord = coord.NewICRS(a[k*29].Coord.RA(), a[k*29].Coord.Dec().Add(angle.Arcsec(0.5)))
	}

	want := 0

	for i := range a {
		for j := range b {
			if coord.Separation(a[i].Coord, b[j].Coord) <= angle.Arcsec(1) {
				want++
			}
		}
	}

	if got := len(CandidatePairs(a, b, angle.Arcsec(1))); got != want || want < 100 {
		t.Errorf("%d candidates, want the %d pairs within 1″ (100 planted)", got, want)
	}
}

// TestCandidatePairsCrossThePoleAndTheMeridian: pairs 0.2″ apart across
// right ascension 0h, and across the celestial pole at right ascensions 12h
// apart, are candidates within 1″.
func TestCandidatePairsCrossThePoleAndTheMeridian(t *testing.T) {
	t.Parallel()

	at := func(raDeg, decDeg float64) resolve.Target {
		return resolve.Target{HasCoord: true, Coord: coord.NewICRS(angle.Deg(raDeg), angle.Deg(decDeg))}
	}

	for _, tc := range []struct {
		name string
		a, b resolve.Target
	}{
		{"across 0h", at(359.99997, 10), at(0.00002, 10)},
		{"across the pole", at(0, 90-0.1/3600), at(180, 90-0.1/3600)},
	} {
		if pairs := CandidatePairs([]resolve.Target{tc.a}, []resolve.Target{tc.b}, angle.Arcsec(1)); len(pairs) != 1 {
			t.Errorf("%s: candidates %v, want the one pair", tc.name, pairs)
		}
	}
}
