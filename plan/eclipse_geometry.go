package plan

import (
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/constants"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
	"github.com/TuSKan/astrogo/vector"
)

// Eclipse geometry, sized as NASA's Five Millennium Canons of Lunar and Solar
// Eclipses (Espenak & Meeus) size it, so that "is this an eclipse" has the
// answer those catalogs give.
//
// Measured against them over six centuries (years 1–200, 501–600, 1001–1100,
// 1501–1600, 1901–2000), DE441: lunar penumbral and umbral magnitudes agree to
// 0.0005 and gamma to 0.0002 Earth radii for all 1452 lunar eclipses; solar
// gamma agrees to 0.0002 Earth radii for all 1433 solar eclipses; greatest
// eclipse falls 0.13 minutes after the catalog's on average. What decides
// that agreement is the Sun: the shadow axis is the apparent Sun's, aberration
// included. With the geometric Sun instead, gamma is off by up to 0.0008 Earth
// radii and greatest eclipse by 0.79 minutes, the time the Moon takes to cross
// the Sun's 20″ of aberration.
//
// Before #401 an eclipse was any syzygy within a fixed 1.58° of ecliptic
// latitude. The real limit depends on the Moon's and Sun's distances that
// month, and the fixed one reported 76 lunar and 97 solar eclipses over those
// centuries that do not happen, and missed the penumbral eclipse of
// 1958-04-04.

const (
	// eclipseLatitudeScreen is the ecliptic latitude at syzygy beyond which no
	// eclipse is possible, used only to skip the geometry for syzygies that
	// cannot qualify. The widest limit either kind of eclipse reaches is
	// 1.59°, at lunar perigee and solar perihelion, measured as the Moon's
	// closest approach to the shadow axis; the latitude at syzygy is that
	// distance over the cosine of the Moon's path's 5–6° tilt to the
	// ecliptic, under 1.60°. The screen has room to spare because a syzygy it
	// wrongly passes costs a little time and one it wrongly stops costs an
	// eclipse.
	eclipseLatitudeScreen = 1.7 // degrees

	// moonRadiusRatio is the Moon's radius in Earth equatorial radii, the
	// value the canons use for penumbral contacts.
	moonRadiusRatio = 0.2725076

	// moonRadiusRatioUmbral is the smaller value the solar canon uses for
	// umbral contacts, the Moon's mean radius less the depth of its limb
	// valleys, which is what decides whether an eclipse is total.
	moonRadiusRatioUmbral = 0.2722810

	// sunSemiDiameterAt1AU is the Sun's semi-diameter at one astronomical
	// unit, Auwers (1891), 959.63″, in radians: the eclipse convention, and
	// not IAU 2015's nominal solar radius, which is 0.4″ smaller (#403).
	sunSemiDiameterAt1AU = 959.63 / 206264.80624709636

	// danjonParallaxFactor enlarges the Moon's parallax, and so Earth's
	// shadows, by Danjon's rule, as NASA's lunar canon does: 1 + 1/85 for an
	// opaque layer 75 km thick, less 1/594 for Earth's oblateness at 45°
	// latitude (eclipse.gsfc.nasa.gov/LEcat5/shadow.html, Eq. 1-5).
	danjonParallaxFactor = 1.01

	// eclipseSearchHalfWidth is how far either side of a syzygy greatest
	// eclipse is searched for. The Moon's closest approach to the shadow
	// axis falls within about 20 minutes of the syzygy.
	eclipseSearchHalfWidth = 60 // minutes
)

// eclipseGeometry is the Sun and Moon at one instant, geocentric, in
// kilometers: the Sun apparent, the Moon geometric.
type eclipseGeometry struct {
	sun, moon vector.Vec3
}

// earthEquatorialRadiusKm is WGS 84's semi-major axis.
var earthEquatorialRadiusKm = constants.WGS84.SemiMajorAxis.Value / 1000

// auKm is the astronomical unit.
var auKm = constants.IAU.AstronomicalUnit.Value / 1000

func newEclipseGeometry(t time.Time, prov eph.Provider) (eclipseGeometry, error) {
	sun, err := eph.ApparentState(prov, eph.Sun, t)
	if err != nil {
		return eclipseGeometry{}, fmt.Errorf("eclipse: sun position: %w", err)
	}

	moon, err := eph.Position(prov, eph.Moon, t)
	if err != nil {
		return eclipseGeometry{}, fmt.Errorf("eclipse: moon position: %w", err)
	}

	return eclipseGeometry{sun: sun.Pos.MulScalar(auKm), moon: moon.MulScalar(auKm)}, nil
}

