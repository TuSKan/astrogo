package coord

import (
	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/time"
)

// ── ICRS <-> Galactic ────────────────────────────────────────────────────────

// ICRSToGalactic converts ICRS coordinates to Galactic coordinates.
func ICRSToGalactic(c ICRS) Galactic {
	l, b := gofaext.Icrs2g(c.RA().Radians(), c.Dec().Radians())
	return NewGalactic(angle.Rad(l).Wrap360(), angle.Rad(b))
}

// GalacticToICRS converts Galactic coordinates to ICRS coordinates.
func GalacticToICRS(c Galactic) ICRS {
	ra, dec := gofaext.G2icrs(c.L().Radians(), c.B().Radians())
	return NewICRS(angle.Rad(ra).Wrap360(), angle.Rad(dec))
}

// ── ICRS <-> Ecliptic ────────────────────────────────────────────────────────

// ICRSToEcliptic converts ICRS coordinates to Geocentric Ecliptic coordinates
// of the given date.
func ICRSToEcliptic(c ICRS, t time.Time) Ecliptic {
	jd1, jd2 := t.JDParts()
	lon, lat := gofaext.Eqec06(jd1, jd2, c.RA().Radians(), c.Dec().Radians())
	return NewEcliptic(angle.Rad(lon).Wrap360(), angle.Rad(lat))
}

// EclipticToICRS converts Geocentric Ecliptic coordinates of the given date
// to ICRS coordinates.
func EclipticToICRS(c Ecliptic, t time.Time) ICRS {
	jd1, jd2 := t.JDParts()
	ra, dec := gofaext.Eceq06(jd1, jd2, c.Lon().Radians(), c.Lat().Radians())
	return NewICRS(angle.Rad(ra).Wrap360(), angle.Rad(dec))
}
