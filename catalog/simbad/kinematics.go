package simbad

import (
	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/internal/finite"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
	"github.com/TuSKan/astrogo/vector"
)

const (
	// gaiaEpochTolerance is how close, in radians, SIMBAD's move of the
	// position found at Gaia's epoch must come to SIMBAD's own position:
	// 0.2 µas, a hundredth of the smallest error Gaia publishes.
	gaiaEpochTolerance = 1e-12

	// gaiaEpochIterations bounds the search. Each step shrinks the residual
	// by about the star's motion between the epochs, in radians, times the
	// tangent of its declination: 6.5e-5 for Barnard's star and 3.3e-4 for
	// 61 Cyg A, so four steps reach the tolerance from a residual of
	// arcminutes. Eight reach it from there while the ratio stays below
	// about 0.07.
	gaiaEpochIterations = 8
)

// readKinematics fills in what a row says about the motion of t, whose
// position it already holds: the epoch of that position, the proper motion
// and parallax, which SIMBAD gives in mas, the radial velocity, in km/s, and
// the astrometric error.
//
// SIMBAD's positions are at J2000.0. A position it took from Gaia is reported
// at Gaia's epoch instead, where Gaia measured it; see atGaiaEpoch.
func readKinematics(t *resolve.Target, row []string, colIdx map[string]int) {
	value := func(name string) (float64, bool) {
		i, ok := colIdx[name]
		if !ok || i >= len(row) || row[i] == "" {
			return 0, false
		}

		return finite.Parse(row[i])
	}

	t.Epoch = time.J2000()

	if v, ok := value("pmra"); ok {
		t.PmRA = angle.Arcsec(v / 1000)
	}

	if v, ok := value("pmdec"); ok {
		t.PmDec = angle.Arcsec(v / 1000)
	}

	if v, ok := value("plx_value"); ok {
		t.Parallax = angle.Arcsec(v / 1000)
	}

	if v, ok := value("rvz_radvel"); ok {
		t.RadialVelocity = unit.KmPerSec(v)
		t.HasRadialVelocity = true
	}

	t.AstrometricError = astrometricError(row, colIdx)

	if i, ok := colIdx["coo_bibcode"]; ok && i < len(row) {
		if epoch, ok := gaiaCoordinateEpochs[row[i]]; ok {
			atGaiaEpoch(t, epoch)
		}
	}
}

// atGaiaEpoch reports t, a star whose position SIMBAD took from Gaia, at
// Gaia's epoch.
//
// SIMBAD moves Gaia's position to J2000.0 with Gaia's proper motion and
// parallax but no radial velocity, and keeps the proper motion as Gaia
// publishes it, at Gaia's epoch. Read as a star at J2000.0 moving with that
// proper motion, the star was moved again from J2000.0, radial velocity
// included, and landed where Gaia never saw it: Barnard's star 164 mas from
// its Gaia DR3 row at J2016.0, the radial-velocity term SIMBAD left out, and
// 61 Cyg A 34 mas, most of it a 2016 proper motion applied from 2000 (#1248).
//
// The position at Gaia's epoch is the one that SIMBAD's move takes to t's.
// That move is a displacement of at most a few arcminutes, and it barely
// changes across the arcseconds of a correction, so stepping by the residual
// converges within a few steps. It changes faster near a pole: the proper
// motion is given along the local RA and Dec directions, which turn as a
// guess moves there. A star whose motion between the epochs carries it near
// a pole is not reached, and like any target the search fails for, it stays
// at J2000.0, as SIMBAD gives it.
func atGaiaEpoch(t *resolve.Target, epoch time.Time) {
	want := vector.FromSpherical(t.Coord.RA().Radians(), t.Coord.Dec().Radians())
	guess := want

	for range gaiaEpochIterations {
		lon, lat := guess.ToSpherical()

		atEpoch := coord.NewICRSWithKinematics(angle.Rad(lon), angle.Rad(lat),
			t.PmRA, t.PmDec, t.Parallax, unit.KmPerSec(0), epoch)

		moved, err := coord.PropagateEpoch(atEpoch, time.J2000())
		if err != nil {
			return
		}

		residual := want.Sub(vector.FromSpherical(moved.RA().Radians(), moved.Dec().Radians()))
		if residual.Norm() < gaiaEpochTolerance {
			t.Coord = coord.NewICRS(angle.Rad(lon), angle.Rad(lat))
			t.Epoch = epoch

			return
		}

		guess = guess.Add(residual).Unit()
	}
}
