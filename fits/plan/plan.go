package plan

import (
	"errors"
	"fmt"
	"strings"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/fits"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/unit"
	"github.com/TuSKan/astrogo/vector"
)

// SiteFromFITS returns the observatory a FITS header was taken from. It reads,
// in the order the FITS Standard 4.0 prefers them (§8.4.1, §9.1):
//
//   - OBSGEO-X, OBSGEO-Y, OBSGEO-Z: the ITRS position in meters, the form the
//     standard calls strongly preferred.
//   - OBSGEO-B, OBSGEO-L, OBSGEO-H: geodetic latitude, longitude east
//     positive, and height in meters. The standard measures the height from
//     the IAU 1976 ellipsoid, whose semi-major axis is 3 m longer than WGS
//     84's; the difference is ignored.
//   - SITELAT, SITELONG, SITEELEV: not in the standard, but written by
//     camera-control software, as decimal degrees or as sexagesimal strings
//     such as '-111 35 59', longitude east positive. SITEELEV is meters above
//     sea level, taken as the ellipsoidal height, and zero when absent.
//
// Until #582 only the last of these was read, and only as numbers: a header
// with the standard keywords, or with sexagesimal SITE keywords, was reported
// as missing them.
//
// The site is named by OBSERVAT, or "FITS Site" without one.
func SiteFromFITS(h *fits.Header) (*plan.Site, error) {
	geodetic, err := siteLocation(h)
	if err != nil {
		return nil, err
	}

	obsName, errObs := h.GetString("OBSERVAT")
	if errObs != nil || obsName == "" {
		obsName = "FITS Site"
	}

	site, err := plan.NewSite(obsName, geodetic)
	if err != nil {
		return nil, fmt.Errorf("fits/plan: new site: %w", err)
	}

	return site, nil
}

// siteLocation reads the first complete set of site keywords, in
// SiteFromFITS's order.
func siteLocation(h *fits.Header) (*coord.Geodetic, error) {
	xyz, ok, err := floats(h, "OBSGEO-X", "OBSGEO-Y", "OBSGEO-Z")
	if err != nil {
		return nil, err
	}

	if ok {
		g, err := coord.FromECEF(vector.V3(xyz[0], xyz[1], xyz[2]), coord.WGS84())
		if err != nil {
			return nil, fmt.Errorf("%w: OBSGEO-X/Y/Z: %w", ErrInvalidGeodetic, err)
		}

		return g, nil
	}

	blh, ok, err := floats(h, "OBSGEO-B", "OBSGEO-L", "OBSGEO-H")
	if err != nil {
		return nil, err
	}

	if ok {
		return geodetic(angle.Deg(blh[1]), angle.Deg(blh[0]), blh[2])
	}

	if !has(h, "SITELAT") || !has(h, "SITELONG") {
		return nil, ErrMissingSiteCoords
	}

	lat, err := sexagesimal(h, "SITELAT")
	if err != nil {
		return nil, err
	}

	lon, err := sexagesimal(h, "SITELONG")
	if err != nil {
		return nil, err
	}

	elev := 0.0

	if has(h, "SITEELEV") {
		if elev, err = h.GetFloat("SITEELEV"); err != nil {
			return nil, fmt.Errorf("%w: SITEELEV: %w", ErrInvalidGeodetic, err)
		}
	}

	return geodetic(lon, lat, elev)
}

func geodetic(lon, lat angle.Angle, heightMeters float64) (*coord.Geodetic, error) {
	g, err := coord.NewGeodetic(lon, lat, unit.Meters(heightMeters))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidGeodetic, err)
	}

	return g, nil
}

// floats reads keys as numbers. It reports ok only when all of them are
// present; some present without the rest is an error, not an absence.
func floats(h *fits.Header, keys ...string) (vals []float64, ok bool, err error) {
	present := 0

	for _, k := range keys {
		if has(h, k) {
			present++
		}
	}

	if present == 0 {
		return nil, false, nil
	}

	if present < len(keys) {
		return nil, false, fmt.Errorf("%w: %s needs all of %s", ErrInvalidGeodetic, keys[0], strings.Join(keys, ", "))
	}

	vals = make([]float64, len(keys))

	for i, k := range keys {
		if vals[i], err = h.GetFloat(k); err != nil {
			return nil, false, fmt.Errorf("%w: %s: %w", ErrInvalidGeodetic, k, err)
		}
	}

	return vals, true, nil
}

