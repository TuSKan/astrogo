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
// the wrong length, the wrong line numbers, a failed checksum, two lines
// describing different satellites, or a field that is not the number the
// format says it is.
//
// SGP4 will initialise happily from a corrupted element set and propagate it
// for ever, as long as every field it reads is still a number. A single digit
// altered in transmission or truncated in a copy moves an inclination or a
// mean motion to another plausible value and puts the satellite somewhere it
// has never been. The last character of each line exists to catch exactly
// that, and is worth checking before trusting the rest.
//
// When a field is NOT still a number, the underlying SGP4 implementation does
// something worse than propagate wrongly: see [ValidateTLE].
var ErrMalformedTLE = errors.New("satellite: malformed TLE")

// tleLineLength is the fixed width of a TLE line, checksum included.
const tleLineLength = 69

// Satellite wraps a NORAD TLE element set with SGP4 propagation state.
type Satellite struct {
	Name       string
	sat        gosatellite.Satellite
	MeanMotion float64

	// epoch is the element set's own epoch, and epochFracSec the fractional
	// second within it. The backend truncates the epoch to a whole second
	// when it builds jdsatepoch, so propagateECI has to measure its
	// sub-second correction from this rather than from zero -- see there.
	epoch        time.Time
	epochFracSec float64
}