// lunarAxisDistance is the angle, in radians, between the Moon's center and the
// axis of Earth's shadow: the direction opposite the apparent Sun. Greatest
// lunar eclipse is its minimum.
func (g eclipseGeometry) lunarAxisDistance() float64 {
	return angleBetweenVectors(g.moon, g.sun.MulScalar(-1))
}

// lunarMagnitudes are the fractions of the Moon's diameter inside Earth's
// penumbra and umbra, each negative when the Moon is outside that shadow by
// that fraction, with the shadows enlarged by Danjon's rule: the canon's
// penumbral and umbral magnitudes. A positive penumbral magnitude is an
// eclipse; the umbral one decides its kind. Also returned is the Moon's
// distance from the shadow axis as a fraction of the distance at which it
// would just graze the penumbra.
func (g eclipseGeometry) lunarMagnitudes() (penumbral, umbral, gamma float64) {
	moonDist := g.moon.Norm()
	sunDist := g.sun.Norm()

	moonParallax := math.Asin(earthEquatorialRadiusKm / moonDist)
	sunParallax := math.Asin(earthEquatorialRadiusKm / sunDist)
	sunSemiDiameter := math.Asin(math.Sin(sunSemiDiameterAt1AU) * auKm / sunDist)
	moonSemiDiameter := math.Asin(moonRadiusRatio * earthEquatorialRadiusKm / moonDist)

	penumbraRadius := danjonParallaxFactor*moonParallax + sunSemiDiameter + sunParallax
	umbraRadius := danjonParallaxFactor*moonParallax - sunSemiDiameter + sunParallax
	limit := penumbraRadius + moonSemiDiameter
	d := g.lunarAxisDistance()

	return (limit - d) / (2 * moonSemiDiameter), (umbraRadius + moonSemiDiameter - d) / (2 * moonSemiDiameter), d / limit
}

// lunarKind classifies a lunar eclipse by its umbral magnitude: total when
// the whole Moon is inside the umbra, partial when some of it is, penumbral
// otherwise.
func lunarKind(umbral float64) EclipseKind {
	switch {
	case umbral >= 1:
		return EclipseTotal
	case umbral > 0:
		return EclipsePartial
	default:
		return EclipsePenumbral
	}
}

// solarShadowAxis returns the unit direction of the Moon's shadow axis, from
// the Sun through the Moon, and the axis's crossing of the fundamental plane —
// the plane through Earth's center perpendicular to it — in kilometers.
func (g eclipseGeometry) solarShadowAxis() (axis, crossing vector.Vec3) {
	axis = g.moon.Sub(g.sun).Unit()
	crossing = g.moon.Sub(axis.MulScalar(g.moon.Dot(axis)))

	return axis, crossing
}

// solarAxisDistance is the shadow axis's distance from Earth's center, in
// Earth equatorial radii: the canon's gamma, whose minimum is greatest solar
// eclipse.
func (g eclipseGeometry) solarAxisDistance() float64 {
	_, crossing := g.solarShadowAxis()

	return crossing.Norm() / earthEquatorialRadiusKm
}

// solarShadow is the Moon's shadow at one instant, described as Besselian
// elements describe it: in the fundamental plane, through Earth's center and
// perpendicular to the shadow axis, in Earth equatorial radii.
type solarShadow struct {
	// axis runs from the Sun through the Moon; pole is Earth's true pole of
	// date; crossing is where the axis meets the fundamental plane.
	axis, pole, crossing vector.Vec3

	// l1 and l2 are the penumbral and umbral radii in the plane, and tanF1
	// and tanF2 how fast each shrinks with height above it, toward the Moon.
	// l2 is negative where the umbra's vertex lies beyond the plane: there the
	// eclipse is total, and where it is positive, annular.
	l1, l2, tanF1, tanF2 float64

	// r is the axis's distance from Earth's center; limb is the radius of
	// Earth's outline in the same direction.
	r, limb float64
}

