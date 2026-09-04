package lsk

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/time"
)

// Sentinel errors for LSK parsing.
var (
	ErrNoLeapseconds = errors.New("jpl: no leapseconds found in LSK")
	ErrInvalidDate   = errors.New("invalid spice date")
	ErrInvalidMonth  = errors.New("invalid month")
)

// Reader is a reader for the JPL LSK kernel.
//
// Despite the name, an LSK is not a leap-second table — it is SPICE's
// time-conversion kernel. Alongside the DELTA_AT steps it carries the
// constants of Moyer's (1981) relativistic model, which is what turns UTC into
// the ET that every SPK segment is indexed by. Both halves are parsed here,
// because using one without the other would evaluate a JPL kernel at an epoch
// JPL did not mean.
type Reader struct {
	F       io.ReadCloser
	DeltaAt []LeapData

	// DeltaTA, K, EB, M0 and M1 are DELTET/DELTA_T_A, DELTET/K, DELTET/EB and
	// the two components of DELTET/M. Together they give
	//
	//	ET − TAI = DELTA_T_A + K·sin(E),  E = M + EB·sin(M),  M = M0 + M1·t
	//
	// with t in ephemeris seconds past J2000. NAIF's own header puts the
	// model at "accurate to about 0.000030 seconds".
	DeltaTA, K, EB, M0, M1 float64
}

// Cache opens the LSK file named kernel, downloading it first when it is
// absent, into remote's registered cache directory for remote.NAIFLSK.
// Downloads are gated by remote's consent configuration — enable them with
// remote.EnableDownloads(0, remote.NAIFLSK) (naif0012.tls is only ~5 KB) or
// pre-seed the file to stay fully offline.
//
// It provides an auto-healing mechanism for CI environments by automatically
// removing corrupt or truncated files.
func Cache(ctx context.Context, kernel string) (*Reader, error) {
	bucket, key, err := remote.GetFile(ctx, remote.NAIFLSK, kernel, remote.WithCacheName(kernel))
	if err != nil {
		return nil, fmt.Errorf("jpl: LSK %s: %w", kernel, err)
	}

	ls, err := bucket.NewReader(ctx, key, nil)
	if err != nil {
		return nil, fmt.Errorf("jpl: failed to load LSK %s: %w", key, err)
	}

	return NewReader(ls)
}

// NewReader parses an LSK kernel from the given reader.
func NewReader(r io.ReadCloser) (*Reader, error) {
	l := &Reader{}
	scanner := bufio.NewScanner(r)
	inDeltaAt := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// The relativistic constants are scalars on their own lines, so they
		// are matched before the DELTA_AT block logic. DELTA_T_A is tested
		// first because it also contains the substring "DELTA_" that the
		// block test below looks for.
		if l.parseDeltaETConstant(line) {
			continue
		}

		if strings.Contains(line, "DELTET/DELTA_AT") {
			inDeltaAt = true

			if strings.Contains(line, "=") {
				line = line[strings.Index(line, "=")+1:]
			}
		}

		// Whether this line belongs to the DELTET/DELTA_AT block, decided
		// before the closing parenthesis is consumed below.
		//
		// It has to be captured first because the block's final entry and
		// its terminator share a line:
		//
		//	37,   @2017-JAN-1 )
		//
		// Clearing inDeltaAt and only then testing it dropped that entry
		// silently — the stripped remainder starts with "37,", not "@", so
		// neither arm of the guard matched. The table therefore ended at
		// 36 leap seconds (2015-07-01), and every UTC epoch from
		// 2017-01-01 onward converted one second early: measured as a
		// 1.0000 s offset against time.TDB(), which put the geocentric Sun
		// about 30 km — one second of Earth's orbital motion — from where
		// DE440 has it.
		inBlock := inDeltaAt

		if inDeltaAt {
			if idx := strings.Index(line, ")"); idx >= 0 {
				inDeltaAt = false
				line = line[:idx]
			}
		}

		if inBlock || (line != "" && strings.HasPrefix(line, "@")) {
			line = strings.ReplaceAll(line, "(", " ")
			line = strings.ReplaceAll(line, ",", " ")

			parts := strings.Fields(line)
			for i := 0; i+1 < len(parts); i += 2 {
				// n is first, then date
				n, err1 := strconv.ParseFloat(parts[i], 64)

				jd, err2 := parseSpiceDate(parts[i+1])
				if err1 == nil && err2 == nil {
					l.DeltaAt = append(l.DeltaAt, LeapData{JD: jd, N: n})
				}
			}
		}
	}

	// A read that failed part-way leaves a *partial* DELTA_AT table, which is
	// the dangerous shape rather than the obvious one: the emptiness check
	// below catches a total failure, and nothing catches a table that simply
	// stops early. That is the same defect a dropped final entry produced —
	// every epoch after the truncation converting a second short, with the
	// geocentric Sun about 30 km from where DE440 has it — except arriving
	// from a short read instead of a parsing slip.
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("lsk: read kernel: %w", err)
	}

	if len(l.DeltaAt) == 0 {
		return nil, ErrNoLeapseconds
	}

	l.F = r

	return l, nil
}

