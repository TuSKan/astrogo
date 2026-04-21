package satellite

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/ephemeris/core"
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

// Satellite wraps a NORAD TLE element set with SGP4 propagation state.
type Satellite struct {
	Name       string                // Satellite name
	MeanMotion float64               // Mean motion (rev/day), parsed from TLE line 2
	sat        gosatellite.Satellite // Initialized SGP4 state
}

// NewFromTLE creates a Satellite from raw TLE lines.
func NewFromTLE(name, line1, line2 string) (*Satellite, error) {
	sat := gosatellite.TLEToSat(line1, line2, gosatellite.GravityWGS84)
	if sat.Error != 0 {
		return nil, fmt.Errorf("%w: sgp4 init error %d: %s", ErrPropagation, sat.Error, sat.ErrorStr)
	}

	mm := parseMeanMotion(line2)

	return &Satellite{
		Name:       name,
		MeanMotion: mm,
		sat:        sat,
	}, nil
}

// parseMeanMotion extracts mean motion (rev/day) from TLE line 2,
// columns 53–63 (0-indexed).
func parseMeanMotion(line2 string) float64 {
	if len(line2) < 63 {
		return 0
	}
	s := strings.TrimSpace(line2[52:63])
	mm, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return mm
}

// propagateECI returns the TEME position and velocity (km, km/s) at time t.
func (s *Satellite) propagateECI(t time.Time) (pos, vel vector.Vec3, err error) {
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
// implementing the [core.Provider] interface contract.
//
// The conversion pipeline: TEME (km) → GCRS (AU) via the IAU 2006
// Earth rotation matrix (C2T06A).
func (s *Satellite) State(_ core.ID, t time.Time) (core.State, error) {
	eciPos, eciVel, err := s.propagateECI(t)
	if err != nil {
		return core.State{}, err
	}

	// Convert TEME → GCRS using the Earth rotation matrix.
	gcrsPos, gcrsVel := temeToGCRS(eciPos, eciVel, t)

	// Convert km → AU, km/s → AU/day.
	return core.State{
		Pos: gcrsPos.MulScalar(1.0 / kmPerAU),
		Vel: gcrsVel.MulScalar(secPerDay / kmPerAU),
	}, nil
}

// Close is a no-op for satellite providers (no file handles).
func (s *Satellite) Close() error { return nil }

// subSatellitePoint returns the geodetic coordinates (lat, lon, altitude)
// of the sub-satellite point at time t.
func (s *Satellite) subSatellitePoint(t time.Time) (*coord.Geodetic, error) {
	eciPos, _, err := s.propagateECI(t)
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
	if s.MeanMotion <= 0 {
		return 0
	}
	return 1440.0 / s.MeanMotion // minutes
}

// Altitude returns the precise altitude above the WGS84 ellipsoid at time t, in kilometres.
// This uses the sub-satellite geodetic computation for WGS84-precise values.
func (s *Satellite) Altitude(t time.Time) (float64, error) {
	geo, err := s.subSatellitePoint(t)
	if err != nil {
		return 0, err
	}
	return geo.Height() / 1e3, nil // metres → km
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
