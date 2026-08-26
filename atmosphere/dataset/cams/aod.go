package cams

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/remote"
)

// ErrAOD reports that an aerosol optical depth could not be resolved.
var ErrAOD = errors.New("cams: aerosol optical depth")

// RegistrationAdvice is appended to a failed fetch, because by far the most
// likely cause is that the caller has no Copernicus account yet.
//
// The account is free and self-service, and it has to be the caller's own:
// credentials are per-user, sharing them breaches the terms they were issued
// under, and anything done with a shared key traces back to whoever
// registered it. So the useful thing to hand somebody here is the URL, not a
// key.
const RegistrationAdvice = "Reaching Copernicus needs your own credentials, which are free:\n" +
	"  1. register at https://dataspace.copernicus.eu\n" +
	"  2. create S3 credentials in the dashboard\n" +
	"  3. export them as AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY, which the\n" +
	"     AWS default chain resolves; astrogo reads no credential file of its own\n" +
	"  4. blank-import github.com/TuSKan/astrogo/remote/s3, and grant\n" +
	"     remote.EnableDownloads for remote.CopernicusEODATA"

// AODVariable is the CAMS variable this reads: "Total Aerosol Optical Depth
// at 550nm", summed over every aerosol species the model carries.
//
// 550 nm because that is the reference wavelength the Angstrom law in
// [github.com/TuSKan/astrogo/atmosphere.Aerosol] extrapolates from, and
// because it is where the aerosol presets are tabulated. The same files carry
// aod469, aod670, aod865 and aod1240, and per-species breakdowns
// (duaod550 for dust, bcaod550 for black carbon, and so on), for a caller
// who opens them directly.
const AODVariable = "aod550"

// AOD550 returns the total aerosol optical depth at 550 nm above a site, for
// an instant.
//
// # Why this rather than a number in the source
//
// Aerosol optical depth is the one aerosol quantity that cannot be a
// constant: it is how much aerosol is overhead right now, it moves by an
// order of magnitude at one site across a year, and every other aerosol
// property this module uses is a property of the aerosol *type* and is
// legitimately fixed. So the presets in
// [github.com/TuSKan/astrogo/atmosphere] take it as a parameter, and this is
// where that parameter can come from instead of from a guess.
//
// # Why CAMS and not a satellite retrieval
//
// Because this is a night-time model. A passive optical instrument infers
// aerosol optical depth from reflected sunlight, so satellite retrievals
// exist only in daylight and can never describe the hour being modelled.
// CAMS is an assimilating forecast model: it has a value at every hour,
// including the middle of the night, which is the whole reason it is the
// source here.
//
// # What it is not
//
// A measurement. It is a model analysis, roughly 0.4 degrees — about 40 km —
// so it describes the regional background rather than the air over one
// building, and it will not see a local dust plume or a nearby fire. Treat it
// as [github.com/TuSKan/astrogo/atmosphere.FidelityModelPropagated], and take
// an AERONET station's value in preference where one is close enough.
//
// # Access
//
// One file of about 1.5 MB per hour, fetched through [remote.GetFile] and
// cached. It is a download, so it is gated: grant
// [remote.EnableDownloads] for [remote.CopernicusEODATA] first. The caller
// must also blank-import remote/s3, which this package deliberately does not
// — it knows nothing about S3, and pulling the AWS SDK into every build that
// merely reads a NetCDF file would be the wrong trade.
func AOD550(ctx context.Context, site *coord.Geodetic, when time.Time) (float64, error) {
	if site == nil {
		return 0, fmt.Errorf("%w: needs a site", ErrAOD)
	}

	bucket, key, err := remote.GetFile(ctx, remote.CopernicusEODATA, AODKey(when))
	if err != nil {
		return 0, fmt.Errorf("%w: %w\n\n%s", ErrAOD, err, RegistrationAdvice)
	}

	f, err := Open(ctx, bucket, key)
	if err != nil {
		return 0, fmt.Errorf("%w: %w", ErrAOD, err)
	}

	defer func() { _ = f.Close() }()

	v, err := f.Var(AODVariable)
	if err != nil {
		return 0, fmt.Errorf("%w: %w", ErrAOD, err)
	}

	lat, lon, err := gridIndex(f.Dims(), site)
	if err != nil {
		return 0, err
	}

	// One time step per file, and this variable has no vertical level: the
	// column is what an optical depth already is.
	aod, err := v.At(0, 0, lat, lon)
	if err != nil {
		return 0, fmt.Errorf("%w: %w", ErrAOD, err)
	}

	if aod < 0 || math.IsNaN(aod) || math.IsInf(aod, 0) {
		return 0, fmt.Errorf("%w: the grid holds %g at lat %d lon %d, which is not an "+
			"optical depth", ErrAOD, aod, lat, lon)
	}

	return aod, nil
}

