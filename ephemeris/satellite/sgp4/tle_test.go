package sgp4_test

import (
	"bufio"
	"errors"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/satellite/sgp4"
	"github.com/TuSKan/astrogo/time"
)

// The ISS element set Vallado and python-sgp4 both use as their worked example,
// and the one every field assertion below is checked against by hand.
const (
	issLine1 = "1 25544U 98067A   08264.51782528 -.00002182  00000-0 -11606-4 0  2927"
	issLine2 = "2 25544  51.6416 247.4627 0006703 130.5360 325.0288 15.72125391563537"
)

// TestParseTLEReadsEveryField is the golden test: one element set, every field,
// each value read off the format by hand rather than from another parser.
//
// Every one of these is a column span, and an off-by-one in a column span does
// not produce an error — it produces the neighbouring field, which is also a
// number. Asserting the whole record is the only way to see that.
func TestParseTLEReadsEveryField(t *testing.T) {
	t.Parallel()

	el, err := sgp4.ParseTLEName("ISS (ZARYA)", issLine1, issLine2)
	if err != nil {
		t.Fatalf("ParseTLEName: %v", err)
	}

	if el.Name != "ISS (ZARYA)" {
		t.Errorf("Name = %q, want %q", el.Name, "ISS (ZARYA)")
	}

	if el.NORAD != 25544 {
		t.Errorf("NORAD = %d, want 25544", el.NORAD)
	}

	if el.Classification != 'U' {
		t.Errorf("Classification = %q, want 'U'", el.Classification)
	}

	if el.Designator != "98067A" {
		t.Errorf("Designator = %q, want %q", el.Designator, "98067A")
	}

	if el.ElementSet != 292 {
		t.Errorf("ElementSet = %d, want 292", el.ElementSet)
	}

	if el.RevAtEpoch != 56353 {
		t.Errorf("RevAtEpoch = %d, want 56353", el.RevAtEpoch)
	}

	// Epoch: year 08 -> 2008 (a leap year), day 264.51782528. Day 1.0 is
	// midnight on 1 January, so day 264 is 20 September and the fraction is
	// 0.51782528 x 86400 = 44740.104192 s = 12:25:40.104.
	wantEpoch := time.Date(2008, time.September, 20, 12, 25, 40, 104192000, time.LocationUTC)
	if gap := math.Abs(el.Epoch.Sub(wantEpoch).Seconds()); gap > 1e-4 {
		t.Errorf("Epoch = %s, want %s (%.6f s apart)", el.Epoch, wantEpoch, gap)
	}

	for _, tc := range []struct {
		name string
		got  float64
		want float64
		tol  float64
	}{
		{"Inclination (deg)", el.Inclination.Degrees(), 51.6416, 1e-10},
		{"RAAN (deg)", el.RAAN.Degrees(), 247.4627, 1e-10},
		{"ArgPerigee (deg)", el.ArgPerigee.Degrees(), 130.5360, 1e-10},
		{"MeanAnomaly (deg)", el.MeanAnomaly.Degrees(), 325.0288, 1e-10},

		// Seven digits after an assumed decimal point.
		{"Eccentricity", el.Eccentricity, 0.0006703, 0},
		{"MeanMotion", el.MeanMotion, 15.72125391, 0},
		{"MeanMotionDot", el.MeanMotionDot, -0.00002182, 0},

		// " 00000-0" is a written zero, not an absent field.
		{"MeanMotionDDot", el.MeanMotionDDot, 0, 0},

		// "-11606-4" is -0.11606e-4. Getting the assumed decimal point wrong
		// here is a factor of 10^5 in the only drag term SGP4 uses.
		{"BStar", el.BStar, -1.1606e-5, 0},
	} {
		if diff := math.Abs(tc.got - tc.want); diff > tc.tol {
			t.Errorf("%s = %.12g, want %.12g", tc.name, tc.got, tc.want)
		}
	}
}

// TestParseTLEAcceptsWhatRealSourcesSend covers the shapes a feed actually
// produces, all of which must read identically.
func TestParseTLEAcceptsWhatRealSourcesSend(t *testing.T) {
	t.Parallel()

	want, err := sgp4.ParseTLE(issLine1, issLine2)
	if err != nil {
		t.Fatalf("baseline: %v", err)
	}

	for _, tc := range []struct {
		name         string
		line1, line2 string
	}{
		{"CRLF", issLine1 + "\r\n", issLine2 + "\r\n"},
		{"trailing spaces", issLine1 + "   ", issLine2 + "   "},
		{
			// Vallado's own verification file appends start/stop/step minutes
			// to every line 2. Refusing that would make the reference suite
			// unreadable.
			name:  "extra fields past column 69",
			line1: issLine1,
			line2: issLine2 + "       0.0      1440.0         120.00",
		},
	} {
		got, err := sgp4.ParseTLE(tc.line1, tc.line2)
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)

			continue
		}

		if got != want {
			t.Errorf("%s: parsed to a different element set than the plain form", tc.name)
		}
	}
}

