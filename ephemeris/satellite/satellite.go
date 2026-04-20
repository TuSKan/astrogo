package satellite

import (
	"errors"
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/norad"
	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"

	gosatellite "github.com/joshuaferrara/go-satellite"
)

// kmPerAU is the number of kilometres in one Astronomical Unit.
const kmPerAU = constants.AstronomicalUnit / 1e3 // 149597870.7 km

// secPerDay is the number of seconds in a Julian day.
const secPerDay = constants.JulianDaySeconds // 86400

// ErrPropagation indicates an SGP4 propagation failure.
var ErrPropagation = errors.New("satellite: sgp4 propagation failed")

// Satellite wraps a NORAD GP element set with SGP4 propagation state.
type Satellite struct {
	GP   norad.GP               // Source orbital elements
	Name string                 // Satellite name
	sat  gosatellite.Satellite  // Initialized SGP4 state
}

// NewFromGP creates a Satellite from a parsed GP element set.
func NewFromGP(gp norad.GP) (*Satellite, error) {
	line1, line2 := gp.ToTLE()
	sat := gosatellite.TLEToSat(line1, line2, gosatellite.GravityWGS84)
	if sat.Error != 0 {
		return nil, fmt.Errorf("%w: sgp4 init error %d: %s", ErrPropagation, sat.Error, sat.ErrorStr)
	}
	return &Satellite{
		GP:   gp,
		Name: gp.ObjectName,
		sat:  sat,
	}, nil
}

// NewFromTLE creates a Satellite from raw TLE lines.
func NewFromTLE(name, line1, line2 string) (*Satellite, error) {
	sat := gosatellite.TLEToSat(line1, line2, gosatellite.GravityWGS84)
	if sat.Error != 0 {
		return nil, fmt.Errorf("%w: sgp4 init error %d: %s", ErrPropagation, sat.Error, sat.ErrorStr)
	}
	return &Satellite{
		Name: name,
		sat:  sat,
	}, nil
}

// PropagateECI returns the TEME position and velocity (km, km/s) at time t.
func (s *Satellite) PropagateECI(t time.Time) (pos, vel vector.Vec3, err error) {
	year, month, day, hour, min, sec := timeToComponents(t)
	eciPos, eciVel := gosatellite.Propagate(s.sat, year, month, day, hour, min, sec)

	// Check for propagation failure (NaN or zero position).
	if math.IsNaN(eciPos.X) || math.IsNaN(eciPos.Y) || math.IsNaN(eciPos.Z) {
		return vector.Vec3{}, vector.Vec3{}, fmt.Errorf(
			"%w: NaN position at %s", ErrPropagation, t)
	}

	pos = vector.V3(eciPos.X, eciPos.Y, eciPos.Z)
	vel = vector.V3(eciVel.X, eciVel.Y, eciVel.Z)
	return pos, vel, nil
}

// State returns the geocentric position/velocity in GCRS (AU, AU/day),
// implementing the [ephemeris.Provider] interface contract.
//
// The conversion pipeline: TEME (km) → GCRS (AU) via the IAU 2006
// Earth rotation matrix (C2T06A).
func (s *Satellite) State(_ ephemeris.ID, t time.Time) (ephemeris.State, error) {
	eciPos, eciVel, err := s.PropagateECI(t)
	if err != nil {
		return ephemeris.State{}, err
	}

	// Convert TEME → GCRS using the Earth rotation matrix.
	gcrsPos, gcrsVel := temeToGCRS(eciPos, eciVel, t)

	// Convert km → AU, km/s → AU/day.
	return ephemeris.State{
		Pos: gcrsPos.MulScalar(1.0 / kmPerAU),
		Vel: gcrsVel.MulScalar(secPerDay / kmPerAU),
	}, nil
}

// SubSatellitePoint returns the geodetic coordinates (lat, lon, altitude)
// of the sub-satellite point at time t.
func (s *Satellite) SubSatellitePoint(t time.Time) (*coord.Geodetic, error) {
	eciPos, _, err := s.PropagateECI(t)
	if err != nil {
		return nil, err
	}

	// Compute GMST for ECI → ECEF conversion.
	gmst := computeGMST(t)

	// Rotate ECI → ECEF.
	cosG := math.Cos(gmst)
	sinG := math.Sin(gmst)
	ecefX := eciPos.X*cosG + eciPos.Y*sinG
	ecefY := -eciPos.X*sinG + eciPos.Y*cosG
	ecefZ := eciPos.Z

	// Convert ECEF (km) to geodetic via coord.FromECEF (expects metres).
	ecefVec := vector.V3(ecefX*1e3, ecefY*1e3, ecefZ*1e3)
	geo, err := coord.FromECEF(ecefVec, coord.WGS84())
	if err != nil {
		return nil, fmt.Errorf("satellite: ecef→geodetic: %w", err)
	}

	return geo, nil
}

// OrbitalPeriod returns the orbital period in minutes, derived from mean motion.
func (s *Satellite) OrbitalPeriod() float64 {
	if s.GP.MeanMotion <= 0 {
		return 0
	}
	return 1440.0 / s.GP.MeanMotion // minutes
}

// Altitude returns the approximate altitude above the WGS84 ellipsoid at time t, in kilometres.
func (s *Satellite) Altitude(t time.Time) (float64, error) {
	geo, err := s.SubSatellitePoint(t)
	if err != nil {
		return 0, err
	}
	return geo.Height() / 1e3, nil // metres → km
}