// Close closes the underlying file reader.
func (r *Reader) Close() error {
	err := r.F.Close()
	if err != nil {
		return fmt.Errorf("lsk: close: %w", err)
	}

	return nil
}

// covers reports whether jd falls within the era the DELTA_AT table speaks
// for — that is, at or after its first entry.
//
// The table starts at 1972-01-01 because that is when UTC began accumulating
// whole leap seconds. An earlier epoch is not a small extrapolation: leap
// seconds do not apply at all, and the offset between atomic and rotational
// time is the historical Delta-T, which no leap-second table carries.
func (r *Reader) covers(jd float64) bool {
	return len(r.DeltaAt) > 0 && jd >= r.DeltaAt[0].JD
}

// hasDeltaET reports whether the relativistic constants were all present.
//
// A kernel missing them is not usable for time conversion, and silently
// falling back to a different model would reintroduce exactly the mismatch
// this file exists to avoid.
func (r *Reader) hasDeltaET() bool {
	return r.DeltaTA != 0 && r.K != 0 && r.EB != 0 && r.M1 != 0
}

// parseDeltaETConstant reads one DELTET scalar assignment, reporting whether
// the line was one.
//
// SPICE writes exponents in Fortran style — "1.657D-3" — which Go's parser
// does not accept, so D is normalised to E before parsing. Getting this wrong
// would leave the constant at zero and silently disable the periodic term,
// which is why hasDeltaET checks for exactly that.
func (r *Reader) parseDeltaETConstant(line string) bool {
	const prefix = "DELTET/"

	if !strings.HasPrefix(line, prefix) {
		return false
	}

	lhs, rhs, ok := strings.Cut(line, "=")
	if !ok {
		return false
	}

	name := strings.TrimSpace(strings.TrimPrefix(lhs, prefix))
	value := strings.NewReplacer("(", " ", ")", " ", "D", "E", "d", "e").Replace(rhs)

	fields := strings.Fields(value)
	if len(fields) == 0 {
		return false
	}

	nums := make([]float64, 0, len(fields))

	for _, f := range fields {
		v, err := strconv.ParseFloat(f, 64)
		if err != nil {
			return false
		}

		nums = append(nums, v)
	}

	switch name {
	case "DELTA_T_A":
		r.DeltaTA = nums[0]
	case "K":
		r.K = nums[0]
	case "EB":
		r.EB = nums[0]
	case "M":
		if len(nums) < 2 {
			return false
		}

		r.M0, r.M1 = nums[0], nums[1]
	default:
		// DELTA_AT, or a constant this reader does not model. Left to the
		// block parser.
		return false
	}

	return true
}

// LeapData represents a leapsecond correction entry.
type LeapData struct {
	JD, N float64
}

// parseSpiceDate parses a SPICE date string into a Julian Date.
//
// The format is "@YYYY-MMM-DD" or "@YYYY-MMM".
// Example: "@2016-JAN-01"
func parseSpiceDate(s string) (float64, error) {
	s = strings.TrimPrefix(s, "@")

	parts := strings.Split(s, "-")
	if len(parts) < 2 {
		return 0, fmt.Errorf("%w: %s", ErrInvalidDate, s)
	}

	year, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("%w: %s", ErrInvalidDate, s)
	}

	monthStr := strings.ToUpper(parts[1])

	day := 1
	if len(parts) > 2 {
		day, err = strconv.Atoi(parts[2])
		if err != nil {
			return 0, fmt.Errorf("%w: %s", ErrInvalidDate, s)
		}
	}

	months := map[string]int{
		"JAN": 1, "FEB": 2, "MAR": 3, "APR": 4, "MAY": 5, "JUN": 6,
		"JUL": 7, "AUG": 8, "SEP": 9, "OCT": 10, "NOV": 11, "DEC": 12,
	}

	month := months[monthStr]
	if month == 0 {
		return 0, fmt.Errorf("%w: %s", ErrInvalidMonth, monthStr)
	}

	// Simple JD calculation for 12:00:00 (standard for leapsecond dates in LSK)
	// JD = 367*Y - (7*(Y + (M+9)/12))/4 + (275*M)/9 + D + 1721013.5
	// This is valid for Gregorian calendar (post-1582).
	a := (14 - month) / 12
	y := year + 4800 - a
	m := month + 12*a - 3
	jd := float64(day) + math.Floor(float64(153*m+2)/5) + float64(365*y) + math.Floor(float64(y)/4) - math.Floor(float64(y)/100) + math.Floor(float64(y)/400) - 32045.5

	return jd, nil
}

