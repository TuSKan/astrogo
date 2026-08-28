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
//
// var, not const: constants.IAU.AstronomicalUnit is a struct, and Go does
// not permit selecting a struct field inside a constant expression.
// constants.IAU (not IAU2015) so this automatically tracks whichever IAU
// vintage constants.IAU currently points at — see constants/iau2015.go.
var kmPerAU = constants.IAU.AstronomicalUnit.Value / 1e3 // 149597870.7 km

// secPerDay is the number of seconds in a Julian day.
var secPerDay = constants.Derived.JulianDaySeconds.Value // 86400

// ErrPropagation indicates an SGP4 propagation failure.
var ErrPropagation = errors.New("satellite: sgp4 propagation failed")

// ErrUnexpectedID indicates State was queried for a body other than the
// satellite this provider tracks. A *Satellite is single-purpose (one
// instance = one object), so id must be the zero value (core.ID(0)); any
// other id — including a real NAIF ID like a Sun/Moon lookup — used to be
// silently ignored and answered with this satellite's own state instead of
// an error, which is exactly what made plan.Satellite.ApparentMagnitudeCtx's
// Sun-position bug possible (see CHANGELOG).
var ErrUnexpectedID = errors.New("satellite: state queried for an id other than the tracked satellite (use core.ID(0))")

// ErrMalformedTLE indicates a two-line element set that is not well formed:
// the wrong length, the wrong line numbers, a failed checksum, or two lines
// describing different satellites.
//
// SGP4 will initialise happily from a corrupted element set and propagate it
// for ever, because every field it reads is still a number. A single digit
// altered in transmission or truncated in a copy moves an inclination or a
// mean motion to another plausible value and puts the satellite somewhere it
// has never been. The last character of each line exists to catch exactly
// that, and is worth checking before trusting the rest.
var ErrMalformedTLE = errors.New("satellite: malformed TLE")

// tleLineLength is the fixed width of a TLE line, checksum included.
const tleLineLength = 69

// Satellite wraps a NORAD TLE element set with SGP4 propagation state.
type Satellite struct {
	Name       string
	sat        gosatellite.Satellite
	MeanMotion float64
}