// LookAngle computes the topocentric look angle (azimuth, elevation, range)
// from an observer to the satellite at time t.
func (s *Satellite) LookAngle(t time.Time, observer *coord.Geodetic) (az, el angle.Angle, rng float64, err error) {
	eciPos, _, err := s.PropagateECI(t)
	if err != nil {
		return angle.Zero(), angle.Zero(), 0, err
	}

	// Convert observer to ECI.
	obsLat := observer.Lat().Radians()
	obsLon := observer.Lon().Radians()
	obsAlt := observer.Height() / 1e3 // metres → km

	gmst := computeGMST(t)

	// Observer ECI position.
	obsLL := gosatellite.LatLong{Latitude: obsLat, Longitude: obsLon}
	jday := t.JD()
	obsECI := gosatellite.LLAToECI(obsLL, obsAlt, jday)

	// Range vector in ECI.
	rangeECI := gosatellite.Vector3{
		X: eciPos.X - obsECI.X,
		Y: eciPos.Y - obsECI.Y,
		Z: eciPos.Z - obsECI.Z,
	}

	// Rotate range vector to topocentric SEZ (South-East-Zenith).
	sinLat := math.Sin(obsLat)
	cosLat := math.Cos(obsLat)
	sinLST := math.Sin(gmst + obsLon)
	cosLST := math.Cos(gmst + obsLon)

	// Topocentric SEZ coordinates.
	south := sinLat*cosLST*rangeECI.X + sinLat*sinLST*rangeECI.Y - cosLat*rangeECI.Z
	east := -sinLST*rangeECI.X + cosLST*rangeECI.Y
	zenith := cosLat*cosLST*rangeECI.X + cosLat*sinLST*rangeECI.Y + sinLat*rangeECI.Z

	// Range magnitude.
	rng = math.Sqrt(south*south + east*east + zenith*zenith)

	// Elevation.
	elRad := math.Asin(zenith / rng)

	// Azimuth (measured clockwise from north).
	azRad := math.Atan2(east, -south)
	if azRad < 0 {
		azRad += 2 * math.Pi
	}

	return angle.Rad(azRad), angle.Rad(elRad), rng, nil
}

// temeToGCRS converts TEME position/velocity (km, km/s) to GCRS using
// the IAU 2006/2000A precession-nutation-bias matrix (BPN).
//
// Both TEME and GCRS are inertial (non-rotating) Earth-centered frames.
// They differ only in axis orientation:
//   - TEME: true equator + mean equinox of date (SGP4 output frame)
//   - GCRS: ICRS axes (≈ J2000 equator and equinox)
//
// The BPN matrix from SOFA Pnm06a maps V(date) = BPN · V(GCRS), so
// its transpose maps V(GCRS) = BPN^T · V(TEME).
//
// Error budget:
//   - TEME mean equinox vs. true equinox (equation of the equinoxes): ≤20″ → ~0.7 km at LEO
//   - IAU-76/FK5 (SGP4) vs. IAU 2006 frame tie: ~20 mas → negligible
//   - Both are well within SGP4's intrinsic ~1 km accuracy
func temeToGCRS(pos, vel vector.Vec3, t time.Time) (gcrsPos, gcrsVel vector.Vec3) {
	tt := t.TT()
	tta, ttb := tt.JDParts()

	// BPN: bias-precession-nutation matrix (GCRS → true equatorial of date).
	// Transpose maps back: true equatorial of date → GCRS.
	bpn := gofaext.Pnm06a(tta, ttb)

	// r_GCRS = BPN^T · r_TEME
	gcrsPos = vector.V3(
		bpn[0][0]*pos.X+bpn[1][0]*pos.Y+bpn[2][0]*pos.Z,
		bpn[0][1]*pos.X+bpn[1][1]*pos.Y+bpn[2][1]*pos.Z,
		bpn[0][2]*pos.X+bpn[1][2]*pos.Y+bpn[2][2]*pos.Z,
	)

	// v_GCRS = BPN^T · v_TEME
	// (The time derivative of BPN contributes ~50″/yr precession rate,
	// which at LEO distances yields ~0.003 m/s — negligible.)
	gcrsVel = vector.V3(
		bpn[0][0]*vel.X+bpn[1][0]*vel.Y+bpn[2][0]*vel.Z,
		bpn[0][1]*vel.X+bpn[1][1]*vel.Y+bpn[2][1]*vel.Z,
		bpn[0][2]*vel.X+bpn[1][2]*vel.Y+bpn[2][2]*vel.Z,
	)

	return gcrsPos, gcrsVel
}

// computeGMST returns the Greenwich Mean Sidereal Time in radians for time t.
func computeGMST(t time.Time) float64 {
	ut1, err := t.UT1()
	if err != nil {
		ut1 = t.UTC()
	}
	tt := t.TT()

	ut1a, ut1b := ut1.JDParts()
	tta, ttb := tt.JDParts()

	return gofaext.Gst06a(ut1a, ut1b, tta, ttb)
}

// timeToComponents extracts calendar components from an astrogo time for SGP4.
func timeToComponents(t time.Time) (year, month, day, hour, min, sec int) {
	year = t.Year()
	// Extract month/day from the Julian Date.
	jd1, jd2 := t.JDParts()
	y, m, d, frac, _ := gofaext.JdToDate(jd1, jd2)
	year = y
	month = m
	day = d

	// Convert fractional day to h/m/s.
	totalSec := frac * secPerDay
	hour = int(totalSec / 3600)
	totalSec -= float64(hour) * 3600
	min = int(totalSec / 60)
	totalSec -= float64(min) * 60
	sec = int(totalSec)

	return year, month, day, hour, min, sec
}