// leapSecondsAt returns DELTA_AT — the accumulated leap seconds — at a Julian
// Date, using the kernel's own table.
//
// The interval is half-open: an entry takes effect at its own instant and
// holds until the next. That matches IERS, which reports 36 at
// 2016-12-31 23:59:60 and 37 at 2017-01-01 00:00:00; time's
// TestLeapSecondBoundaryConvention pins the same rule for the built-in table.
func (r *Reader) leapSecondsAt(jdTDB float64) float64 {
	lastN := 0.0

	for _, d := range r.DeltaAt {
		if jdTDB < d.JD {
			break
		}

		lastN = d.N
	}

	return lastN
}

// UTCToTDB converts a time.Time to a Julian Date in the Barycentric Dynamical
// Time (TDB) scale.
//
// The conversion is delegated to [time.Time.TDB], which is the scale-aware
// path the rest of the library uses.
//
// # Two defects this replaced
//
// The previous implementation special-cased only Scale() == TDB and treated
// every other scale as UTC, so a TT instant had leap seconds plus 32.184 s
// added on top of an offset it already carried. 69.184 s late — about 40
// arcsec of lunar motion and 1.8 arcsec for Mars. The type is scale-aware
// precisely so this cannot be left to the caller, and the SOFA provider always
// honoured it.
//
// The formula it used, TDB = UTC + (LS + 32.184)/86400, is the formula for
// **TT**, not TDB. It omitted the TDB−TT periodic term entirely — an amplitude
// of about 1.7 ms, which is 1.7 m of lunar motion and 85 m for Mars. Small
// against DE440 itself, and not small against the 33 mm this library claims
// against Horizons. SPICE's own str2et includes those terms.
//
// The second defect was invisible until the first was fixed, because a
// 69-second error hides a 1.7-millisecond one.
//
// # Why the kernel drives this and not time.Time.TDB
//
// Because an SPK segment is indexed by the ET that *its own kernel set*
// defines, and the LSK is where that definition lives. Evaluating a JPL kernel
// at an epoch computed by a different model means asking JPL's ephemeris a
// question JPL did not pose.
//
// astrogo's time.tdbMinusTT implements Fairhead & Bretagnon and carries more
// terms than Moyer's single sinusoid, so it is nominally the more accurate
// model — and using it here would still be wrong. The two differ by up to
// about 30 µs, which is roughly 3 cm of lunar motion: the same order as the
// 33 mm this library claims against Horizons, and Horizons computes ET from
// the LSK. Matching the reference means adopting its convention, not a better
// one.
//
// This is also why the LSK is a hard requirement of jpl.NewProvider rather
// than a nicety. SPICE itself refuses to convert a time with no LSK furnished;
// astrogo failing the same way is the contract, not a wart.
//
// # The conversion
//
// From the kernel's own header, with constants read out of the file:
//
//	ET − TAI = DELTA_T_A + K·sin(E)
//	E        = M + EB·sin(M)
//	M        = M0 + M1·t        (t = ephemeris seconds past J2000)
//	TAI      = UTC + DELTA_AT
//
// t is nominally ET, which appears on both sides. Seeding it with TAI instead
// costs about 10 ns — M1 is 2e-7 rad/s, so the 32.184 s difference moves
// K·sin(E) by ~1e-8 s — so no iteration is worth the cycles.
//
// # The input is normalised first
//
// Every scale is converted to UTC before DELTA_AT is applied. Without that the
// caller's label was silently reinterpreted: a TT instant had leap seconds
// plus 32.184 s added on top of an offset it already carried, landing 69.184 s
// late — about 40 arcsec of lunar motion. See
// ephemeris.TestProviderStateIsScaleInvariant, which pins it.
func UTCToTDB(t time.Time, l *Reader) float64 {
	utc := t.UTC()
	d1, d2 := utc.JDParts()
	jdUTC := d1 + d2

	if l == nil || !l.hasDeltaET() || !l.covers(jdUTC) {
		// Either no usable kernel, or an epoch the kernel cannot speak for.
		//
		// The DELTA_AT table begins at 1972-01-01, because that is when UTC
		// began accumulating whole leap seconds. Before it there are no leap
		// seconds to add, and TT-UT1 is the historical Delta-T instead —
		// roughly 10,500 s at year 1, which is not a rounding difference.
		//
		// [time.DeltaT] owns that model: the Espenak & Meeus (2006) polynomial
		// expressions, with the Morrison & Stephenson (2004) base and a secular
		// acceleration correction. Delegating reaches it rather than
		// duplicating it — time.Time's own scale conversion switches to DeltaT
		// below 1972, the same boundary [Reader.covers] uses, so the two agree
		// by construction rather than by coincidence.
		//
		// Measured against time.TDB before this guard existed: -174.9 min at
		// year 1, -25.5 min at year 1000, +0.6 min at 1900, and exact from
		// 1972 on. It surfaced as ~180 min errors in the AstroPixels lunar
		// phase comparison for year 0001.
		td1, td2 := t.TDB().JDParts()

		return td1 + td2
	}

	tai := jdUTC + l.leapSecondsAt(jdUTC+(69.184/86400.0))/86400.0

	// Ephemeris seconds past J2000, seeded with TAI as described above.
	secPastJ2000 := (tai - 2451545.0) * 86400.0

	m := l.M0 + l.M1*secPastJ2000
	e := m + l.EB*math.Sin(m)

	return tai + (l.DeltaTA+l.K*math.Sin(e))/86400.0
}

