package sgp4

import (
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/vector"
)

// keplerMaxIterations and keplerTolerance are the reference's own limits on its
// Kepler iteration, and they are deliberately not improved.
//
// The iteration is a damped Newton step — the correction is clamped to ±0.95
// radians — and it gives up after ten passes whether or not it has converged.
// That is a known limitation of the published algorithm rather than a bug in
// this transcription: Vallado's own note reads "the following iteration needs
// better limits on corrections", and his verification suite includes satellite
// 23333 (WIND, e = 0.96) specifically because the solver stops converging past
// about 200 minutes.
//
// Replacing it with a solver that converges would stop reproducing the
// reference, which is the one thing this package must do. What this
// implementation adds instead is that it SAYS so — see [ErrKeplerNotConverged].
// The reference proceeds in silence, and that silence is the whole of the
// 23333 divergence.
const (
	keplerMaxIterations = 10
	keplerTolerance     = 1.0e-12
	keplerMaxCorrection = 0.95
)

// nearEarth evaluates the near-Earth SGP4 path at tsince minutes from epoch.
//
// It returns the TEME position in km and velocity in km/s. A non-nil error with
// a populated state means the state exists and should not be trusted; see the
// error sentinels.
//
// Pure: every quantity below is a local, so a *Propagator is safe to evaluate
// from any number of goroutines at once.
func (p *Propagator) nearEarth(tsince float64) (pos, vel vector.Vec3, err error) {
	g := p.grav
	vkmpersec := g.radiusKM * g.xke / 60.0

	// ---- update for secular gravity and atmospheric drag ----
	xmdf := p.el.MeanAnomaly.Radians() + p.mdot*tsince
	argpdf := p.el.ArgPerigee.Radians() + p.argpdot*tsince
	nodedf := p.el.RAAN.Radians() + p.nodedot*tsince

	argpm := argpdf
	mm := xmdf
	t2 := tsince * tsince
	nodem := nodedf + p.nodecf*t2

	tempa := 1.0 - p.cc1*tsince
	tempe := p.el.BStar * p.cc4 * tsince
	templ := p.t2cof * t2

	if !p.isimp {
		delomg := p.omgcof * tsince

		delmtemp := 1.0 + p.eta*math.Cos(xmdf)
		delm := p.xmcof * (delmtemp*delmtemp*delmtemp - p.delmo)

		temp := delomg + delm
		mm = xmdf + temp
		argpm = argpdf - temp

		t3 := t2 * tsince
		t4 := t3 * tsince

		tempa = tempa - p.d2*t2 - p.d3*t3 - p.d4*t4
		tempe += p.el.BStar * p.cc5 * (math.Sin(mm) - p.sinmao)
		templ = templ + p.t3cof*t3 + t4*(p.t4cof+tsince*p.t5cof)
	}

	nm := p.noUnkozai
	em := p.el.Eccentricity
	inclm := p.el.Inclination.Radians()

	if nm <= 0.0 {
		return vector.Vec3{}, vector.Vec3{},
			fmt.Errorf("%w: %g radians per minute at tsince %g", ErrMeanMotion, nm, tsince)
	}

	am := math.Pow(g.xke/nm, twoThirds) * tempa * tempa
	nm = g.xke / math.Pow(am, 1.5)
	em -= tempe

	// The reference's own tolerance, with its note: am is already fixed from
	// the nm check above, so the commented-out `|| am < 0.95` is not needed.
	if em >= 1.0 || em < -0.001 {
		return vector.Vec3{}, vector.Vec3{},
			fmt.Errorf("%w: mean eccentricity %g is outside [0, 1) at tsince %g",
				ErrEccentricity, em, tsince)
	}

	// Not a validation bound: a floor that keeps a circular orbit from dividing
	// by zero further down.
	if em < 1.0e-6 {
		em = 1.0e-6
	}

	mm += p.noUnkozai * templ
	xlm := mm + argpm + nodem

	// math.Mod is C's fmod — the sign follows the dividend — which is what the
	// reference uses for all four of these.
	nodem = math.Mod(nodem, twoPi)
	argpm = math.Mod(argpm, twoPi)
	xlm = math.Mod(xlm, twoPi)
	mm = math.Mod(xlm-argpm-nodem, twoPi)

	// ---- long period periodics ----
	ep := em
	xincp := inclm
	argpp := argpm
	nodep := nodem
	mp := mm
	sinip := math.Sin(inclm)
	cosip := math.Cos(inclm)

	axnl := ep * math.Cos(argpp)
	temp := 1.0 / (am * (1.0 - ep*ep))
	aynl := ep*math.Sin(argpp) + temp*p.aycof
	xl := mp + argpp + nodep + temp*p.xlcof*axnl

	// ---- solve kepler's equation ----
	u := math.Mod(xl-nodep, twoPi)

	eo1 := u
	tem5 := 9999.9
	converged := false

	for ktr := 1; ktr <= keplerMaxIterations; ktr++ {
		sineo1 := math.Sin(eo1)
		coseo1 := math.Cos(eo1)

		tem5 = 1.0 - coseo1*axnl - sineo1*aynl
		tem5 = (u - aynl*coseo1 + axnl*sineo1 - eo1) / tem5

		if math.Abs(tem5) >= keplerMaxCorrection {
			tem5 = math.Copysign(keplerMaxCorrection, tem5)
		}

		eo1 += tem5

		if math.Abs(tem5) < keplerTolerance {
			converged = true

			break
		}
	}

	sineo1 := math.Sin(eo1)
	coseo1 := math.Cos(eo1)

	// ---- short period preliminary quantities ----
	ecose := axnl*coseo1 + aynl*sineo1
	esine := axnl*sineo1 - aynl*coseo1
	el2 := axnl*axnl + aynl*aynl
	pl := am * (1.0 - el2)

	if pl < 0.0 {
		return vector.Vec3{}, vector.Vec3{},
			fmt.Errorf("%w: %g at tsince %g", ErrSemiLatusRectum, pl, tsince)
	}

	rl := am * (1.0 - ecose)
	rdotl := math.Sqrt(am) * esine / rl
	rvdotl := math.Sqrt(pl) / rl
	betal := math.Sqrt(1.0 - el2)
	temp = esine / (1.0 + betal)
	sinu := am / rl * (sineo1 - aynl - axnl*temp)
	cosu := am / rl * (coseo1 - axnl + aynl*temp)
	su := math.Atan2(sinu, cosu)
	sin2u := (cosu + cosu) * sinu
	cos2u := 1.0 - 2.0*sinu*sinu
	temp = 1.0 / pl
	temp1 := 0.5 * g.j2 * temp
	temp2 := temp1 * temp

	// ---- update for short period periodics ----
	mrt := rl*(1.0-1.5*temp2*betal*p.con41) + 0.5*temp1*p.x1mth2*cos2u
	su -= 0.25 * temp2 * p.x7thm1 * sin2u
	xnode := nodep + 1.5*temp2*cosip*sin2u
	xinc := xincp + 1.5*temp2*cosip*sinip*cos2u
	mvt := rdotl - nm*temp1*p.x1mth2*sin2u/g.xke
	rvdot := rvdotl + nm*temp1*(p.x1mth2*cos2u+1.5*p.con41)/g.xke

	// ---- orientation vectors ----
	sinsu := math.Sin(su)
	cossu := math.Cos(su)
	snod := math.Sin(xnode)
	cnod := math.Cos(xnode)
	sini := math.Sin(xinc)
	cosi := math.Cos(xinc)
	xmx := -snod * cosi
	xmy := cnod * cosi
	ux := xmx*sinsu + cnod*cossu
	uy := xmy*sinsu + snod*cossu
	uz := sini * sinsu
	vx := xmx*cossu - cnod*sinsu
	vy := xmy*cossu - snod*sinsu
	vz := sini * cossu

	mr := mrt * g.radiusKM

	pos = vector.V3(mr*ux, mr*uy, mr*uz)
	vel = vector.V3(
		(mvt*ux+rvdot*vx)*vkmpersec,
		(mvt*uy+rvdot*vy)*vkmpersec,
		(mvt*uz+rvdot*vz)*vkmpersec,
	)

	// Both of the conditions below leave the state populated, because both
	// describe a result that exists and should not be trusted — which is a
	// different thing from a computation that could not be performed.
	if mrt < 1.0 {
		return pos, vel, fmt.Errorf(
			"%w: geocentric distance is %g Earth radii at tsince %g, which is below the surface",
			ErrDecayed, mrt, tsince)
	}

	if !converged {
		return pos, vel, fmt.Errorf(
			"%w: %d iterations left a correction of %g radians at tsince %g",
			ErrKeplerNotConverged, keplerMaxIterations, tem5, tsince)
	}

	return pos, vel, nil
}