// NewFromTLE creates a Satellite from raw TLE lines.
//
// The element set is checked for well-formedness before SGP4 sees it: see
// [ErrMalformedTLE] for why, since SGP4 itself will accept a corrupted set
// and propagate it without complaint.
func NewFromTLE(name, line1, line2 string) (*Satellite, error) {
	if err := validateTLEStructure(line1, line2); err != nil {
		return nil, err
	}

	// Both the guard against TLEToSat's os.Exit and the source of MeanMotion:
	// one parse, so the value this reports can never disagree with the one
	// SGP4 propagates from.
	mm, epoch, err := parseTLENumerics(line1, line2)
	if err != nil {
		return nil, err
	}

	// The same conversion propagateECI applies to a query time, so the two
	// fractional seconds are on identical footing and cancel exactly when the
	// query lands a whole number of seconds after the epoch.
	_, _, _, _, _, _, epochFracSec := timeToComponents(epoch)

	sat := gosatellite.TLEToSat(line1, line2, gosatellite.GravityWGS84)
	if sat.Error != 0 {
		return nil, fmt.Errorf("%w: sgp4 init error %d: %s", ErrPropagation, sat.Error, sat.ErrorStr)
	}

	return &Satellite{
		Name:         name,
		MeanMotion:   mm,
		sat:          sat,
		epoch:        epoch,
		epochFracSec: epochFracSec,
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
		Pos:    gcrsPos.MulScalar(1.0 / kmPerAU),
		Vel:    gcrsVel.MulScalar(secPerDay / kmPerAU),
		Frame:  core.FrameGCRS,
		Center: core.CenterGeocentre,
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

// ValidateTLE reports whether the two lines form a well-formed element set.
//
// Five things are checked, in the order a corrupted set is most likely to fail
// them:
//
//   - each line is the standard 69 characters;
//   - the first begins with "1" and the second with "2", which catches them
//     being supplied in the wrong order;
//   - the two carry the same satellite number, which catches line 1 of one
//     object pasted against line 2 of another - a substitution no checksum can
//     see, since each line is individually intact;
//   - each line's modulo-10 checksum matches its own last character;
//   - every field the SGP4 backend will read as a number parses as one.
//
// None of this validates the orbit. It establishes that the element set
// arrived as it was sent, which is the part SGP4 cannot tell for itself.
//
// # Why the numeric check is not merely tidiness
//
// The backend (joshuaferrara/go-satellite) parses the twelve numeric fields
// through helpers that call log.Fatal on a parse error - os.Exit(1), from
// inside a library, taking the caller's whole process with it. No error is
// returned, no panic is raised, and there is nothing to recover: a program
// that fed one bad element set into a batch of ten thousand simply stops.
// Confirmed by running it, not inferred from reading it.
//
// A checksum does not close this. It is a modulo-10 sum in which letters,
// spaces and punctuation all count for nothing, so a field can be replaced
// with text - a truncated feed, a hand-built set, an "N/A" placeholder, a
// generator that pads with spaces - and still carry a correct checksum.
//
// So this parses each field exactly as the backend will, using the same
// column slices and the same two-space Replace (a field with three spaces
// where the backend removes two is one the backend would die on, and is
// therefore refused here rather than accepted as close enough), and reports
// [ErrMalformedTLE] naming the field. NewFromTLE runs this before SGP4 sees
// anything, which is the only place the guard is any use.
func ValidateTLE(line1, line2 string) error {
	if err := validateTLEStructure(line1, line2); err != nil {
		return err
	}

	_, _, err := parseTLENumerics(line1, line2)

	return err
}

// validateTLEStructure is ValidateTLE's length/ordering/checksum half, split
// out so NewFromTLE can run it before parseTLENumerics without paying for the
// numeric parse twice - it needs the mean motion that parse produces.
func validateTLEStructure(line1, line2 string) error {
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

// parseTLENumerics parses every field the SGP4 backend will parse, from the
// same columns and with the same string surgery, and returns the mean motion
// (rev/day, line 2 columns 53-63) as the one value this package keeps.
//
// The transformations are copied from the backend's ParseTLE rather than
// written afresh, deliberately: the contract is not "these look like numbers"
// but "these are the exact strings that function will hand to strconv", and
// only an identical construction can promise that. Notably ecco is prefixed
// with "." (a TLE stores eccentricity with an assumed leading decimal point)
// and nddot/bstar are reassembled from three slices into a mantissa-exponent
// form, so neither field parses as a number in its raw column form at all.
//
// The two-space Replace count is copied too, and matters: a field carrying
// three spaces would leave one behind, and " 15.72" does not parse. Using -1
// here would accept a set the backend then dies on, which is worse than not
// checking, because the guard would read as if it worked.
//
// Callers must have established the 69-character length first
// (validateTLEStructure) - every slice below indexes fixed columns.
func parseTLENumerics(line1, line2 string) (meanMotion float64, epoch time.Time, err error) {
	ints := [...]struct {
		name  string
		value string
	}{
		{"satellite number", strings.TrimSpace(line1[2:7])},
		{"epoch year", line1[18:20]},
	}

	for _, f := range ints {
		if _, err := strconv.ParseInt(f.value, 10, 0); err != nil {
			return 0, time.Time{}, fmt.Errorf("%w: %s is %q, which is not an integer", ErrMalformedTLE, f.name, f.value)
		}
	}

	floats := [...]struct {
		name  string
		value string
	}{
		{"epoch day", line1[20:32]},
		{"first derivative of mean motion", strings.Replace(line1[33:43], " ", "", 2)},
		{"second derivative of mean motion", strings.Replace(line1[44:45]+"."+line1[45:50]+"e"+line1[50:52], " ", "", 2)},
		{"B* drag term", strings.Replace(line1[53:54]+"."+line1[54:59]+"e"+line1[59:61], " ", "", 2)},
		{"inclination", strings.Replace(line2[8:16], " ", "", 2)},
		{"right ascension of the ascending node", strings.Replace(line2[17:25], " ", "", 2)},
		{"eccentricity", "." + line2[26:33]},
		{"argument of perigee", strings.Replace(line2[34:42], " ", "", 2)},
		{"mean anomaly", strings.Replace(line2[43:51], " ", "", 2)},
		{"mean motion", strings.Replace(line2[52:63], " ", "", 2)},
	}

	for _, f := range floats {
		v, err := strconv.ParseFloat(f.value, 64)
		if err != nil {
			return 0, time.Time{}, fmt.Errorf("%w: %s is %q, which is not a number", ErrMalformedTLE, f.name, f.value)
		}

		if f.name == "mean motion" {
			meanMotion = v
		}
	}

	return meanMotion, tleEpoch(line1), nil
}

// tleEpoch reconstructs the element set's epoch from line 1's two-digit year
// (columns 19-20) and fractional day of year (21-32).
//
// The two-digit year follows the NORAD convention the backend also uses: 57
// and above is 19xx, below is 20xx. It is not a Y2K bug to be fixed here --
// the format has no other year, so the window is the format's, and changing it
// would disagree with the propagator being driven.
//
// Both fields are already known to parse; this runs after the loop above.
func tleEpoch(line1 string) time.Time {
	yy, _ := strconv.Atoi(strings.TrimSpace(line1[18:20]))

	year := 2000 + yy
	if yy >= 57 {
		year = 1900 + yy
	}

	// Day of year is 1-based: day 1.0 is midnight on January 1st.
	doy, _ := strconv.ParseFloat(strings.TrimSpace(line1[20:32]), 64)

	jan1 := time.Date(year, 1, 1, 0, 0, 0, 0, time.LocationUTC)

	return jan1.Add(time.Duration((doy - 1) * float64(24*time.Hour)))
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
//
// # The sub-second correction, and why it is measured from the epoch
//
// The go-satellite Propagate API accepts integer seconds only, so it can be
// asked for a state at the truncated second and the sub-second remainder has
// to be recovered by a linear step along the velocity vector.
//
// The subtle part is what the remainder is measured from. Propagate computes
// its own time argument as
//
//	tsince = (JDay(truncated query) - sat.jdsatepoch) * 1440
//
// and `jdsatepoch` is itself built with `JDay(..., int(sec))` — the element
// set's epoch is truncated to a whole second exactly like the query is. Both
// ends of the subtraction lose their fraction, so what Propagate actually
// evaluates is offset from the intended instant by
//
//	frac(t) - frac(epoch)
//
// not by frac(t). Correcting by frac(t) alone — which this function did until
// the Vallado verification suite was run against it — leaves a residual of
// vel * frac(epoch): a fixed offset per element set, up to one second of
// motion, which is 7.5 km for a low Earth orbit and was measured at 5.94 km
// for Vallado's own satellite 5. It is worst at the epoch itself, where the
// answer should be exact, and it does not average out, because frac(epoch) is
// a property of the element set rather than of the query.
//
// See TestSGP4AgreesWithValladoReferenceVectors, which measures this end to
// end and would fail again at kilometre scale if the origin were dropped.
func (s *Satellite) propagateECI(t time.Time) (pos, vel vector.Vec3, err error) {
	year, month, day, hour, minute, second, fracSec := timeToComponents(t)
	eciPos, eciVel := gosatellite.Propagate(s.sat, year, month, day, hour, minute, second)

	// Check for propagation failure (NaN or zero position).
	if math.IsNaN(eciPos.X) || math.IsNaN(eciPos.Y) || math.IsNaN(eciPos.Z) {
		return vector.Vec3{}, vector.Vec3{}, fmt.Errorf(
			"%w: NaN position at %s", ErrPropagation, t)
	}

	vel = vector.V3(eciVel.X, eciVel.Y, eciVel.Z)

	// pos_corrected = pos_truncated + vel * (frac(t) - frac(epoch))
	dt := fracSec - s.epochFracSec
	pos = vector.V3(
		eciPos.X+eciVel.X*dt,
		eciPos.Y+eciVel.Y*dt,
		eciPos.Z+eciVel.Z*dt,
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
	// GAST rather than failing if IERS EOP data is unavailable, matching
	// this function's existing no-error-return contract.
	//
	// That fallback costs up to 0.9 s of Earth rotation — the bound the
	// leap-second system enforces on UT1-UTC — which is about 13.5 arcsec
	// of longitude, or 420 m of sub-satellite position at the equator. The
	// comment here previously said "a few hundred ms", which understated it
	// threefold. It is a ground-track error, not an orbit error: the ECI
	// state is unaffected.
	//
	// CGPM Resolution 4 (2022) ends leap seconds by 2035 and with them that
	// bound, so this degradation is unbounded thereafter. Discarding the
	// error is deliberate here and stays deliberate, but it is worth knowing
	// what is being discarded.
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
	// SGP4 is defined against UTC, so normalise before reading the calendar
	// fields. Without this the caller's scale is silently reinterpreted: a TT
	// instant lands 69.184 s late, which for the ISS at 7.66 km/s is 530 km,
	// and TAI lands 37 s late for 283 km. The type is scale-aware precisely so
	// this cannot be left to the caller.
	//
	// temeToGCRS in this same file already does the equivalent (t.TT()), and
	// coord.NewContext opens with t = t.UTC(); this brings SGP4 in line with
	// both.
	t = t.UTC()

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
