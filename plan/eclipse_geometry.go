package plan

import (
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/constants"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/time"
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

// lunarPenumbralMagnitude is the fraction of the Moon's diameter inside
// Earth's penumbra, negative when it is outside by that fraction, with the
// shadow enlarged by Danjon's rule. Positive is an eclipse. Also returned is
// the Moon's distance from the shadow axis as a fraction of the distance at
// which it would just graze the penumbra.
func (g eclipseGeometry) lunarPenumbralMagnitude() (magnitude, gamma float64) {
	moonDist := g.moon.Norm()
	sunDist := g.sun.Norm()

	moonParallax := math.Asin(earthEquatorialRadiusKm / moonDist)
	sunParallax := math.Asin(earthEquatorialRadiusKm / sunDist)
	sunSemiDiameter := math.Asin(math.Sin(sunSemiDiameterAt1AU) * auKm / sunDist)
	moonSemiDiameter := math.Asin(moonRadiusRatio * earthEquatorialRadiusKm / moonDist)

	penumbraRadius := danjonParallaxFactor*moonParallax + sunSemiDiameter + sunParallax
	limit := penumbraRadius + moonSemiDiameter
	d := g.lunarAxisDistance()

	return (limit - d) / (2 * moonSemiDiameter), d / limit
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

// solarPenumbraMargin is how far, in Earth equatorial radii, the Moon's
// penumbra reaches past Earth's limb as seen along the shadow axis: positive
// when it falls on the Earth, which is an eclipse. Also returned is the axis's
// distance from Earth's center as a fraction of the distance at which the
// penumbra would just graze the limb.
//
// Earth is the WGS 84 spheroid, and its outline along the axis is an ellipse
// whose short axis points at the true pole of date. That matters here and
// not for the Moon: the eclipses at the limit are partials grazing the polar
// regions, where the limb is up to 0.0034 Earth radii closer in, and with a
// spherical Earth every one of them comes out that much deeper than the
// canon has it.
//
// The penumbra's reach is compared with the outline's radius in the same
// direction, which is not quite the distance to the nearest point of the limb:
// the limb's normal leans from the radius by at most the flattening, 0.0034
// radians, and the error that makes is second order in it. Stretching the
// plane until the outline is a circle, as Besselian elements do, would
// stretch the penumbra too, by up to 0.0018 Earth radii.
func (g eclipseGeometry) solarPenumbraMargin(t time.Time) (margin, gamma float64) {
	axis, crossing := g.solarShadowAxis()

	// The penumbral cone: its half-angle, and its radius where it crosses the
	// fundamental plane, the Moon's distance behind which is -moon·axis.
	moonRadius := moonRadiusRatio * earthEquatorialRadiusKm
	sunRadius := math.Sin(sunSemiDiameterAt1AU) * auKm
	halfAngle := math.Asin((sunRadius + moonRadius) / g.moon.Sub(g.sun).Norm())
	behind := -g.moon.Dot(axis)
	penumbra := (moonRadius/math.Cos(halfAngle) + behind*math.Tan(halfAngle)) / earthEquatorialRadiusKm

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

	return limb + penumbra - r, r / (limb + penumbra)
}

// angleBetweenVectors is the angle between a and b, in radians.
func angleBetweenVectors(a, b vector.Vec3) float64 {
	return math.Atan2(a.Cross(b).Norm(), a.Dot(b))
}
