package coord

import (
	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/epoch"
	"github.com/TuSKan/astrogo/internal/gofaext"
)

// ── ICRS <-> Galactic ────────────────────────────────────────────────────────

// ICRSToGalactic converts ICRS coordinates to Galactic coordinates.
func ICRSToGalactic(c ICRS) Galactic {
	l, b := gofaext.Icrs2g(c.RA().Radians(), c.Dec().Radians())
	return NewGalactic(angle.Latitude{Angle: angle.Rad(b)}, angle.Longitude{Angle: angle.Rad(l).Wrap360()})
}

// GalacticToICRS converts Galactic coordinates to ICRS coordinates.
func GalacticToICRS(c Galactic) ICRS {
	ra, dec := gofaext.G2icrs(c.L().Radians(), c.B().Radians())
	return NewICRS(angle.Rad(ra).Wrap360(), angle.Rad(dec))
}

// ── ICRS <-> Ecliptic ────────────────────────────────────────────────────────

// ICRSToEcliptic converts ICRS coordinates to Geocentric Ecliptic coordinates
// of the given date.
//
// The date is taken as TT, which is what the ecliptic of date is defined on,
// whatever scale t holds.
func ICRSToEcliptic(c ICRS, t epoch.Time) Ecliptic {
	jd1, jd2 := t.TT().JDParts()
	lon, lat := gofaext.Eqec06(jd1, jd2, c.RA().Radians(), c.Dec().Radians())

	return NewEcliptic(angle.Latitude{Angle: angle.Rad(lat)}, angle.Longitude{Angle: angle.Rad(lon).Wrap360()})
}

// EclipticToICRS converts Geocentric Ecliptic coordinates of the given date
// to ICRS coordinates. The date is taken as TT, as for [ICRSToEcliptic].
func EclipticToICRS(c Ecliptic, t epoch.Time) ICRS {
	jd1, jd2 := t.TT().JDParts()
	ra, dec := gofaext.Eceq06(jd1, jd2, c.Lon().Radians(), c.Lat().Radians())

	return NewICRS(angle.Rad(ra).Wrap360(), angle.Rad(dec))
}
