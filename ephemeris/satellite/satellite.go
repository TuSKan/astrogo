package satellite

import (
	"errors"
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/satellite/sgp4"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
	"github.com/TuSKan/astrogo/vector"
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
// Both halves of that check now live in
// [github.com/TuSKan/astrogo/ephemeris/satellite/sgp4], which separates them:
// [sgp4.VerifyTLEChecksums] asks whether the text arrived intact and
// [sgp4.ParseTLE] asks whether the elements make sense. This error wraps
// either, so an existing caller matching on it is unaffected.
var ErrMalformedTLE = errors.New("satellite: malformed TLE")

// Satellite wraps a NORAD TLE element set with SGP4 propagation state.
type Satellite struct {
	Name string

	// MeanMotion is the element set's own, in revolutions per day. Public
	// since before this type held a propagator, and kept.
	MeanMotion float64

	// prop holds the initialised model. It is immutable, so a *Satellite is
	// as safe to share across goroutines as the propagator inside it.
	prop *sgp4.Propagator

	// epoch is the element set's own epoch. Read straight from prop's
	// elements, so the two cannot disagree.
	epoch time.Time
}

// NewFromTLE creates a Satellite from raw TLE lines.
//
// The element set is checked for well-formedness before SGP4 sees it — see
// [ErrMalformedTLE] — because SGP4 itself will accept a corrupted set and
// propagate it without complaint.
//
// WGS-72 is used, which is [sgp4]'s default and is the gravity model TLEs are
// fitted with. Choosing otherwise is a model mismatch rather than a
// refinement; see [sgp4.Gravity].
func NewFromTLE(name, line1, line2 string) (*Satellite, error) {
	// Not ValidateTLE, although the two ask the same questions: that would
	// parse the element set and throw the result away, then parse it again
	// here. Parsing one input twice is how two places come to disagree about
	// what it says, which is a defect this package has already had once.
	if err := sgp4.VerifyTLEChecksums(line1, line2); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMalformedTLE, err)
	}

	el, err := sgp4.ParseTLEName(name, line1, line2)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMalformedTLE, err)
	}

	// sgp4.New re-runs Elements.Validate, which ParseTLEName has already
	// passed, so with the default options there is no input that reaches this
	// branch today. It is wrapped rather than ignored because the failure it
	// would report is a propagation one, not a parse one, and a caller
	// matching on ErrPropagation should not have to care that the distinction
	// is currently theoretical.
	prop, err := sgp4.New(el)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrPropagation, err)
	}

	return &Satellite{
		Name:       el.Name,
		MeanMotion: el.MeanMotion,
		prop:       prop,
		epoch:      el.Epoch,
	}, nil
}

// Propagator returns the underlying SGP4 model.
//
// Exposed because this type is an ephemeris provider — it answers in GCRS
// astronomical units, which is the wrong shape for a caller who wants raw TEME
// kilometres, the element set as parsed, or the model's own branch predicates.
// Those callers should not have to reach for a second parse of the same text.
func (s *Satellite) Propagator() *sgp4.Propagator { return s.prop }

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
		Center: core.CenterGeocenter,
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

// Altitude returns the precise altitude above the WGS84 ellipsoid at time t.
// This uses the sub-satellite geodetic computation for WGS84-precise values.
func (s *Satellite) Altitude(t time.Time) (unit.Length, error) {
	geo, err := s.subSatellitePoint(t)
	if err != nil {
		return 0, err
	}

	return geo.Height(), nil
}

// ValidateTLE reports whether the two lines form a well-formed element set.
//
// Four things are checked, in the order a corrupted set is most likely to fail
// them:
//
//   - each line carries at least the standard 69 columns and begins with its
//     own line number, which catches truncation and catches the two being
//     supplied in the wrong order. Trailing whitespace and anything appended
//     past column 69 are accepted rather than refused: feeds emit CRLF and
//     padding constantly, Vallado's own verification file appends three fields
//     to every line 2, and neither loses data. A line SHORTER than 69 does, and
//     is refused;
//   - each line's modulo-10 checksum matches its own last character;
//   - the two carry the same satellite number, which catches line 1 of one
//     object pasted against line 2 of another — a substitution no checksum can
//     see, since each line is individually intact;
//   - every field parses as the number the format says it is, and the
//     resulting elements are physically possible.
//
// None of this validates the orbit. It establishes that the element set
// arrived as it was sent, which is the part SGP4 cannot tell for itself.
//
// # What this used to be
//
// Eighty lines reproducing the old backend's own column slices and string
// surgery, field for field, because that backend parsed them through helpers
// that called log.Fatal on a parse error — os.Exit(1), from inside a library,
// taking the caller's whole process with it. A program that fed one bad
// element set into a batch of ten thousand simply stopped. Another forty lines
// predicted a panic in its day-of-year arithmetic, which a fuzzer found in
// about two seconds.
//
// None of that is needed now. [sgp4.ParseTLE] returns errors, names the field
// that failed, and cannot exit anybody's process, so this is the two calls it
// takes to ask both questions — and the reason it is still one function is
// that "did this arrive intact" is exactly the question a caller reading a
// network feed wants answered in one place.
func ValidateTLE(line1, line2 string) error {
	if err := sgp4.VerifyTLEChecksums(line1, line2); err != nil {
		return fmt.Errorf("%w: %w", ErrMalformedTLE, err)
	}

	if _, err := sgp4.ParseTLE(line1, line2); err != nil {
		return fmt.Errorf("%w: %w", ErrMalformedTLE, err)
	}

	return nil
}

// propagateECI returns the TEME position and velocity (km, km/s) at time t.
//
// # What used to be here
//
// Forty lines correcting for an API that accepted whole seconds only. The old
// backend's Propagate took integer year/month/day/hour/minute/second, and
// built its own jdsatepoch the same way, so the correction was not frac(t) but
// frac(t) − frac(epoch) — a subtlety that left a fixed per-element-set offset
// of up to one second of motion, 7.5 km for a low Earth orbit, and was worst
// at the epoch itself where the answer should be exact. It measured 5.94 km on
// Vallado's satellite 5 before the reference suite caught it.
//
// [sgp4.Propagator.AtTime] takes the instant. There is nothing to correct.
func (s *Satellite) propagateECI(t time.Time) (pos, vel vector.Vec3, err error) {
	pos, vel, err = s.prop.AtTime(t)
	if err != nil {
		return vector.Vec3{}, vector.Vec3{}, fmt.Errorf("%w: %w", ErrPropagation, err)
	}

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