// j2000JD is the Julian Date of the J2000.0 epoch, and secondsPerDay the
// length of a Julian day.
const (
	j2000JD       = 2451545.0
	secondsPerDay = 86400.0
)

// TDBToET converts a Julian Date in TDB to elapsed seconds past J2000.
//
// Deprecated: use [UTCToET], which keeps the two-part Julian Date the caller's
// time.Time already holds. A single float64 Julian Date has an ULP of about
// 40 microseconds at a modern epoch, so a value that has been summed before
// reaching here has already lost more precision than this conversion can
// recover. See [UTCToET].
func TDBToET(jdTDB float64) float64 {
	return (jdTDB - j2000JD) * secondsPerDay
}

// UTCToET converts a time directly to ephemeris seconds past J2000 — the
// argument every SPK segment is indexed by.
//
// # Why this exists rather than TDBToET(UTCToTDB(...))
//
// That pair sums the two-part Julian Date before subtracting the epoch, and
// the order matters. A Julian Date at a modern epoch is about 2.46e6, where one
// ULP of a float64 is
//
//	math.Ulp(2460545.0) = 4.657e-10 days = 40.2 microseconds
//
// so the ET handed to the evaluator was quantised to 40 µs. That is 4 cm of
// lunar motion and 31 cm for the ISS — and the Moon figure sits at the level of
// the 33 mm this library claims against Horizons.
//
// It also made comparison meaningless: measuring the kernel's Moyer model
// against time's Fairhead & Bretagnon series gave readings of exactly 0.0 or
// exactly 40.2 µs at every epoch, because the two differ by about 30 µs and
// the API could not represent the difference.
//
// Subtracting the epoch first keeps the value small. time.Time stores the day
// number and the fraction separately for exactly this reason, and JDParts
// hands both over:
//
//	(jd1 - 2451545.0) + jd2     ~1e4      ULP 1.8e-12 days = 0.16 ns
//	jd1 + jd2                   ~2.46e6   ULP 4.7e-10 days = 40   µs
//
// The first subtraction is exact — both operands are whole days — so the
// fraction keeps every bit it arrived with. SPICE works in ET seconds past
// J2000 for the same reason.
//
// The conversion itself is unchanged; see [UTCToTDB] for the model and for
// why the kernel drives it.
func UTCToET(t time.Time, l *Reader) float64 {
	utc := t.UTC()
	d1, d2 := utc.JDParts()
	jdUTC := d1 + d2

	if l == nil || !l.hasDeltaET() || !l.covers(jdUTC) {
		// Outside the kernel's era, or no usable kernel. Same delegation as
		// UTCToTDB, in the same two-part form.
		td1, td2 := t.TDB().JDParts()

		return ((td1 - j2000JD) + td2) * secondsPerDay
	}

	// Seconds of UTC past J2000, with the epoch removed from the day number
	// before the fraction is added back.
	secUTC := ((d1 - j2000JD) + d2) * secondsPerDay

	// The leap-second lookup only needs to land in the right interval, so the
	// summed Julian Date is precise enough for it.
	secTAI := secUTC + l.leapSecondsAt(jdUTC+(69.184/secondsPerDay))

	m := l.M0 + l.M1*secTAI
	e := m + l.EB*math.Sin(m)

	return secTAI + l.DeltaTA + l.K*math.Sin(e)
}