// AODKey is the object key of the CAMS analysis or forecast covering an
// instant.
//
// CAMS runs two cycles a day, at 00Z and 12Z, and publishes hourly forecast
// steps from each. This takes the most recent cycle at or before the instant
// and the step that lands on it, so any hour resolves to the freshest run
// that covers it rather than to whichever cycle happens to be nearer.
func AODKey(when time.Time) string {
	utc := when.UTC()

	cycleHour := 0
	if utc.Hour() >= 12 {
		cycleHour = 12
	}

	cycle := time.Date(utc.Year(), utc.Month(), utc.Day(), cycleHour, 0, 0, 0, time.UTC)
	step := int(utc.Sub(cycle).Hours())

	stamp := cycle.Format("20060102150405")
	name := fmt.Sprintf("z_cams_c_ecmf_%s_prod_fc_sfc_%03d_%s", stamp, step, AODVariable)

	return fmt.Sprintf("CAMS/GLOBAL/%04d/%02d/%02d/%s/%s.nc",
		cycle.Year(), cycle.Month(), cycle.Day(), name, name)
}

// gridIndex maps a site onto the file's own grid.
//
// # Why the steps are derived rather than written down
//
// The grid is regular in latitude and longitude, and its spacing follows from
// the dimensions the file itself reports: 451 by 900 is the 0.4-degree global
// grid, and a file at another resolution changes those numbers rather than
// needing different code. Hard-coding 0.4 would silently read the wrong cell
// the day a finer product appears, and an index error here is invisible — an
// off-by-one is a point 40 km away that still returns a perfectly plausible
// optical depth.
//
// The orientation is the ECMWF convention these files are written in:
// latitude descending from +90, longitude ascending from 0 through 360. That
// is an assumption about the data rather than something the reader can see,
// which is why TestAODOrientationIsSane checks it against physical geography
// instead of trusting it.
func gridIndex(dims map[string]int, site *coord.Geodetic) (lat, lon int, err error) {
	nLat, okLat := dims["latitude"]
	nLon, okLon := dims["longitude"]

	if !okLat || !okLon || nLat < 2 || nLon < 1 {
		return 0, 0, fmt.Errorf("%w: the file reports dimensions %v, which is not a "+
			"latitude-longitude grid", ErrAOD, dims)
	}

	// Latitude runs from +90 to -90 inclusive, so nLat points span 180
	// degrees in nLat-1 steps.
	latStep := 180.0 / float64(nLat-1)
	lat = int(math.Round((90 - site.Lat().Degrees()) / latStep))
	lat = min(max(lat, 0), nLat-1)

	// Longitude wraps, so nLon points span the full 360 with no repeat of the
	// prime meridian.
	lonStep := 360.0 / float64(nLon)

	deg := math.Mod(site.Lon().Degrees(), 360)
	if deg < 0 {
		deg += 360
	}

	lon = int(math.Round(deg/lonStep)) % nLon

	return lat, lon, nil
}
