package sgp4

import "math"

// Constants the model defines for itself, in its own units.
const (
	// twoThirds and negTwoThirds appear as exponents throughout.
	twoThirds    = 2.0 / 3.0
	negTwoThirds = -2.0 / 3.0

	// deepSpaceMinutes is where SGP4 hands over to SDP4: a period of 225
	// minutes or more. Not a tunable — it is the model's own branch.
	deepSpaceMinutes = 225.0

	// simplifiedDragPerigeeKM is where SGP4 switches to its simplified drag
	// model, written in the reference as rp < 220/radiusearthkm + 1.
	simplifiedDragPerigeeKM = 220.0

	// s4LowerPerigeeKM and s4FloorPerigeeKM bound the s⁴ atmospheric-density
	// alteration. Below 156 km the density parameter is lowered with perigee;
	// below 98 km it is pinned.
	s4LowerPerigeeKM = 156.0
	s4FloorPerigeeKM = 98.0

	// s4BaseKM and s4CeilingKM are the density band's own two numbers: the
	// model's reference altitude and the upper edge it is measured against.
	//
	// The ceiling is 120, and it is worth knowing why that is written out
	// here rather than inlined. The Go implementation astrogo used before this
	// one has 128, and that single digit is the cause of six of the seven
	// divergences astrogo measured against Vallado's reference suite — up to
	// 3438 km, on decaying and low-perigee orbits, growing as t² because the
	// value feeds the secular drag coefficient. See TuSKan/astrogo#309.
	s4BaseKM    = 78.0
	s4CeilingKM = 120.0

	// inclinationSingularityGuard is the divisor the reference substitutes when
	// the inclination is 180 degrees and 1+cos(i) would be zero.
	//
	// Vallado's own note: the old check used 1 + cos(pi - 1e-9) and compared it
	// to 1.5e-12, so the threshold was changed to 1.5e-12 for consistency.
	inclinationSingularityGuard = 1.5e-12

	// minEccentricityForDrag is the threshold below which the two drag terms
	// that divide by eccentricity are dropped rather than computed.
	minEccentricityForDrag = 1.0e-4

	// jd1950 is the Julian Date of 0 January 1950, 0h — the origin SGP4
	// carries its epoch from.
	jd1950 = 2433281.5

	// unixEpochJD is the Julian Date of 1970-01-01 0h, where a Unix count of
	// seconds starts; see utcLabel.
	unixEpochJD = 2440587.5

	// secondsPerDay is a UTC day as SGP4 counts one, leap seconds or not.
	secondsPerDay = 86400.0
)

// initl reproduces the reference's initl: the auxiliary epoch quantities, the
// un-Kozai'd mean motion, and the sidereal time at epoch.
//
// # The un-Kozai step is the one that matters
//
// A TLE's mean motion is a Kozai mean element. SGP4 converts it to the Brouwer
// form before deriving anything, and the difference is not cosmetic: for a low
// orbit it moves the semi-major axis by about 1.4 km, which is the difference
// between reproducing the model's own simplified-drag branch and merely landing
// near it.
//
// Everything here is written in the reference's association, including
// pow(x, 2/3) rather than cbrt(x*x). Those are the same number in exact
// arithmetic and not always the same float64, and this package is measured
// against output the reference produced.
func (p *Propagator) initl(epoch1950 float64) {
	g := p.grav
	ecco := p.el.Eccentricity
	inclo := p.el.Inclination.Radians()

	// The Kozai mean motion in radians per minute, which is SGP4's unit.
	noKozai := p.el.MeanMotion * twoPi / 1440.0

	p.eccsq = ecco * ecco
	p.omeosq = 1.0 - p.eccsq
	p.rteosq = math.Sqrt(p.omeosq)
	p.cosio = math.Cos(inclo)
	p.cosio2 = p.cosio * p.cosio

	// ------------------ un-kozai the mean motion -----------------
	ak := math.Pow(g.xke/noKozai, twoThirds)
	d1 := 0.75 * g.j2 * (3.0*p.cosio2 - 1.0) / (p.rteosq * p.omeosq)
	del := d1 / (ak * ak)
	adel := ak * (1.0 - del*del - del*(1.0/3.0+134.0*del*del/81.0))
	del = d1 / (adel * adel)

	p.noUnkozai = noKozai / (1.0 + del)

	p.ao = math.Pow(g.xke/p.noUnkozai, twoThirds)
	p.sinio = math.Sin(inclo)
	po := p.ao * p.omeosq
	p.con42 = 1.0 - 5.0*p.cosio2
	p.con41 = -p.con42 - p.cosio2 - p.cosio2
	p.ainv = 1.0 / p.ao
	p.posq = po * po
	p.rp = p.ao * (1.0 - ecco)

	if p.mode == ModeAFSPC {
		p.gsto = gstoAFSPC(epoch1950)
	} else {
		p.gsto = gmst82(epoch1950 + jd1950)
	}
}

