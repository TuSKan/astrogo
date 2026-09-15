package sgp4

import "math"

// deepElements are the six mean elements the deep-space corrections act on.
//
// Passed and returned by value rather than mutated through a record, which is
// what makes [Propagator.At] safe to call concurrently. The reference threads
// them through satrec; keeping them here is the single structural difference
// between this transcription and its source.
type deepElements struct {
	em, argpm, inclm, mm, nodem, nm float64
}

// dspace applies the deep-space secular and resonance contributions at t
// minutes from epoch.
//
// # The resonance integrator, and why restarting it is free
//
// For a resonant orbit this runs a fixed-step Euler–Maclaurin integration from
// t = 0 out to t, in 720-minute steps. Vallado's version carries atime, xli and
// xni between calls so a forward march does not redo its steps — his own note
// reads "sgp4fix take out atime = 0.0 and fix for faster operation" — and that
// carried state is exactly what makes the reference unsafe to call from two
// goroutines at once.
//
// It can be dropped without changing a single bit, because the step grid is
// anchored at zero and the step size is fixed: the state at any atime on the
// grid is the same whether it was reached in one call or twenty. The carried
// value is a memo, not a path dependency. The cost of recomputing is |t|/720
// steps — two for a day, 730 for a year, each a handful of trigonometric calls.
//
// That trade is worth naming because it is the only place where this package
// chooses differently from its reference on performance grounds, and it chooses
// the slower option.
func (p *Propagator) dspace(t float64, in deepElements) (out deepElements, dndt float64) {
	out = in

	// The secular lunisolar rates, applied directly.
	theta := math.Mod(p.gsto+t*rptim, twoPi)

	out.em += p.rz.dedt * t
	out.inclm += p.rz.didt * t
	out.argpm += p.rz.domdt * t
	out.nodem += p.rz.dnodt * t
	out.mm += p.rz.dmdt * t

	// The reference has a commented-out negative-inclination fixup here, with
	// "the following if statement should be commented out". Left out.

	if p.rz.irez == resonanceNone {
		return out, 0.0
	}

	no := p.noUnkozai

	// Restart from the grid origin. See the doc comment: this is the memo the
	// reference keeps and this one does not.
	atime := 0.0
	xni := no
	xli := p.rz.xlamo

	delt := resonanceStep
	if t <= 0.0 {
		delt = -resonanceStep
	}

	var (
		ft          float64
		xndt, xnddt float64
		xldot       float64
	)

	for {
		xndt, xldot, xnddt = p.resonanceDerivatives(xli, xni, atime)

		if math.Abs(t-atime) < resonanceStep {
			ft = t - atime

			break
		}

		xli += xldot*delt + xndt*resonanceHalfStep
		xni += xndt*delt + xnddt*resonanceHalfStep
		atime += delt
	}

	nm := xni + xndt*ft + xnddt*ft*ft*0.5
	xl := xli + xldot*ft + xndt*ft*ft*0.5

	if p.rz.irez != resonanceSynchronous {
		out.mm = xl - 2.0*out.nodem + 2.0*theta
	} else {
		out.mm = xl - out.nodem - out.argpm + theta
	}

	dndt = nm - no
	out.nm = no + dndt

	return out, dndt
}

// resonanceDerivatives returns the first and second time derivatives of the
// resonance variable, and the rate of the mean longitude.
//
// Split out of dspace's loop because the two resonance classes share the loop
// and nothing else, and because a 30-line expression inside a loop inside a
// branch is where a transcription error hides.
func (p *Propagator) resonanceDerivatives(xli, xni, atime float64) (xndt, xldot, xnddt float64) {
	rz := &p.rz

	if rz.irez != resonanceHalfDay {
		// ---- near-synchronous resonance ----
		xndt = rz.del1*math.Sin(xli-fasx2) +
			rz.del2*math.Sin(2.0*(xli-fasx4)) +
			rz.del3*math.Sin(3.0*(xli-fasx6))
		xldot = xni + rz.xfact
		xnddt = rz.del1*math.Cos(xli-fasx2) +
			2.0*rz.del2*math.Cos(2.0*(xli-fasx4)) +
			3.0*rz.del3*math.Cos(3.0*(xli-fasx6))

		return xndt, xldot, xnddt * xldot
	}

	// ---- near-half-day resonance ----
	xomi := p.el.ArgPerigee.Radians() + p.argpdot*atime
	x2omi := xomi + xomi
	x2li := xli + xli

	xndt = rz.d2201*math.Sin(x2omi+xli-g22) + rz.d2211*math.Sin(xli-g22) +
		rz.d3210*math.Sin(xomi+xli-g32) + rz.d3222*math.Sin(-xomi+xli-g32) +
		rz.d4410*math.Sin(x2omi+x2li-g44) + rz.d4422*math.Sin(x2li-g44) +
		rz.d5220*math.Sin(xomi+xli-g52) + rz.d5232*math.Sin(-xomi+xli-g52) +
		rz.d5421*math.Sin(xomi+x2li-g54) + rz.d5433*math.Sin(-xomi+x2li-g54)

	xldot = xni + rz.xfact

	xnddt = rz.d2201*math.Cos(x2omi+xli-g22) + rz.d2211*math.Cos(xli-g22) +
		rz.d3210*math.Cos(xomi+xli-g32) + rz.d3222*math.Cos(-xomi+xli-g32) +
		rz.d5220*math.Cos(xomi+xli-g52) + rz.d5232*math.Cos(-xomi+xli-g52) +
		2.0*(rz.d4410*math.Cos(x2omi+x2li-g44)+
			rz.d4422*math.Cos(x2li-g44)+
			rz.d5421*math.Cos(xomi+x2li-g54)+
			rz.d5433*math.Cos(-xomi+x2li-g54))

	return xndt, xldot, xnddt * xldot
}

