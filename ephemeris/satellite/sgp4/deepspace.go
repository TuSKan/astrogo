package sgp4

import "math"

// Deep-space constants, all of them the model's own.
//
// They are lunar and solar orbital elements, resonance frequencies and
// geopotential coefficients, fitted rather than derived. None of them is a
// tunable and none has a better modern value that belongs here — see [gmst82]
// for the same argument about sidereal time.
const (
	// Solar and lunar mean motions, radians per minute.
	zns = 1.19459e-5
	znl = 1.5835218e-4

	// Solar and lunar eccentricities.
	zes = 0.01675
	zel = 0.05490

	// The Sun's orbital plane relative to the equator, and its argument of
	// perigee, at the model's epoch.
	zsinis = 0.39785416
	zcosis = 0.91744867
	zcosgs = 0.1945905
	zsings = -0.98088458

	// Third-body scale factors.
	c1ss = 2.9864797e-6
	c1l  = 4.7968065e-7

	// Earth rotation in radians per minute — 7.29211514668855e-5 rad/s, which
	// is the reference's own note beside the number.
	rptim = 4.37526908801129966e-3

	// Geopotential resonance coefficients.
	q22    = 1.7891679e-6
	q31    = 2.1460748e-6
	q33    = 2.2123015e-7
	root22 = 1.7891679e-6
	root32 = 3.7393792e-7
	root44 = 7.3636953e-9
	root52 = 1.1428639e-7
	root54 = 2.1765803e-9

	// Resonance phase angles.
	fasx2 = 0.13130908
	fasx4 = 2.8843198
	fasx6 = 0.37448087
	g22   = 5.7686396
	g32   = 0.95240898
	g44   = 1.8014998
	g52   = 1.0508330
	g54   = 4.4108898

	// The resonance integrator's fixed step, in minutes, and half its square.
	resonanceStep     = 720.0
	resonanceHalfStep = 259200.0 // 0.5 * 720²

	// The inclination band, in radians, where the node correction is dropped
	// because sin(i) is too near zero to divide by. 5.24e-2 rad is 3 degrees.
	//
	// Vallado's own `sgp4fix for 180 deg incl`: without it a near-equatorial or
	// near-retrograde orbit divides a finite numerator by an infinitesimal.
	polarSingularityBand = 5.2359877e-2

	// lyddaneInclination is where dpper switches to the Lyddane formulation,
	// 0.2 rad = 11.46 degrees.
	//
	// The reference notes the choice is not forced: STRN3 used the original
	// inclination here and GSFC the perturbed one, both defensible, and
	// Vallado's comment says the limit itself probably wants readjusting. This
	// follows his code — the perturbed inclination — because the reference
	// states were generated that way.
	lyddaneInclination = 0.2
)

// Resonance classes. SGP4 treats two commensurabilities specially, and
// everything else not at all.
const (
	resonanceNone = 0
	// resonanceSynchronous is the one-day (geosynchronous) resonance.
	resonanceSynchronous = 1
	// resonanceHalfDay is the twelve-hour resonance, which needs an
	// eccentricity of at least 0.5 to matter — the Molniya case.
	resonanceHalfDay = 2
)

// dscomResult carries the quantities dscom computes for dsinit's use only.
//
// They are not stored on the propagator because nothing after initialisation
// reads them. Vallado threads roughly eighty values between the two procedures
// through one mutable record; grouping the forty that are actually consumed is
// the only structural liberty taken here, and the arithmetic is unchanged.
type dscomResult struct {
	cosim, sinim float64
	em, emsq     float64
	nm           float64

	s1, s2, s3, s4, s5         float64
	ss1, ss2, ss3, ss4, ss5    float64
	sz1, sz3, sz11, sz13       float64
	sz21, sz23, sz31, sz33     float64
	z1, z3, z11, z13, z21, z23 float64
	z31, z33                   float64
}