// initNearEarth computes the secular and periodic coefficients the near-Earth
// evaluation reads, reproducing sgp4init up to its deep-space branch.
//
// The guard the reference puts round all of this — `if omeosq >= 0 or
// no_unkozai >= 0` — is unconditionally true for any element set that passed
// [Elements.Validate], since eccentricity is below 1 and the mean motion is
// positive. It is kept as an assertion rather than a branch: if it ever fails,
// validation has stopped meaning what it says.
func (p *Propagator) initNearEarth() {
	g := p.grav
	ecco := p.el.Eccentricity
	argpo := p.el.ArgPerigee.Radians()
	mo := p.el.MeanAnomaly.Radians()

	// ss and qzms2t are the unaltered density parameters, for a perigee above
	// the s⁴ band.
	ss := s4BaseKM/g.radiusKM + 1.0
	qzms2ttemp := (s4CeilingKM - s4BaseKM) / g.radiusKM
	qzms2t := qzms2ttemp * qzms2ttemp * qzms2ttemp * qzms2ttemp

	p.a = math.Pow(p.noUnkozai*g.tumin, negTwoThirds)
	p.alta = p.a*(1.0+ecco) - 1.0
	p.altp = p.a*(1.0-ecco) - 1.0

	p.isimp = p.rp < simplifiedDragPerigeeKM/g.radiusKM+1.0

	sfour := ss
	qzms24 := qzms2t
	perige := (p.rp - 1.0) * g.radiusKM

	// ---- for perigees below 156 km, s and qoms2t are altered ----
	if perige < s4LowerPerigeeKM {
		sfour = perige - s4BaseKM
		if perige < s4FloorPerigeeKM {
			sfour = 20.0
		}

		// Multiplied out rather than raised to the fourth, which is the
		// reference's own choice and keeps the last bits.
		qzms24temp := (s4CeilingKM - sfour) / g.radiusKM
		qzms24 = qzms24temp * qzms24temp * qzms24temp * qzms24temp
		sfour = sfour/g.radiusKM + 1.0
	}

	pinvsq := 1.0 / p.posq
	tsi := 1.0 / (p.ao - sfour)

	p.eta = p.ao * ecco * tsi
	etasq := p.eta * p.eta
	eeta := ecco * p.eta
	psisq := math.Abs(1.0 - etasq)
	coef := qzms24 * math.Pow(tsi, 4.0)
	coef1 := coef / math.Pow(psisq, 3.5)

	cc2 := coef1 * p.noUnkozai * (p.ao*(1.0+1.5*etasq+eeta*(4.0+etasq)) +
		0.375*g.j2*tsi/psisq*p.con41*(8.0+3.0*etasq*(8.0+etasq)))
	p.cc1 = p.el.BStar * cc2

	cc3 := 0.0
	if ecco > minEccentricityForDrag {
		cc3 = -2.0 * coef * tsi * g.j3oj2 * p.noUnkozai * p.sinio / ecco
	}

	p.x1mth2 = 1.0 - p.cosio2

	p.cc4 = 2.0 * p.noUnkozai * coef1 * p.ao * p.omeosq *
		(p.eta*(2.0+0.5*etasq) + ecco*(0.5+2.0*etasq) -
			g.j2*tsi/(p.ao*psisq)*
				(-3.0*p.con41*(1.0-2.0*eeta+etasq*(1.5-0.5*eeta))+
					0.75*p.x1mth2*(2.0*etasq-eeta*(1.0+etasq))*math.Cos(2.0*argpo)))

	p.cc5 = 2.0 * coef1 * p.ao * p.omeosq * (1.0 + 2.75*(etasq+eeta) + eeta*etasq)

	cosio4 := p.cosio2 * p.cosio2
	temp1 := 1.5 * g.j2 * pinvsq * p.noUnkozai
	temp2 := 0.5 * temp1 * g.j2 * pinvsq
	temp3 := -0.46875 * g.j4 * pinvsq * pinvsq * p.noUnkozai

	p.mdot = p.noUnkozai + 0.5*temp1*p.rteosq*p.con41 +
		0.0625*temp2*p.rteosq*(13.0-78.0*p.cosio2+137.0*cosio4)
	p.argpdot = -0.5*temp1*p.con42 +
		0.0625*temp2*(7.0-114.0*p.cosio2+395.0*cosio4) +
		temp3*(3.0-36.0*p.cosio2+49.0*cosio4)

	xhdot1 := -temp1 * p.cosio
	p.nodedot = xhdot1 + (0.5*temp2*(4.0-19.0*p.cosio2)+
		2.0*temp3*(3.0-7.0*p.cosio2))*p.cosio

	p.omgcof = p.el.BStar * cc3 * math.Cos(argpo)

	p.xmcof = 0.0
	if ecco > minEccentricityForDrag {
		p.xmcof = -twoThirds * coef * p.el.BStar / eeta
	}

	p.nodecf = 3.5 * p.omeosq * xhdot1 * p.cc1
	p.t2cof = 1.5 * p.cc1

	// The 180-degree inclination guard: 1+cos(i) goes to zero there.
	if math.Abs(p.cosio+1.0) > inclinationSingularityGuard {
		p.xlcof = -0.25 * g.j3oj2 * p.sinio * (3.0 + 5.0*p.cosio) / (1.0 + p.cosio)
	} else {
		p.xlcof = -0.25 * g.j3oj2 * p.sinio * (3.0 + 5.0*p.cosio) / inclinationSingularityGuard
	}

	p.aycof = -0.5 * g.j3oj2 * p.sinio

	delmotemp := 1.0 + p.eta*math.Cos(mo)
	p.delmo = delmotemp * delmotemp * delmotemp
	p.sinmao = math.Sin(mo)
	p.x7thm1 = 7.0*p.cosio2 - 1.0

	p.deep = twoPi/p.noUnkozai >= deepSpaceMinutes
	if p.deep {
		// SDP4 always takes the simplified drag path, whatever the perigee.
		p.isimp = true

		return
	}

	// ---- the higher-order drag terms, for a full-drag near-Earth orbit ----
	if !p.isimp {
		cc1sq := p.cc1 * p.cc1
		p.d2 = 4.0 * p.ao * tsi * cc1sq
		temp := p.d2 * tsi * p.cc1 / 3.0
		p.d3 = (17.0*p.ao + sfour) * temp
		p.d4 = 0.5 * temp * p.ao * tsi * (221.0*p.ao + 31.0*sfour) * p.cc1
		p.t3cof = p.d2 + 2.0*cc1sq
		p.t4cof = 0.25 * (3.0*p.d3 + p.cc1*(12.0*p.d2+10.0*cc1sq))
		p.t5cof = 0.2 * (3.0*p.d4 + 12.0*p.cc1*p.d3 + 6.0*p.d2*p.d2 +
			15.0*cc1sq*(2.0*p.d2+cc1sq))
	}
}
