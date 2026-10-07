package plan

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/fits"
	"github.com/TuSKan/astrogo/time"
)

func TestSiteFromFITS(t *testing.T) {
	h := fits.NewHeader()
	h.Append(fits.Card{Keyword: "SITELONG", Value: "149.0661"}) // Siding Spring
	h.Append(fits.Card{Keyword: "SITELAT", Value: "-31.2770"})
	h.Append(fits.Card{Keyword: "SITEELEV", Value: "1165.0"})
	h.Append(fits.Card{Keyword: "OBSERVAT", Value: "'AAO'"})

	site, err := SiteFromFITS(h)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if site.Name() != "AAO" {
		t.Errorf("expected site name AAO, got: %s", site.Name())
	}

	if site.Longitude().Degrees() != 149.0661 {
		t.Errorf("expected longitude 149.0661, got: %v", site.Longitude().Degrees())
	}

	if site.Latitude().Degrees() != -31.2770 {
		t.Errorf("expected latitude -31.2770, got: %v", site.Latitude().Degrees())
	}

	if site.Height().Meters() != 1165.0 {
		t.Errorf("expected elevation 1165.0 m, got: %v", site.Height().Meters())
	}
}

func TestTargetFromFITS(t *testing.T) {
	h := fits.NewHeader()
	h.Append(fits.Card{Keyword: "OBJECT", Value: "'M42'"})
	h.Append(fits.Card{Keyword: "CTYPE1", Value: "'RA---TAN'"})
	h.Append(fits.Card{Keyword: "CTYPE2", Value: "'DEC--TAN'"})
	h.Append(fits.Card{Keyword: "CRVAL1", Value: "83.82208"})
	h.Append(fits.Card{Keyword: "CRVAL2", Value: "-5.39111"})

	target, err := TargetFromFITS(h)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if target.Name() != "M42" {
		t.Errorf("expected name M42, got: %s", target.Name())
	}

	// test RA and Dec
	pos, _ := target.Position(time.Time{})
	if pos.RA().Degrees() != 83.82208 {
		t.Errorf("expected RA 83.82208, got: %v", pos.RA().Degrees())
	}

	if pos.Dec().Degrees() != -5.39111 {
		t.Errorf("expected Dec -5.39111, got: %v", pos.Dec().Degrees())
	}
}

func header(cards ...[2]string) *fits.Header {
	h := fits.NewHeader()
	for _, c := range cards {
		h.Append(fits.Card{Keyword: c[0], Value: c[1]})
	}

	return h
}

// TestSiteFromFITSReadsTheStandardKeywords: the FITS Standard 4.0's observatory
// keywords, OBSGEO-X/Y/Z and OBSGEO-B/L/H, and SITELAT/SITELONG written as
// sexagesimal strings (#582). Each of these was reported as missing.
func TestSiteFromFITSReadsTheStandardKeywords(t *testing.T) {
	t.Parallel()

	// Kitt Peak, 31.9583°N 111.5967°W 2096 m; ITRS by astropy 8.0.1's
	// EarthLocation.from_geodetic on WGS 84.
	for _, c := range []struct {
		name          string
		h             *fits.Header
		lat, lon, elv float64
	}{
		{"OBSGEO-X/Y/Z", header([2]string{"OBSGEO-X", "-1994313.7434"}, [2]string{"OBSGEO-Y", "-5037909.2658"}, [2]string{"OBSGEO-Z", "3357618.6151"}),
			31.9583, -111.5967, 2096},
		{"OBSGEO-B/L/H", header([2]string{"OBSGEO-B", "31.9583"}, [2]string{"OBSGEO-L", "-111.5967"}, [2]string{"OBSGEO-H", "2096"}),
			31.9583, -111.5967, 2096},
		{"sexagesimal SITE keywords", header([2]string{"SITELAT", "'+31 57 31'"}, [2]string{"SITELONG", "'-111 35 59'"}, [2]string{"SITEELEV", "2096"}),
			31 + 57.0/60 + 31.0/3600, -(111 + 35.0/60 + 59.0/3600), 2096},
		// The preferred set wins when a header carries two.
		{"OBSGEO-X/Y/Z over SITE", header([2]string{"SITELAT", "0"}, [2]string{"SITELONG", "0"},
			[2]string{"OBSGEO-X", "-1994313.7434"}, [2]string{"OBSGEO-Y", "-5037909.2658"}, [2]string{"OBSGEO-Z", "3357618.6151"}),
			31.9583, -111.5967, 2096},
	} {
		site, err := SiteFromFITS(c.h)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)

			continue
		}

		if math.Abs(site.Latitude().Degrees()-c.lat) > 1e-8 || math.Abs(site.Longitude().Degrees()-c.lon) > 1e-8 ||
			math.Abs(site.Height().Meters()-c.elv) > 1e-3 {
			t.Errorf("%s: %.8f° %.8f° %.4f m, want %.8f° %.8f° %.4f m", c.name,
				site.Latitude().Degrees(), site.Longitude().Degrees(), site.Height().Meters(), c.lat, c.lon, c.elv)
		}
	}
}