// TestParseTLERejectsMalformedInput checks that each way of being wrong is
// reported as such, rather than silently producing a plausible orbit.
func TestParseTLERejectsMalformedInput(t *testing.T) {
	t.Parallel()

	replaceAt := func(line string, at int, with string) string {
		return line[:at] + with + line[at+len(with):]
	}

	for _, tc := range []struct {
		name         string
		line1, line2 string
		wantErr      error
	}{
		{"short line 1", issLine1[:40], issLine2, sgp4.ErrMalformedTLE},
		{"short line 2", issLine1, issLine2[:40], sgp4.ErrMalformedTLE},
		{"empty", "", "", sgp4.ErrMalformedTLE},
		{"lines swapped", issLine2, issLine1, sgp4.ErrMalformedTLE},
		{
			name:  "two different satellites",
			line1: issLine1,
			line2: replaceAt(issLine2, 2, "25545"),
			// Each line is individually intact and would checksum on its own
			// if the digits were fixed; only comparing them catches this.
			wantErr: sgp4.ErrMalformedTLE,
		},
		{"inclination is not a number", issLine1, replaceAt(issLine2, 8, " XX.XXXX"), sgp4.ErrMalformedTLE},
		{"mean motion is not a number", issLine1, replaceAt(issLine2, 52, "nonsense   "), sgp4.ErrMalformedTLE},
		{"epoch year is not a number", replaceAt(issLine1, 18, "XX"), issLine2, sgp4.ErrMalformedTLE},
		{
			// The crash the previous implementation took on this: day 400 of a
			// 366-day year, in an element set that is otherwise perfect.
			name:    "day of year past the end of the year",
			line1:   replaceAt(issLine1, 20, "400.51782528"),
			line2:   issLine2,
			wantErr: sgp4.ErrMalformedTLE,
		},
		{"day of year below 1", replaceAt(issLine1, 20, "000.51782528"), issLine2, sgp4.ErrMalformedTLE},
		{
			name:    "mean motion of zero",
			line1:   issLine1,
			line2:   replaceAt(issLine2, 52, " 0.00000000"),
			wantErr: sgp4.ErrElements,
		},
		{
			name:    "inclination past 180 degrees",
			line1:   issLine1,
			line2:   replaceAt(issLine2, 8, "251.6416"),
			wantErr: sgp4.ErrElements,
		},
	} {
		_, err := sgp4.ParseTLE(tc.line1, tc.line2)
		if err == nil {
			t.Errorf("%s: parsed without error", tc.name)

			continue
		}

		if !errors.Is(err, tc.wantErr) {
			t.Errorf("%s: error is %v, want one wrapping %v", tc.name, err, tc.wantErr)
		}
	}
}