// NewFromTLE creates a Satellite from raw TLE lines.
//
// The element set is checked for well-formedness before SGP4 sees it: see
// [ErrMalformedTLE] for why, since SGP4 itself will accept a corrupted set
// and propagate it without complaint.
func NewFromTLE(name, line1, line2 string) (*Satellite, error) {
	if err := ValidateTLE(line1, line2); err != nil {
		return nil, err
	}

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

// State returns the geocentric position/velocity in GCRS (AU, AU/day),
// implementing the [core.Provider] interface contract.
//
// id must be core.ID(0) — this provider tracks exactly one body — any other
// id returns [ErrUnexpectedID] rather than silently answering for the wrong
// body.
//
// The conversion pipeline: TEME (km) → GCRS (AU) via the IAU 2006
// Earth rotation matrix (C2T06A).
func (s *Satellite) State(id core.ID, t time.Time) (core.State, error) {
	if id != 0 {
		return core.State{}, fmt.Errorf("%w: %d", ErrUnexpectedID, id)
	}

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

// ValidateTLE reports whether the two lines form a well-formed element set.
//
// Four things are checked, in the order a corrupted set is most likely to fail
// them:
//
//   - each line is the standard 69 characters;
//   - the first begins with "1" and the second with "2", which catches them
//     being supplied in the wrong order;
//   - the two carry the same satellite number, which catches line 1 of one
//     object pasted against line 2 of another - a substitution no checksum can
//     see, since each line is individually intact;
//   - each line's modulo-10 checksum matches its own last character.
//
// None of this validates the orbit. It establishes that the element set
// arrived as it was sent, which is the part SGP4 cannot tell for itself.
func ValidateTLE(line1, line2 string) error {
	for i, line := range [2]string{line1, line2} {
		want := byte('1' + i)

		if len(line) != tleLineLength {
			return fmt.Errorf("%w: line %d is %d characters, want %d",
				ErrMalformedTLE, i+1, len(line), tleLineLength)
		}

		if line[0] != want {
			return fmt.Errorf("%w: line %d begins with %q, want %q - are the two lines swapped?",
				ErrMalformedTLE, i+1, line[0], want)
		}

		// Compared as a digit rather than by converting the sum to a byte, so
		// that a checksum character which is not a digit at all is refused
		// here rather than wrapping into some other digit's value.
		got := line[tleLineLength-1]
		if sum := tleChecksum(line[:tleLineLength-1]); got < '0' || got > '9' || int(got-'0') != sum {
			return fmt.Errorf("%w: line %d checksum is %q, computed %d - a character has been altered or lost",
				ErrMalformedTLE, i+1, got, sum)
		}
	}

	// Columns 3-7 carry the satellite catalogue number on both lines.
	if a, b := line1[2:7], line2[2:7]; a != b {
		return fmt.Errorf("%w: line 1 is satellite %q and line 2 is satellite %q",
			ErrMalformedTLE, strings.TrimSpace(a), strings.TrimSpace(b))
	}

	return nil
}

// tleChecksum is the modulo-10 sum a TLE line's last character records: every
// digit counts for its value and every minus sign for one, with everything
// else - letters, spaces, decimal points and plus signs - counting for
// nothing.
func tleChecksum(line string) int {
	sum := 0

	for _, c := range line {
		switch {
		case c >= '0' && c <= '9':
			sum += int(c - '0')
		case c == '-':
			sum++
		}
	}

	return sum % 10
}

// propagateECI returns the TEME position and velocity (km, km/s) at time t.
// The go-satellite Propagate API accepts integer seconds, so we propagate
// to the truncated second and linearly interpolate position for the
// sub-second remainder using the velocity vector. This reduces the
// position error from up to ~7.7 km (at LEO velocity) to < 1 m.
func (s *Satellite) propagateECI(t time.Time) (pos, vel vector.Vec3, err error) {
	year, month, day, hour, minute, second, fracSec := timeToComponents(t)
	eciPos, eciVel := gosatellite.Propagate(s.sat, year, month, day, hour, minute, second)

	// Check for propagation failure (NaN or zero position).
	if math.IsNaN(eciPos.X) || math.IsNaN(eciPos.Y) || math.IsNaN(eciPos.Z) {
		return vector.Vec3{}, vector.Vec3{}, fmt.Errorf(
			"%w: NaN position at %s", ErrPropagation, t)
	}

	vel = vector.V3(eciVel.X, eciVel.Y, eciVel.Z)

	// Linear interpolation for sub-second fraction:
	// pos_corrected = pos_truncated + vel * fracSec
	pos = vector.V3(
		eciPos.X+eciVel.X*fracSec,
		eciPos.Y+eciVel.Y*fracSec,
		eciPos.Z+eciVel.Z*fracSec,
	)

	return pos, vel, nil
}

// subSatellitePoint returns the geodetic coordinates (lat, lon, altitude)
// of the sub-satellite point at time t.
func (s *Satellite) subSatellitePoint(t time.Time) (*coord.Geodetic, error) {
	eciPos, _, err := s.propagateECI(t)
	if err != nil {
		return nil, err
	}

	// Compute GAST for ECI → ECEF conversion. Falls back to UTC-derived
	// GAST (a few hundred ms of error at worst) rather than failing if
	// IERS EOP data is unavailable, matching this function's existing
	// no-error-return contract.
	gast, _ := t.GAST()

	// Rotate ECI → ECEF.
	cosG := math.Cos(gast.Radians())
	sinG := math.Sin(gast.Radians())
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

// temeToGCRS converts TEME position/velocity (km, km/s) to GCRS using
// the IAU 2006/2000A precession-nutation-bias matrix (BPN) and the
// equation of the equinoxes correction.
//
// Both TEME and GCRS are inertial (non-rotating) Earth-centered frames.
// They differ in axis orientation:
//   - TEME: true equator + mean equinox of date (SGP4 output frame)
//   - GCRS: ICRS axes (≈ J2000 equator and equinox)
//
// The conversion applies R3(-EqEq) to rotate from TEME's mean equinox
// to the true equinox of date, then BPN^T to rotate from the true
// equatorial frame to GCRS. The EqEq correction closes the ~20″
// residual that the old BPN-only path left on the table.
//
// Error budget after EqEq correction:
//   - IAU-76/FK5 (SGP4) vs. IAU 2006 frame tie: ~20 mas → negligible
//   - Well within SGP4's intrinsic ~1 km accuracy
func temeToGCRS(pos, vel vector.Vec3, t time.Time) (gcrsPos, gcrsVel vector.Vec3) {
	tt := t.TT()
	tta, ttb := tt.JDParts()

	// Equation of the equinoxes: rotates TEME (mean equinox) → true equinox.
	ee := gofaext.Ee06a(tta, ttb)
	cosEE := math.Cos(ee)
	sinEE := math.Sin(ee)

	// Apply R3(-EqEq) to get true equatorial of date.
	truePos := vector.V3(
		cosEE*pos.X-sinEE*pos.Y,
		sinEE*pos.X+cosEE*pos.Y,
		pos.Z,
	)
	trueVel := vector.V3(
		cosEE*vel.X-sinEE*vel.Y,
		sinEE*vel.X+cosEE*vel.Y,
		vel.Z,
	)

	// BPN: bias-precession-nutation matrix (GCRS → true equatorial of date).
	// Transpose maps back: true equatorial of date → GCRS.
	bpn := gofaext.Pnm06a(tta, ttb)

	// r_GCRS = BPN^T · r_true
	gcrsPos = vector.V3(
		bpn[0][0]*truePos.X+bpn[1][0]*truePos.Y+bpn[2][0]*truePos.Z,
		bpn[0][1]*truePos.X+bpn[1][1]*truePos.Y+bpn[2][1]*truePos.Z,
		bpn[0][2]*truePos.X+bpn[1][2]*truePos.Y+bpn[2][2]*truePos.Z,
	)

	// v_GCRS = BPN^T · v_true
	gcrsVel = vector.V3(
		bpn[0][0]*trueVel.X+bpn[1][0]*trueVel.Y+bpn[2][0]*trueVel.Z,
		bpn[0][1]*trueVel.X+bpn[1][1]*trueVel.Y+bpn[2][1]*trueVel.Z,
		bpn[0][2]*trueVel.X+bpn[1][2]*trueVel.Y+bpn[2][2]*trueVel.Z,
	)

	return gcrsPos, gcrsVel
}

// timeToComponents extracts calendar components from an astrogo time for SGP4.
// Returns integer year/month/day/hour/min/sec and the fractional second
// remainder for sub-second velocity interpolation.
func timeToComponents(t time.Time) (year, month, day, hour, minute, second int, fracSec float64) {
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
	minute = int(totalSec / 60)
	totalSec -= float64(minute) * 60
	second = int(totalSec)
	fracSec = totalSec - float64(second)

	return year, month, day, hour, minute, second, fracSec
}