// deepSpaceTerms are the lunisolar periodic coefficients dscom produces and
// dpper reads on every evaluation.
type deepSpaceTerms struct {
	e3, ee2          float64
	se2, se3         float64
	sgh2, sgh3, sgh4 float64
	sh2, sh3         float64
	si2, si3         float64
	sl2, sl3, sl4    float64
	xgh2, xgh3, xgh4 float64
	xh2, xh3         float64
	xi2, xi3         float64
	xl2, xl3, xl4    float64
	zmol, zmos       float64
}

// resonanceTerms are the geopotential resonance coefficients dsinit produces
// and dspace reads.
type resonanceTerms struct {
	irez int

	d2201, d2211, d3210, d3222 float64
	d4410, d4422, d5220, d5232 float64
	d5421, d5433               float64

	del1, del2, del3 float64

	dedt, didt, dmdt, dnodt, domdt float64

	xfact, xlamo float64
}

// dscom computes the lunisolar contributions, averaged over one revolution of
// the Sun and of the Moon.
//
// epoch1950 is days from 0 January 1950; tc is an offset in minutes, zero at
// initialisation.
func (p *Propagator) dscom(epoch1950, tc float64) dscomResult {
	ep := p.el.Eccentricity
	argpp := p.el.ArgPerigee.Radians()
	inclp := p.el.Inclination.Radians()
	nodep := p.el.RAAN.Radians()

	var r dscomResult

	r.nm = p.noUnkozai
	r.em = ep

	snodm := math.Sin(nodep)
	cnodm := math.Cos(nodep)
	sinomm := math.Sin(argpp)
	cosomm := math.Cos(argpp)

	r.sinim = math.Sin(inclp)
	r.cosim = math.Cos(inclp)
	r.emsq = r.em * r.em

	betasq := 1.0 - r.emsq
	rtemsq := math.Sqrt(betasq)

	// ---- the Moon's orbital plane at this epoch ----
	day := epoch1950 + 18261.5 + tc/1440.0

	xnodce := math.Mod(4.5236020-9.2422029e-4*day, twoPi)
	stem := math.Sin(xnodce)
	ctem := math.Cos(xnodce)

	zcosil := 0.91375164 - 0.03568096*ctem
	zsinil := math.Sqrt(1.0 - zcosil*zcosil)
	zsinhl := 0.089683511 * stem / zsinil
	zcoshl := math.Sqrt(1.0 - zsinhl*zsinhl)

	gam := 5.8351514 + 0.0019443680*day

	zx := 0.39785416 * stem / zsinil
	zy := zcoshl*ctem + 0.91744867*zsinhl*stem
	zx = math.Atan2(zx, zy)
	zx = gam + zx - xnodce

	zcosgl := math.Cos(zx)
	zsingl := math.Sin(zx)

	// ---- the solar pass, then the lunar one ----
	zcosg := zcosgs
	zsing := zsings
	zcosi := zcosis
	zsini := zsinis
	zcosh := cnodm
	zsinh := snodm
	cc := c1ss
	xnoi := 1.0 / r.nm

	var (
		s1, s2, s3, s4, s5, s6, s7 float64
		z1, z2, z3                 float64
		z11, z12, z13              float64
		z21, z22, z23              float64
		z31, z32, z33              float64
	)

	for lsflg := 1; lsflg <= 2; lsflg++ {
		a1 := zcosg*zcosh + zsing*zcosi*zsinh
		a3 := -zsing*zcosh + zcosg*zcosi*zsinh
		a7 := -zcosg*zsinh + zsing*zcosi*zcosh
		a8 := zsing * zsini
		a9 := zsing*zsinh + zcosg*zcosi*zcosh
		a10 := zcosg * zsini
		a2 := r.cosim*a7 + r.sinim*a8
		a4 := r.cosim*a9 + r.sinim*a10
		a5 := -r.sinim*a7 + r.cosim*a8
		a6 := -r.sinim*a9 + r.cosim*a10

		x1 := a1*cosomm + a2*sinomm
		x2 := a3*cosomm + a4*sinomm
		x3 := -a1*sinomm + a2*cosomm
		x4 := -a3*sinomm + a4*cosomm
		x5 := a5 * sinomm
		x6 := a6 * sinomm
		x7 := a5 * cosomm
		x8 := a6 * cosomm

		z31 = 12.0*x1*x1 - 3.0*x3*x3
		z32 = 24.0*x1*x2 - 6.0*x3*x4
		z33 = 12.0*x2*x2 - 3.0*x4*x4

		z1 = 3.0*(a1*a1+a2*a2) + z31*r.emsq
		z2 = 6.0*(a1*a3+a2*a4) + z32*r.emsq
		z3 = 3.0*(a3*a3+a4*a4) + z33*r.emsq

		z11 = -6.0*a1*a5 + r.emsq*(-24.0*x1*x7-6.0*x3*x5)
		z12 = -6.0*(a1*a6+a3*a5) + r.emsq*
			(-24.0*(x2*x7+x1*x8)-6.0*(x3*x6+x4*x5))
		z13 = -6.0*a3*a6 + r.emsq*(-24.0*x2*x8-6.0*x4*x6)

		z21 = 6.0*a2*a5 + r.emsq*(24.0*x1*x5-6.0*x3*x7)
		z22 = 6.0*(a4*a5+a2*a6) + r.emsq*
			(24.0*(x2*x5+x1*x6)-6.0*(x4*x7+x3*x8))
		z23 = 6.0*a4*a6 + r.emsq*(24.0*x2*x6-6.0*x4*x8)

		z1 = z1 + z1 + betasq*z31
		z2 = z2 + z2 + betasq*z32
		z3 = z3 + z3 + betasq*z33

		s3 = cc * xnoi
		s2 = -0.5 * s3 / rtemsq
		s4 = s3 * rtemsq
		s1 = -15.0 * r.em * s4
		s5 = x1*x3 + x2*x4
		s6 = x2*x3 + x1*x4
		s7 = x2*x4 - x1*x3

		if lsflg == 1 {
			// Stash the solar pass and switch to lunar inputs.
			r.ss1, r.ss2, r.ss3, r.ss4, r.ss5 = s1, s2, s3, s4, s5
			ss6, ss7 := s6, s7

			r.sz1, r.sz3 = z1, z3
			r.sz11, r.sz13 = z11, z13
			r.sz21, r.sz23 = z21, z23
			r.sz31, r.sz33 = z31, z33

			sz2, sz12, sz22, sz32 := z2, z12, z22, z32

			// The solar periodics, formed here because ss6/ss7 and the sz
			// values are about to be overwritten by the lunar pass.
			p.ds.se2 = 2.0 * r.ss1 * ss6
			p.ds.se3 = 2.0 * r.ss1 * ss7
			p.ds.si2 = 2.0 * r.ss2 * sz12
			p.ds.si3 = 2.0 * r.ss2 * (r.sz13 - r.sz11)
			p.ds.sl2 = -2.0 * r.ss3 * sz2
			p.ds.sl3 = -2.0 * r.ss3 * (r.sz3 - r.sz1)
			p.ds.sl4 = -2.0 * r.ss3 * (-21.0 - 9.0*r.emsq) * zes
			p.ds.sgh2 = 2.0 * r.ss4 * sz32
			p.ds.sgh3 = 2.0 * r.ss4 * (r.sz33 - r.sz31)
			p.ds.sgh4 = -18.0 * r.ss4 * zes
			p.ds.sh2 = -2.0 * r.ss2 * sz22
			p.ds.sh3 = -2.0 * r.ss2 * (r.sz23 - r.sz21)

			zcosg = zcosgl
			zsing = zsingl
			zcosi = zcosil
			zsini = zsinil
			zcosh = zcoshl*cnodm + zsinhl*snodm
			zsinh = snodm*zcoshl - cnodm*zsinhl
			cc = c1l

			continue
		}

		// The lunar pass keeps its z values for dsinit.
		r.z1, r.z3 = z1, z3
		r.z11, r.z13 = z11, z13
		r.z21, r.z23 = z21, z23
		r.z31, r.z33 = z31, z33

		r.s1, r.s2, r.s3, r.s4, r.s5 = s1, s2, s3, s4, s5

		p.ds.ee2 = 2.0 * s1 * s6
		p.ds.e3 = 2.0 * s1 * s7
		p.ds.xi2 = 2.0 * s2 * z12
		p.ds.xi3 = 2.0 * s2 * (z13 - z11)
		p.ds.xl2 = -2.0 * s3 * z2
		p.ds.xl3 = -2.0 * s3 * (z3 - z1)
		p.ds.xl4 = -2.0 * s3 * (-21.0 - 9.0*r.emsq) * zel
		p.ds.xgh2 = 2.0 * s4 * z32
		p.ds.xgh3 = 2.0 * s4 * (z33 - z31)
		p.ds.xgh4 = -18.0 * s4 * zel
		p.ds.xh2 = -2.0 * s2 * z22
		p.ds.xh3 = -2.0 * s2 * (z23 - z21)
	}

	p.ds.zmol = math.Mod(4.7199672+0.22997150*day-gam, twoPi)
	p.ds.zmos = math.Mod(6.2565837+0.017201977*day, twoPi)

	return r
}