// TestEveryFieldReportsItselfByName covers the parser's error branches one
// field at a time.
//
// These are the messages somebody debugging a feed at two in the morning reads,
// and "malformed TLE" alone does not tell them which of twenty columns to look
// at. The assertion is therefore on the text as well as the sentinel: each
// failure has to name its own field.
func TestEveryFieldReportsItselfByName(t *testing.T) {
	t.Parallel()

	replaceAt := func(line string, at int, with string) string {
		return line[:at] + with + line[at+len(with):]
	}

	for _, tc := range []struct {
		name   string
		line1  string
		line2  string
		expect string
	}{
		{"satellite number on line 2", issLine1, replaceAt(issLine2, 2, "XXXXX"), "satellite number on line 2"},
		{"first derivative", replaceAt(issLine1, 33, "nonsense10"), issLine2, "first derivative"},
		{"element set number", replaceAt(issLine1, 64, "XXXX"), issLine2, "element set number"},
		{"revolution number", issLine1, replaceAt(issLine2, 63, "XXXXX"), "revolution number"},
		{"right ascension", issLine1, replaceAt(issLine2, 17, "XXX.XXXX"), "right ascension"},
		{"argument of perigee", issLine1, replaceAt(issLine2, 34, "XXX.XXXX"), "argument of perigee"},
		{"mean anomaly", issLine1, replaceAt(issLine2, 43, "XXX.XXXX"), "mean anomaly"},
		{"eccentricity", issLine1, replaceAt(issLine2, 26, "XXXXXXX"), "eccentricity"},
		{"B* drag term", replaceAt(issLine1, 53, "!1234-4"), issLine2, "B* drag term"},
		{"second derivative", replaceAt(issLine1, 44, "!1234-4"), issLine2, "second derivative"},
		{
			// The assumed-exponent fields reassemble their pieces, so a digit
			// that is not a digit fails at ParseFloat rather than at the sign.
			name:   "B* mantissa that is not a number",
			line1:  replaceAt(issLine1, 54, "XXXXX"),
			line2:  issLine2,
			expect: "B* drag term",
		},
	} {
		_, err := sgp4.ParseTLE(tc.line1, tc.line2)
		if err == nil {
			t.Errorf("%s: parsed without error", tc.name)

			continue
		}

		if !errors.Is(err, sgp4.ErrMalformedTLE) {
			t.Errorf("%s: %v does not wrap ErrMalformedTLE", tc.name, err)
		}

		if !strings.Contains(err.Error(), tc.expect) {
			t.Errorf("%s: error is %q, which does not name the field (%q)", tc.name, err, tc.expect)
		}
	}
}

// TestVerifyTLEChecksumsRefusesANonDigit covers the one branch that is about
// the check digit's own shape rather than its value: column 69 holding
// something that is not a digit at all.
func TestVerifyTLEChecksumsRefusesANonDigit(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name         string
		line1, line2 string
	}{
		{"line 1", issLine1[:68] + "X", issLine2},
		{"line 2", issLine1, issLine2[:68] + "X"},
	} {
		err := sgp4.VerifyTLEChecksums(tc.line1, tc.line2)
		if !errors.Is(err, sgp4.ErrChecksum) {
			t.Errorf("%s: %v, want one wrapping ErrChecksum", tc.name, err)
		}
	}

	// And a structurally broken line is reported as malformed rather than as a
	// checksum failure: there is no check digit to compare yet.
	if err := sgp4.VerifyTLEChecksums("too short", issLine2); !errors.Is(err, sgp4.ErrMalformedTLE) {
		t.Errorf("a short line reported %v, want one wrapping ErrMalformedTLE", err)
	}
}

// TestTheEccentricityFieldCannotExpressAnEscapeOrbit records a property of the
// format that a test tried to assert the opposite of.
//
// The eccentricity field is seven digits after an assumed decimal point, so the
// largest value it can carry is 0.9999999 and e >= 1 is unreachable through a
// TLE at all. Elements.Validate still refuses it, because an Elements value can
// be built from an OMM or by hand — but no input to ParseTLE can reach that
// branch, and a test claiming to exercise it through this door was testing
// nothing.
//
// The near-parabolic end is legitimate and must parse: Vallado's suite carries
// an element set at e = 0.995.
func TestTheEccentricityFieldCannotExpressAnEscapeOrbit(t *testing.T) {
	t.Parallel()

	line2 := issLine2[:26] + "9999999" + issLine2[33:]

	el, err := sgp4.ParseTLE(issLine1, line2)
	if err != nil {
		t.Fatalf("the largest eccentricity the field can hold was refused: %v", err)
	}

	if el.Eccentricity != 0.9999999 {
		t.Errorf("Eccentricity = %v, want 0.9999999", el.Eccentricity)
	}

	if err := (sgp4.Elements{
		MeanMotion:   15.0,
		Eccentricity: 1.0,
		Epoch:        time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC),
	}).Validate(); !errors.Is(err, sgp4.ErrElements) {
		t.Errorf("Validate accepted e = 1: %v", err)
	}
}

// TestChecksumsAreVerifiedSeparately is the design decision of §6.5 as a test.
//
// Parsing and checksum verification answer different questions — "are these
// elements" versus "did this text arrive intact" — and keeping them apart is
// what lets a caller read a hand-maintained file and a network feed with the
// same parser.
func TestChecksumsAreVerifiedSeparately(t *testing.T) {
	t.Parallel()

	if err := sgp4.VerifyTLEChecksums(issLine1, issLine2); err != nil {
		t.Errorf("a real element set failed its own check digits: %v", err)
	}

	// Break one digit in the middle of line 2. The elements stay readable and
	// the check digit stops matching, which is exactly the split.
	broken := issLine2[:30] + "9" + issLine2[31:]

	if _, err := sgp4.ParseTLE(issLine1, broken); err != nil {
		t.Errorf("ParseTLE refused an element set with a bad check digit; that is "+
			"VerifyTLEChecksums' job, not the parser's: %v", err)
	}

	err := sgp4.VerifyTLEChecksums(issLine1, broken)
	if !errors.Is(err, sgp4.ErrChecksum) {
		t.Errorf("VerifyTLEChecksums returned %v, want one wrapping ErrChecksum", err)
	}
}

