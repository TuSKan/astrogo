package plan

import (
	"fmt"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/fits"
	"github.com/TuSKan/astrogo/plan"
)

// SiteFromFITS extracts observatory location metadata from standard FITS keywords
// (SITELONG, SITELAT, SITEELEV) and returns a plan.Site matching the observation origin.
func SiteFromFITS(h *fits.Header) (*plan.Site, error) {
	lon, errLon := h.GetFloat("SITELONG")
	lat, errLat := h.GetFloat("SITELAT")

	if errLon != nil || errLat != nil {
		return nil, ErrMissingSiteCoords
	}

	elev, errElev := h.GetFloat("SITEELEV")
	if errElev != nil {
		elev = 0 // Assume sea level if elevation is absent
	}

	geodetic, errGeo := coord.NewGeodetic(angle.Deg(lon), angle.Deg(lat), elev)
	if errGeo != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidGeodetic, errGeo)
	}

	obsName, errObs := h.GetString("OBSERVAT")
	if errObs != nil || obsName == "" {
		obsName = "FITS Site"
	}

	// Assuming 0 horizon limitation as standard ingestion
	site, err := plan.NewSite(obsName, geodetic, angle.Deg(0), nil)
	if err != nil {
		return nil, fmt.Errorf("fits/plan: new site: %w", err)
	}

	return site, nil
}

// TargetFromFITS extracts observation pointing coordinates constructing a Custom plan target.
// It prioritizes standard numeric World Coordinate System reference pixels (CRVAL1, CRVAL2)
// representing the geometric center of the sensor frame.
func TargetFromFITS(h *fits.Header) (plan.Observable, error) {
	name, errName := h.GetString("OBJECT")
	if errName != nil {
		name, _ = h.GetString("OBJNAME")
	}

	if name == "" {
		name = "FITS Target"
	}

	ra, errRa := h.GetFloat("CRVAL1")
	if errRa != nil {
		// Provide an explicit fallback trying to grab standard numeric RA if CRVAL is missing.
		if explicit, errExp := h.GetFloat("RA_DEG"); errExp == nil {
			ra = explicit
		} else {
			return nil, ErrMissingRA
		}
	}

	dec, errDec := h.GetFloat("CRVAL2")
	if errDec != nil {
		if explicit, errExp := h.GetFloat("DEC_DEG"); errExp == nil {
			dec = explicit
		} else {
			return nil, ErrMissingDec
		}
	}

	return plan.NewDeepSkyObject(name, angle.Deg(ra), angle.Deg(dec)), nil
}