// dsinit computes the secular lunisolar rates and, for a resonant orbit, the
// geopotential resonance coefficients.
//
// Called once, at initialisation, where the reference's t and tc are both zero.
// That matters: the element updates the reference performs here (em += dedt*t
// and its four siblings) are therefore no-ops, and their results are discarded
// by the caller. Only the coefficients survive, and only those are computed.
func (p *Propagator) dsinit(r dscomResult, xpidot float64) {
	ecco := p.el.Eccentricity
	argpo := p.el.ArgPerigee.Radians()
	nodeo := p.el.RAAN.Radians()
	mo := p.el.MeanAnomaly.Radians()
	inclm := p.el.Inclination.Radians()

	no := p.noUnkozai
	nm := r.nm
	em := r.em
	emsq := r.emsq

	// ---- which resonance, if any ----
	p.rz.irez = resonanceNone

	if nm > 0.0034906585 && nm < 0.0052359877 {
		p.rz.irez = resonanceSynchronous
	}

	if nm >= 8.26e-3 && nm <= 9.24e-3 && em >= 0.5 {
		p.rz.irez = resonanceHalfDay
	}

	// ---- solar terms ----
	ses := r.ss1 * zns * r.ss5
	sis := r.ss2 * zns * (r.sz11 + r.sz13)
	sls := -zns * r.ss3 * (r.sz1 + r.sz3 - 14.0 - 6.0*emsq)
	sghs := r.ss4 * zns * (r.sz31 + r.sz33 - 6.0)
	shs := -zns * r.ss2 * (r.sz21 + r.sz23)

	// The node correction is meaningless within a few degrees of the poles of
	// the inclination range, where sin(i) goes to zero.
	nearPolar := inclm < polarSingularityBand || inclm > math.Pi-polarSingularityBand
	if nearPolar {
		shs = 0.0
	}

	if r.sinim != 0.0 {
		shs /= r.sinim
	}

	sgs := sghs - r.cosim*shs

	// ---- lunar terms ----
	p.rz.dedt = ses + r.s1*znl*r.s5
	p.rz.didt = sis + r.s2*znl*(r.z11+r.z13)
	p.rz.dmdt = sls - znl*r.s3*(r.z1+r.z3-14.0-6.0*emsq)

	sghl := r.s4 * znl * (r.z31 + r.z33 - 6.0)
	shll := -znl * r.s2 * (r.z21 + r.z23)

	if nearPolar {
		shll = 0.0
	}

	p.rz.domdt = sgs + sghl
	p.rz.dnodt = shs

	if r.sinim != 0.0 {
		p.rz.domdt -= r.cosim / r.sinim * shll
		p.rz.dnodt += shll / r.sinim
	}

	if p.rz.irez == resonanceNone {
		return
	}

	// theta is the Greenwich hour angle; tc is zero here, so this is gsto
	// reduced. Written with the tc term so the expression matches dspace's.
	const tc = 0.0

	theta := math.Mod(p.gsto+tc*rptim, twoPi)

	aonv := math.Pow(nm/p.grav.xke, twoThirds)

	if p.rz.irez == resonanceHalfDay {
		p.initHalfDayResonance(r, aonv, theta, ecco, mo, nodeo)
	}

	if p.rz.irez == resonanceSynchronous {
		p.initSynchronousResonance(r, aonv, theta, emsq, argpo, mo, nodeo, xpidot)
	}

	_ = no
}