// sexagesimal reads key as degrees, either a number or a sexagesimal string.
func sexagesimal(h *fits.Header, key string) (angle.Angle, error) {
	s, err := h.GetString(key)
	if err != nil {
		return angle.Zero(), fmt.Errorf("%w: %s: %w", ErrInvalidGeodetic, key, err)
	}

	a, err := angle.ParseDMS(s)
	if err != nil {
		return angle.Zero(), fmt.Errorf("%w: %s %q: %w", ErrInvalidGeodetic, key, s, err)
	}

	return a, nil
}

func has(h *fits.Header, key string) bool {
	_, err := h.Get(key)

	return err == nil
}

// TargetFromFITS returns what a FITS image is pointed at: the world
// coordinate of the frame's centre, pixel ((NAXIS1+1)/2, (NAXIS2+1)/2), or of
// the reference pixel when the header gives no image size, through the
// header's own WCS ([fits.ExtractWCS]).
//
// The celestial axes are found by CTYPE, in either order:
//
//   - RA/DEC in ICRS, or in FK5 at J2000, whose 25 mas from ICRS is below
//     anything a pointing needs. RADESYS and EQUINOX default as the FITS
//     Standard 4.0 has them: FK4 when EQUINOX is before 1984, FK5 from 1984,
//     ICRS when EQUINOX is absent. Any other frame or equinox is
//     [ErrUnsupportedFrame], not a position in the wrong frame.
//   - GLON/GLAT, converted to ICRS.
//   - Ecliptic and other celestial axes are [ErrUnsupportedFrame].
//
// A header with no celestial axes falls back to the non-standard RA_DEG and
// DEC_DEG keywords, in degrees.
//
// Until #582 it returned CRVAL1 and CRVAL2 as right ascension and declination
// whatever CTYPE said, documented as the frame's centre: a reference pixel at
// the corner of a 4096-pixel frame at 1″ put the target 0.57° off in each
// coordinate, and the Galactic Centre came back as RA 0, Dec 0.
//
// The target is named by OBJECT, else OBJNAME, else "FITS Target".
func TargetFromFITS(h *fits.Header) (plan.Observable, error) {
	name, errName := h.GetString("OBJECT")
	if errName != nil {
		name, _ = h.GetString("OBJNAME")
	}

	if name == "" {
		name = "FITS Target"
	}

	pos, err := frameCentre(h)
	if err == nil {
		return plan.NewDeepSkyObject(name, pos.RA(), pos.Dec()), nil
	}

	if !errors.Is(err, errNoCelestialAxes) {
		return nil, err
	}

	ra, errRa := h.GetFloat("RA_DEG")
	if errRa != nil {
		return nil, ErrMissingRA
	}

	dec, errDec := h.GetFloat("DEC_DEG")
	if errDec != nil {
		return nil, ErrMissingDec
	}

	return plan.NewDeepSkyObject(name, angle.Deg(ra), angle.Deg(dec)), nil
}

// errNoCelestialAxes reports a header whose CTYPEs name no celestial pair,
// which TargetFromFITS answers from RA_DEG and DEC_DEG instead.
var errNoCelestialAxes = errors.New("fits/plan: no celestial axes in CTYPE")

type celestialKind int

const (
	equatorial celestialKind = iota + 1
	galactic
)