// TestChecksumCountsWhatTheFormatSays pins the rule itself.
func TestChecksumCountsWhatTheFormatSays(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		line string
		want int
	}{
		{"digits sum", "12345", 15 % 10},
		{"minus counts one", "-----", 5},
		{"plus counts nothing", "+++++", 0},
		{"letters and spaces count nothing", "AB CD.", 0},
		{"the check digit itself is excluded", strings.Repeat("9", 69), (68 * 9) % 10},
	} {
		if got := sgp4.Checksum(tc.line); got != tc.want {
			t.Errorf("%s: Checksum(%q) = %d, want %d", tc.name, tc.line, got, tc.want)
		}
	}
}

// TestParsesEveryValladoElementSet runs the parser over the reference suite
// this package will be measured against.
//
// # Why 33 and not 30
//
// Because the three that astrogo could not read before are the point. Vallado
// hand-built 33333, 33334 and 33335 to drive SGP4 into three of its error
// returns, and never maintained their check digits. A parser that enforces the
// check digit cannot read them, which is why astrogo's coverage of its own
// reference suite was 30 of 33 and the model's error paths were untested.
//
// Splitting the checksum out of the parser is what fixes that, so this asserts
// both halves: all 33 parse, and the ones with bad check digits are still
// reported as such by the separate call.
func TestParsesEveryValladoElementSet(t *testing.T) {
	t.Parallel()

	sets := loadValladoTLEs(t)

	// 33 line pairs over 32 distinct satellites: 20413 appears twice with
	// different propagation spans, which is why this counts pairs and not
	// catalogue numbers.
	const wantSets = 33

	if len(sets) != wantSets {
		t.Errorf("read %d element sets from the reference file, want %d", len(sets), wantSets)
	}

	var badChecksum []string

	for _, s := range sets {
		el, err := sgp4.ParseTLE(s.line1, s.line2)
		if err != nil {
			t.Errorf("satellite %s: ParseTLE: %v", s.satnum, err)

			continue
		}

		if el.MeanMotion <= 0 {
			t.Errorf("satellite %s: parsed a mean motion of %g", s.satnum, el.MeanMotion)
		}

		if err := sgp4.VerifyTLEChecksums(s.line1, s.line2); err != nil {
			badChecksum = append(badChecksum, s.satnum)
		}
	}

	// The three error-return cases, and only those.
	want := map[string]bool{"33333": true, "33334": true, "33335": true}

	for _, num := range badChecksum {
		if !want[num] {
			t.Errorf("satellite %s has a bad check digit and was not expected to", num)
		}

		delete(want, num)
	}

	for num := range want {
		t.Errorf("satellite %s was expected to have a bad check digit and does not — if the "+
			"fixture was regenerated, TestParsesEveryValladoElementSet's premise has changed", num)
	}
}

type valladoSet struct {
	satnum       string
	line1, line2 string
}

// loadValladoTLEs reads the reference element sets.
//
// The fixture lives with the parent package, which is where astrogo's SGP4
// validation has been until now — see its testdata/vallado/README.md for
// provenance and SHA-256s. Reading it across the package boundary rather than
// copying 30 KB of somebody else's data twice into the same repository.
func loadValladoTLEs(t *testing.T) []valladoSet {
	t.Helper()

	f, err := os.Open("../testdata/vallado/SGP4-VER.TLE")
	if err != nil {
		t.Fatalf("open the reference element sets: %v", err)
	}

	defer func() { _ = f.Close() }()

	var (
		out   []valladoSet
		line1 string
	)

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r\n ")

		switch {
		case strings.HasPrefix(line, "1 "):
			line1 = line
		case strings.HasPrefix(line, "2 "):
			out = append(out, valladoSet{
				satnum: strings.TrimLeft(strings.TrimSpace(line[2:7]), "0"),
				line1:  line1,
				line2:  line,
			})
		}
	}

	if err := sc.Err(); err != nil {
		t.Fatalf("read the reference element sets: %v", err)
	}

	return out
}