// solarShadow computes the shadow at t.
//
// Earth is the WGS 84 spheroid, and its outline along the axis is an ellipse
// whose short axis points at the true pole of date. That matters: the
// eclipses at the limit are partials grazing the polar regions, where the
// limb is up to 0.0034 Earth radii closer in, and with a spherical Earth
// every one of them comes out that much deeper than the canon has it.
//
// limb is the outline's radius in the axis's direction, which is not quite the
// distance to the nearest point of the limb: the limb's normal leans from the
// radius by at most the flattening, 0.0034 radians, and the error that makes
// is second order in it. Stretching the plane until the outline is a circle,
// as Besselian elements do, would stretch the penumbra too, by up to 0.0018
// Earth radii.
func (g eclipseGeometry) solarShadow(t time.Time) solarShadow {
	axis, crossing := g.solarShadowAxis()

	// The cones: the penumbra's half-angle f1 and the umbra's f2, and their
	// radii in the plane, from the Moon's height above it, z.
	sunRadius := math.Sin(sunSemiDiameterAt1AU) * auKm / earthEquatorialRadiusKm
	sunMoon := g.moon.Sub(g.sun).Norm() / earthEquatorialRadiusKm
	z := -g.moon.Dot(axis) / earthEquatorialRadiusKm

	sinF1 := (sunRadius + moonRadiusRatio) / sunMoon
	sinF2 := (sunRadius - moonRadiusRatioUmbral) / sunMoon
	tanF1 := math.Tan(math.Asin(sinF1))
	tanF2 := math.Tan(math.Asin(sinF2))

	// The true pole of date in GCRS is the third row of the bias-precession-
	// nutation matrix.
	tt1, tt2 := t.TT().JDParts()
	bpn := gofaext.Pnm06a(tt1, tt2)
	pole := vector.Vec3{X: bpn[2][0], Y: bpn[2][1], Z: bpn[2][2]}

	// The outline has semi-axes 1 and rho, rho along the pole's projection;
	// d is the axis's declination of date.
	north := pole.Sub(axis.MulScalar(pole.Dot(axis))).Unit()
	east := north.Cross(axis)

	f := constants.Derived.WGS84Flattening.Value
	sinD := pole.Dot(axis)
	rho := math.Sqrt(1 - f*(2-f)*(1-sinD*sinD))

	x := crossing.Dot(east) / earthEquatorialRadiusKm
	y := crossing.Dot(north) / earthEquatorialRadiusKm
	r := math.Hypot(x, y)

	// The outline's radius toward the crossing: 1/sqrt(cos²θ + sin²θ/rho²).
	limb := 1.0

	if r > 0 {
		cos, sin := x/r, y/r
		limb = 1 / math.Sqrt(cos*cos+sin*sin/(rho*rho))
	}

	return solarShadow{
		axis: axis, pole: pole, crossing: crossing.MulScalar(1 / earthEquatorialRadiusKm),
		l1: (z + moonRadiusRatio/sinF1) * tanF1, l2: (z - moonRadiusRatioUmbral/sinF2) * tanF2,
		tanF1: tanF1, tanF2: tanF2,
		r: r, limb: limb,
	}
}

// solarPenumbraMargin is how far, in Earth equatorial radii, the Moon's
// penumbra reaches past Earth's limb as seen along the shadow axis: positive
// when it falls on the Earth, which is an eclipse. Also returned is the axis's
// distance from Earth's center as a fraction of the distance at which the
// penumbra would just graze the limb.
func (g eclipseGeometry) solarPenumbraMargin(t time.Time) (margin, gamma float64) {
	return g.solarShadow(t).margin()
}

// margin is solarPenumbraMargin for a shadow already computed.
func (s solarShadow) margin() (margin, gamma float64) {
	return s.limb + s.l1 - s.r, s.r / (s.limb + s.l1)
}

// central reports whether the shadow axis meets the Earth.
func (s solarShadow) central() bool { return s.r < s.limb }

// greatest is the eclipse's kind and magnitude at the point of greatest
// eclipse, as the solar canon defines both.
//
// When the axis meets the Earth that point is where it does, on the surface
// facing the Sun, at height zeta above the plane, where the shadow's radii are
// L1 = l1 − zeta·tanF1 and L2 = l2 − zeta·tanF2. The eclipse is total there if
// L2 < 0 and annular if not, and its magnitude is the ratio of the Moon's
// apparent diameter to the Sun's, (L1 − L2)/(L1 + L2). If the limb, at the
// same instant, sees the other kind, the eclipse is hybrid.
//
// When the axis misses the Earth the point is on the limb nearest it, a
// distance m outside the Earth, where the magnitude is the fraction of the
// Sun's diameter covered, (l1 − m)/(l1 + l2) — for a non-central total or
// annular eclipse as much as a partial one, which is what the canon
// tabulates for those.
//
// A hybrid can also change kind along its path rather than across it; that
// needs the path's ends, which this instant does not have (see
// solarEclipseKind).
func (s solarShadow) greatest() (EclipseKind, float64) {
	if s.central() {
		zeta := s.surfaceHeight()
		big := s.l1 - zeta*s.tanF1
		small := s.l2 - zeta*s.tanF2
		magnitude := (big - small) / (big + small)

		switch {
		case small >= 0:
			return EclipseAnnular, magnitude
		case s.l2 > 0:
			return EclipseHybrid, magnitude
		default:
			return EclipseTotal, magnitude
		}
	}

	m := s.r - s.limb
	magnitude := (s.l1 - m) / (s.l1 + s.l2)

	switch {
	case s.l2 < 0 && m < -s.l2:
		return EclipseTotal, magnitude
	case s.l2 > 0 && m < s.l2:
		return EclipseAnnular, magnitude
	default:
		return EclipsePartial, magnitude
	}
}