// perturbed carries the five osculating-ish elements dpper adjusts.
type perturbed struct {
	ep, xincp, nodep, argpp, mp float64
}

// dpper applies the lunisolar long-period periodics at t minutes from epoch.
//
// # Two formulations, and the discontinuity between them
//
// Above 11.46 degrees of inclination the corrections are applied directly.
// Below it, sin(i) is small enough that dividing the node correction by it
// amplifies noise, so the Lyddane formulation works in the equinoctial
// components instead.
//
// The reference is candid that neither the threshold nor the choice of which
// inclination to test is forced: STRN3 tested the original inclination, GSFC
// the perturbed one, and Vallado's own comment says the 0.2 rad limit and the
// resulting discontinuity "probably" want readjusting. This follows his code —
// the perturbed inclination — because that is what generated the reference
// states. Changing it would be a different model, not a better one.
func (p *Propagator) dpper(t float64, in perturbed) perturbed {
	out := in
	ds := &p.ds

	// ---- the time-varying periodics ----
	zm := ds.zmos + zns*t
	zf := zm + 2.0*zes*math.Sin(zm)
	sinzf := math.Sin(zf)
	f2 := 0.5*sinzf*sinzf - 0.25
	f3 := -0.5 * sinzf * math.Cos(zf)

	ses := ds.se2*f2 + ds.se3*f3
	sis := ds.si2*f2 + ds.si3*f3
	sls := ds.sl2*f2 + ds.sl3*f3 + ds.sl4*sinzf
	sghs := ds.sgh2*f2 + ds.sgh3*f3 + ds.sgh4*sinzf
	shs := ds.sh2*f2 + ds.sh3*f3

	zm = ds.zmol + znl*t
	zf = zm + 2.0*zel*math.Sin(zm)
	sinzf = math.Sin(zf)
	f2 = 0.5*sinzf*sinzf - 0.25
	f3 = -0.5 * sinzf * math.Cos(zf)

	sel := ds.ee2*f2 + ds.e3*f3
	sil := ds.xi2*f2 + ds.xi3*f3
	sll := ds.xl2*f2 + ds.xl3*f3 + ds.xl4*sinzf
	sghl := ds.xgh2*f2 + ds.xgh3*f3 + ds.xgh4*sinzf
	shll := ds.xh2*f2 + ds.xh3*f3

	pe := ses + sel
	pinc := sis + sil
	pl := sls + sll
	pgh := sghs + sghl
	ph := shs + shll

	// The reference subtracts peo, pinco, plo, pgho and pho here. All five are
	// set to zero by dscom and never assigned again, so the subtraction is a
	// no-op; they exist for a calling convention this package does not use.

	out.xincp += pinc
	out.ep += pe

	sinip := math.Sin(out.xincp)
	cosip := math.Cos(out.xincp)

	if out.xincp >= lyddaneInclination {
		ph /= sinip
		pgh -= cosip * ph
		out.argpp += pgh
		out.nodep += ph
		out.mp += pl

		return out
	}

	// ---- the Lyddane modification ----
	sinop := math.Sin(out.nodep)
	cosop := math.Cos(out.nodep)

	alfdp := sinip * sinop
	betdp := sinip * cosop

	dalf := ph*cosop + pinc*cosip*sinop
	dbet := -ph*sinop + pinc*cosip*cosop

	alfdp += dalf
	betdp += dbet

	out.nodep = math.Mod(out.nodep, twoPi)

	// AFSPC's original code used intrinsics that returned a positive angle
	// here, and the node is used below without a trigonometric function in
	// front of it, so the difference survives into the result.
	if out.nodep < 0.0 && p.mode == ModeAFSPC {
		out.nodep += twoPi
	}

	xls := out.mp + out.argpp + pl + pgh + (cosip-pinc*sinip)*out.nodep

	xnoh := out.nodep
	out.nodep = math.Atan2(alfdp, betdp)

	if out.nodep < 0.0 && p.mode == ModeAFSPC {
		out.nodep += twoPi
	}

	// Keep the node on the same branch it was on before the atan2, so a
	// wraparound does not appear as a half-turn jump.
	if math.Abs(xnoh-out.nodep) > math.Pi {
		if out.nodep < xnoh {
			out.nodep += twoPi
		} else {
			out.nodep -= twoPi
		}
	}

	out.mp += pl
	out.argpp = xls - out.mp - cosip*out.nodep

	return out
}