// initHalfDayResonance is the twelve-hour (Molniya) case.
//
// The g-coefficients are polynomial fits in eccentricity with three separate
// break points — 0.65, 0.715 and 0.7 — which do not coincide and are not
// typos. They are the boundaries of the fits Hoots published, and smoothing
// them would change the model.
//
// Note also that this branch evaluates its polynomials in the element set's
// ORIGINAL eccentricity, not the lunisolar-updated one: the reference saves em
// and emsq, substitutes ecco and eccsq, and restores them afterwards. Since
// this implementation does not mutate anything, the substitution is simply
// which variable each expression reads — but it is deliberate, and reading the
// updated value here would be wrong.
func (p *Propagator) initHalfDayResonance(r dscomResult, aonv, theta, ecco, mo, nodeo float64) {
	cosim := r.cosim
	sinim := r.sinim
	cosisq := cosim * cosim

	em := ecco
	emsq := p.eccsq
	eoc := em * emsq

	g201 := -0.306 - (em-0.64)*0.440

	var g211, g310, g322, g410, g422, g520 float64

	if em <= 0.65 {
		g211 = 3.616 - 13.2470*em + 16.2900*emsq
		g310 = -19.302 + 117.3900*em - 228.4190*emsq + 156.5910*eoc
		g322 = -18.9068 + 109.7927*em - 214.6334*emsq + 146.5816*eoc
		g410 = -41.122 + 242.6940*em - 471.0940*emsq + 313.9530*eoc
		g422 = -146.407 + 841.8800*em - 1629.014*emsq + 1083.4350*eoc
		g520 = -532.114 + 3017.977*em - 5740.032*emsq + 3708.2760*eoc
	} else {
		g211 = -72.099 + 331.819*em - 508.738*emsq + 266.724*eoc
		g310 = -346.844 + 1582.851*em - 2415.925*emsq + 1246.113*eoc
		g322 = -342.585 + 1554.908*em - 2366.899*emsq + 1215.972*eoc
		g410 = -1052.797 + 4758.686*em - 7193.992*emsq + 3651.957*eoc
		g422 = -3581.690 + 16178.110*em - 24462.770*emsq + 12422.520*eoc

		if em > 0.715 {
			g520 = -5149.66 + 29936.92*em - 54087.36*emsq + 31324.56*eoc
		} else {
			g520 = 1464.74 - 4664.75*em + 3763.64*emsq
		}
	}

	var g521, g532, g533 float64

	if em < 0.7 {
		g533 = -919.22770 + 4988.6100*em - 9064.7700*emsq + 5542.21*eoc
		g521 = -822.71072 + 4568.6173*em - 8491.4146*emsq + 5337.524*eoc
		g532 = -853.66600 + 4690.2500*em - 8624.7700*emsq + 5341.4*eoc
	} else {
		g533 = -37995.780 + 161616.52*em - 229838.20*emsq + 109377.94*eoc
		g521 = -51752.104 + 218913.95*em - 309468.16*emsq + 146349.42*eoc
		g532 = -40023.880 + 170470.89*em - 242699.48*emsq + 115605.82*eoc
	}

	sini2 := sinim * sinim
	f220 := 0.75 * (1.0 + 2.0*cosim + cosisq)
	f221 := 1.5 * sini2
	f321 := 1.875 * sinim * (1.0 - 2.0*cosim - 3.0*cosisq)
	f322 := -1.875 * sinim * (1.0 + 2.0*cosim - 3.0*cosisq)
	f441 := 35.0 * sini2 * f220
	f442 := 39.3750 * sini2 * sini2
	f522 := 9.84375 * sinim * (sini2*(1.0-2.0*cosim-5.0*cosisq) +
		0.33333333*(-2.0+4.0*cosim+6.0*cosisq))
	f523 := sinim * (4.92187512*sini2*(-2.0-4.0*cosim+10.0*cosisq) +
		6.56250012*(1.0+2.0*cosim-3.0*cosisq))
	f542 := 29.53125 * sinim * (2.0 - 8.0*cosim + cosisq*
		(-12.0+8.0*cosim+10.0*cosisq))
	f543 := 29.53125 * sinim * (-2.0 - 8.0*cosim + cosisq*
		(12.0+8.0*cosim-10.0*cosisq))

	xno2 := r.nm * r.nm
	ainv2 := aonv * aonv

	temp1 := 3.0 * xno2 * ainv2
	temp := temp1 * root22
	p.rz.d2201 = temp * f220 * g201
	p.rz.d2211 = temp * f221 * g211

	temp1 *= aonv
	temp = temp1 * root32
	p.rz.d3210 = temp * f321 * g310
	p.rz.d3222 = temp * f322 * g322

	temp1 *= aonv
	temp = 2.0 * temp1 * root44
	p.rz.d4410 = temp * f441 * g410
	p.rz.d4422 = temp * f442 * g422

	temp1 *= aonv
	temp = temp1 * root52
	p.rz.d5220 = temp * f522 * g520
	p.rz.d5232 = temp * f523 * g532

	temp = 2.0 * temp1 * root54
	p.rz.d5421 = temp * f542 * g521
	p.rz.d5433 = temp * f543 * g533

	p.rz.xlamo = math.Mod(mo+nodeo+nodeo-theta-theta, twoPi)
	p.rz.xfact = p.mdot + p.rz.dmdt + 2.0*(p.nodedot+p.rz.dnodt-rptim) - p.noUnkozai
}