// frameCentre is the ICRS position of the frame's centre, or of the
// reference pixel when NAXISn is absent.
func frameCentre(h *fits.Header) (coord.ICRS, error) {
	lonAxis, latAxis, kind, err := celestialAxes(h)
	if err != nil {
		return coord.ICRS{}, err
	}

	lon, lat, err := worldAtCentre(h, lonAxis, latAxis)
	if err != nil {
		return coord.ICRS{}, err
	}

	if kind == galactic {
		return coord.GalacticToICRS(coord.NewGalactic(angle.Deg(lon), angle.Deg(lat))), nil
	}

	if err := checkEquatorialFrame(h); err != nil {
		return coord.ICRS{}, err
	}

	return coord.NewICRS(angle.Deg(lon), angle.Deg(lat)), nil
}

// celestialAxes finds the longitude and latitude axes, 0-indexed, by CTYPE.
func celestialAxes(h *fits.Header) (lonAxis, latAxis int, kind celestialKind, err error) {
	lonAxis, latAxis = -1, -1

	for i := range 9 {
		ctype, errC := h.GetString(fmt.Sprintf("CTYPE%d", i+1))
		if errC != nil {
			continue
		}

		axis := strings.ToUpper(strings.TrimSpace(ctype))
		coordType, _, _ := strings.Cut(axis, "-")

		switch coordType {
		case "RA":
			lonAxis, kind = i, equatorial
		case "DEC":
			latAxis = i
		case "GLON":
			lonAxis, kind = i, galactic
		case "GLAT":
			latAxis = i
		case "ELON", "ELAT", "HLON", "HLAT", "SLON", "SLAT":
			return 0, 0, 0, fmt.Errorf("%w: CTYPE%d %q", ErrUnsupportedFrame, i+1, ctype)
		}
	}

	if lonAxis < 0 || latAxis < 0 {
		return 0, 0, 0, errNoCelestialAxes
	}

	return lonAxis, latAxis, kind, nil
}

// worldAtCentre evaluates the WCS at the frame's centre. With no NAXIS, there
// is no frame to take the centre of, and the reference pixel's world
// coordinate is CRVAL by definition.
func worldAtCentre(h *fits.Header, lonAxis, latAxis int) (lon, lat float64, err error) {
	if !has(h, "NAXIS") {
		lon, errLon := h.GetFloat(fmt.Sprintf("CRVAL%d", lonAxis+1))
		lat, errLat := h.GetFloat(fmt.Sprintf("CRVAL%d", latAxis+1))

		if errLon != nil {
			return 0, 0, ErrMissingRA
		}

		if errLat != nil {
			return 0, 0, ErrMissingDec
		}

		return lon, lat, nil
	}

	w, err := fits.ExtractWCS(h)
	if err != nil {
		return 0, 0, fmt.Errorf("fits/plan: WCS: %w", err)
	}

	pixel := w.CRPIX()

	for i := range pixel {
		if n, errN := h.GetInt(fmt.Sprintf("NAXIS%d", i+1)); errN == nil && n > 0 {
			pixel[i] = (float64(n) + 1) / 2
		}
	}

	world, err := w.PixelToWorld(pixel)
	if err != nil {
		return 0, 0, fmt.Errorf("fits/plan: WCS at the frame centre: %w", err)
	}

	return world[lonAxis], world[latAxis], nil
}

// checkEquatorialFrame accepts ICRS, and FK5 at J2000, by RADESYS and
// EQUINOX with the FITS Standard 4.0's defaults.
func checkEquatorialFrame(h *fits.Header) error {
	radesys, errSys := h.GetString("RADESYS")
	radesys = strings.ToUpper(strings.TrimSpace(radesys))

	equinox, errEq := h.GetFloat("EQUINOX")
	hasEquinox := errEq == nil

	if errSys != nil || radesys == "" {
		switch {
		case !hasEquinox:
			radesys = "ICRS"
		case equinox < 1984:
			radesys = "FK4"
		default:
			radesys = "FK5"
		}
	}

	switch {
	case radesys == "ICRS":
		return nil
	case radesys == "FK5" && (!hasEquinox || equinox == 2000):
		return nil
	case hasEquinox:
		return fmt.Errorf("%w: RADESYS %s at EQUINOX %g", ErrUnsupportedFrame, radesys, equinox)
	default:
		return fmt.Errorf("%w: RADESYS %s", ErrUnsupportedFrame, radesys)
	}
}