// surfaceHeight is how far above the fundamental plane, toward the Sun, the
// shadow axis meets the WGS 84 spheroid, in Earth equatorial radii. The point
// is crossing + zeta·n with n = −axis, and on the spheroid
//
//	|P|² + e′(P·pole)² = 1,  e′ = 1/(1−f)² − 1,
//
// a quadratic in zeta whose larger root is the sunward side. Only meaningful
// for a central eclipse.
func (s solarShadow) surfaceHeight() float64 {
	f := constants.Derived.WGS84Flattening.Value
	ep := 1/((1-f)*(1-f)) - 1

	n := s.axis.MulScalar(-1)
	qp, np := s.crossing.Dot(s.pole), n.Dot(s.pole)

	a := 1 + ep*np*np
	b := 2 * ep * qp * np
	c := s.crossing.Norm2() + ep*qp*qp - 1

	return (-b + math.Sqrt(math.Max(0, b*b-4*a*c))) / (2 * a)
}

// solarEclipseKind is the kind and magnitude of the solar eclipse whose
// greatest eclipse is at t: greatest's answer, and a hybrid where the
// central path's two ends disagree with its middle.
//
// A hybrid that is annular at both ends and total in the middle shows at the
// instant of greatest eclipse, across the path. One that begins total and
// ends annular, or the reverse, does not: the umbra's vertex crosses the
// Earth's surface along the path, hours apart. So the path's ends are found —
// the instants the axis enters and leaves the Earth, where the point on it is
// on the limb and its umbral radius is l2 — and their kind compared with the
// middle's. Against the canon — the six centuries plan's NASA test reads,
// and 2001–2100 — this finds all 85 hybrids, 4 of which the instant alone
// calls total.
func solarEclipseKind(prov eph.Provider, t time.Time, s solarShadow) (EclipseKind, float64, error) {
	kind, magnitude := s.greatest()
	if !s.central() || (kind != EclipseTotal && kind != EclipseAnnular) {
		return kind, magnitude, nil
	}

	for _, dir := range []float64{-1, 1} {
		end, ok, err := centralPathEnd(prov, t, dir)
		if err != nil {
			return 0, 0, err
		}

		if !ok {
			continue
		}

		g, err := newEclipseGeometry(end, prov)
		if err != nil {
			return 0, 0, err
		}

		if (g.solarShadow(end).l2 < 0) != (kind == EclipseTotal) {
			return EclipseHybrid, magnitude, nil
		}
	}

	return kind, magnitude, nil
}

// centralPathEnd finds the last instant, before t (dir −1) or after it (dir
// +1), at which the shadow axis still meets the Earth: stepping out five
// minutes at a time for up to five hours, longer than any central path, then
// bisecting to well under a second. ok is false if the axis never leaves.
func centralPathEnd(prov eph.Provider, t time.Time, dir float64) (end time.Time, ok bool, err error) {
	central := func(minutes float64) (bool, error) {
		at := t.Add(unit.Minutes(dir * minutes))

		g, err := newEclipseGeometry(at, prov)
		if err != nil {
			return false, err
		}

		return g.solarShadow(at).central(), nil
	}

	var inside, outside float64

	for step := 5.0; step <= 300; step += 5 {
		c, err := central(step)
		if err != nil {
			return time.Time{}, false, err
		}

		if !c {
			outside, ok = step, true

			break
		}

		inside = step
	}

	if !ok {
		return time.Time{}, false, nil
	}

	for range 20 {
		mid := (inside + outside) / 2

		c, err := central(mid)
		if err != nil {
			return time.Time{}, false, err
		}

		if c {
			inside = mid
		} else {
			outside = mid
		}
	}

	return t.Add(unit.Minutes(dir * inside)), true, nil
}

// angleBetweenVectors is the angle between a and b, in radians.
func angleBetweenVectors(a, b vector.Vec3) float64 {
	return math.Atan2(a.Cross(b).Norm(), a.Dot(b))
}