// initSynchronousResonance is the one-day (geosynchronous) case.
func (p *Propagator) initSynchronousResonance(
	r dscomResult, aonv, theta, emsq, argpo, mo, nodeo, xpidot float64,
) {
	cosim := r.cosim
	sinim := r.sinim

	g200 := 1.0 + emsq*(-2.5+0.8125*emsq)
	g310 := 1.0 + 2.0*emsq
	g300 := 1.0 + emsq*(-6.0+6.60937*emsq)

	f220 := 0.75 * (1.0 + cosim) * (1.0 + cosim)
	f311 := 0.9375*sinim*sinim*(1.0+3.0*cosim) - 0.75*(1.0+cosim)
	f330 := 1.0 + cosim
	f330 = 1.875 * f330 * f330 * f330

	del1 := 3.0 * r.nm * r.nm * aonv * aonv
	p.rz.del2 = 2.0 * del1 * f220 * g200 * q22
	p.rz.del3 = 3.0 * del1 * f330 * g300 * q33 * aonv
	p.rz.del1 = del1 * f311 * g310 * q31 * aonv

	p.rz.xlamo = math.Mod(mo+nodeo+argpo-theta, twoPi)
	p.rz.xfact = p.mdot + xpidot - rptim + p.rz.dmdt + p.rz.domdt + p.rz.dnodt - p.noUnkozai
}

// initDeepSpace runs the deep-space half of sgp4init.
//
// # On the reference's dpper call here
//
// sgp4init calls dpper with init = 'y' and assigns the result back over the
// element set. That call is a no-op: with init = 'y', dpper's entire body is
// inside an `if init == 'n'` block and it returns its inputs unchanged. It is
// omitted rather than transcribed, and said so here because its absence is the
// kind of thing that looks like a missing step.
func (p *Propagator) initDeepSpace(epoch1950 float64) {
	const tc = 0.0

	xpidot := p.argpdot + p.nodedot

	r := p.dscom(epoch1950, tc)
	p.dsinit(r, xpidot)
}