// TestSiteFromFITSRefusesAnIncompleteSet: two of the three OBSGEO-X/Y/Z is a
// broken header, not an absent one, and says so rather than falling through.
func TestSiteFromFITSRefusesAnIncompleteSet(t *testing.T) {
	t.Parallel()

	_, err := SiteFromFITS(header([2]string{"OBSGEO-X", "-1994313.7"}, [2]string{"OBSGEO-Y", "-5037909.3"}))
	if !errors.Is(err, ErrInvalidGeodetic) {
		t.Errorf("OBSGEO-X and -Y without -Z: %v, want ErrInvalidGeodetic", err)
	}

	_, err = SiteFromFITS(header([2]string{"OBSERVAT", "'nowhere'"}))
	if !errors.Is(err, ErrMissingSiteCoords) {
		t.Errorf("no site keywords: %v, want ErrMissingSiteCoords", err)
	}
}

// TestTargetFromFITSReadsTheWCS: the frame centre through the header's WCS, in
// the frame CTYPE and RADESYS name (#582). Each case was read as CRVAL1 and
// CRVAL2 in right ascension and declination.
func TestTargetFromFITSReadsTheWCS(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name    string
		h       *fits.Header
		ra, dec float64
		tol     float64 // degrees; zero means 2e-6
	}{
		// The reference pixel at the corner of a 4096-pixel frame at 1"; the
		// centre by astropy 8.0.1's wcs_pix2world.
		{"reference pixel at the corner", header([2]string{"NAXIS", "2"}, [2]string{"NAXIS1", "4096"}, [2]string{"NAXIS2", "4096"},
			[2]string{"CTYPE1", "'RA---TAN'"}, [2]string{"CTYPE2", "'DEC--TAN'"},
			[2]string{"CRPIX1", "1"}, [2]string{"CRPIX2", "1"}, [2]string{"CRVAL1", "83.82208"}, [2]string{"CRVAL2", "-5.39111"},
			[2]string{"CDELT1", "-0.000277778"}, [2]string{"CDELT2", "0.000277778"}),
			83.2513561, -4.8221401, 0},
		// The Galactic Centre; ICRS by astropy's SkyCoord. astropy defines the
		// galactic frame through FK5, SOFA's iauG2icrs on ICRS through the
		// Hipparcos frame, and the two differ by about 25 mas: 0.024" here.
		{"galactic axes", header([2]string{"CTYPE1", "'GLON-CAR'"}, [2]string{"CTYPE2", "'GLAT-CAR'"},
			[2]string{"CRVAL1", "0"}, [2]string{"CRVAL2", "0"}),
			266.404988, -28.936178, 1e-5},
		{"declination first", header([2]string{"CTYPE1", "'DEC--TAN'"}, [2]string{"CTYPE2", "'RA---TAN'"},
			[2]string{"CRVAL1", "-5.39111"}, [2]string{"CRVAL2", "83.82208"}),
			83.82208, -5.39111, 0},
		{"FK5 at J2000", header([2]string{"CTYPE1", "'RA---TAN'"}, [2]string{"CTYPE2", "'DEC--TAN'"},
			[2]string{"RADESYS", "'FK5'"}, [2]string{"EQUINOX", "2000.0"},
			[2]string{"CRVAL1", "83.82208"}, [2]string{"CRVAL2", "-5.39111"}),
			83.82208, -5.39111, 0},
		{"no celestial WCS, RA_DEG", header([2]string{"RA_DEG", "83.82208"}, [2]string{"DEC_DEG", "-5.39111"}),
			83.82208, -5.39111, 0},
	} {
		target, err := TargetFromFITS(c.h)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)

			continue
		}

		tol := c.tol
		if tol == 0 {
			tol = 2e-6
		}

		pos, _ := target.Position(time.Time{})
		if math.Abs(pos.RA().Degrees()-c.ra) > tol || math.Abs(pos.Dec().Degrees()-c.dec) > tol {
			t.Errorf("%s: (%.7f°, %.7f°), want (%.7f°, %.7f°)", c.name, pos.RA().Degrees(), pos.Dec().Degrees(), c.ra, c.dec)
		}
	}
}

// TestTargetFromFITSRefusesOtherFrames: a frame TargetFromFITS does not
// convert is an error, not a position in the wrong frame. FK4 B1950 is 0.7°
// from ICRS, and FITS defaults RADESYS to FK4 for an EQUINOX before 1984.
func TestTargetFromFITSRefusesOtherFrames(t *testing.T) {
	t.Parallel()

	radec := [][2]string{{"CTYPE1", "'RA---TAN'"}, {"CTYPE2", "'DEC--TAN'"}, {"CRVAL1", "83.0"}, {"CRVAL2", "-5.0"}}

	for _, c := range []struct {
		name  string
		extra [][2]string
		ctype [][2]string
	}{
		{"RADESYS FK4", [][2]string{{"RADESYS", "'FK4'"}, {"EQUINOX", "1950.0"}}, nil},
		{"EQUINOX 1950 without RADESYS", [][2]string{{"EQUINOX", "1950.0"}}, nil},
		{"FK5 at J2015", [][2]string{{"RADESYS", "'FK5'"}, {"EQUINOX", "2015.0"}}, nil},
		{"ecliptic axes", nil, [][2]string{{"CTYPE1", "'ELON-TAN'"}, {"CTYPE2", "'ELAT-TAN'"}, {"CRVAL1", "83.0"}, {"CRVAL2", "-5.0"}}},
	} {
		cards := radec
		if c.ctype != nil {
			cards = c.ctype
		}

		_, err := TargetFromFITS(header(append(append([][2]string{}, cards...), c.extra...)...))
		if !errors.Is(err, ErrUnsupportedFrame) {
			t.Errorf("%s: %v, want ErrUnsupportedFrame", c.name, err)
		}
	}
}
