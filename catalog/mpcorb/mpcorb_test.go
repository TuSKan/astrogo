package mpcorb_test

import (
	"bytes"
	"compress/gzip"
	"errors"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/catalog/mpcorb"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/ephemeris/kepler"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/time"
)

// fixture is five verbatim rows of the MPC's Distant.txt and NEA.txt, fetched
// 2026-09-08. Literals rather than a checked-in file, so they run offline as
// ordinary tests and a reviewer can see the 202 columns being counted.
//
// Each row is here for something it breaks:
//
//   - 00433 Eros — the ordinary case, and an object whose orbit is known well
//     enough to check the numbers against a published source.
//   - 00944 Hidalgo — a numbered object with a name.
//   - K04PB2C — no H and no G at all. The whole magnitude block is blank.
//   - ~000v — the base-62 packed number the MPC uses above 620000, which no
//     amount of digit-parsing will read.
//   - J94T00G — a 1994 epoch, so the century marker is J and not K, and an
//     eccentricity of exactly zero, which a "missing means zero" reader could
//     not tell from a blank.
const fixture = `00433   10.39  0.15 K2669  62.51145  178.91814  304.26797   10.82855  0.2228780  0.55970463   1.4582437  0 E2026-R13 18046  59 1893-2026 0.61 M-v 3Ek MPCORBFIT  1804    (433) Eros               20260902
00944   10.55  0.15 K2669 199.68322   56.62737   21.33337   42.52433  0.6620703  0.07188129   5.7287361  0 MPO964281  1959  31 1920-2024 0.68 M-v 3Ek MPCLINUX   0000    (944) Hidalgo            20240512
K04PB2C             K048N   0.03358  277.30161   69.53412    2.35778  0.0417836  0.00333727  44.3481040  E MPO 70391     4   1   29 days 0.23         MPCM       200A          2004 PC112         20040911
~000v    6.39  0.15 K2669 345.61933   56.19354  134.59964   26.11425  0.2400784  0.00247747  54.0916422  4 MPO733961    93  13 2005-2023 0.19 M-v 3Ek Pan        000A (620057) 2005 CG81          20230318
J94T00G  7.0   0.15 J949P   0.00000  353.02318   15.50983    6.76386  0.0000000  0.00358836  42.2543833  E MPC 24084     8   1    3 days              Marsden    200A          1994 TG            19941006
`

func readAll(t *testing.T, body string) []resolve.Target {
	t.Helper()

	var out []resolve.Target

	for tgt, err := range mpcorb.Read(strings.NewReader(body)) {
		if err != nil {
			t.Fatalf("Read: %v", err)
		}

		out = append(out, tgt)
	}

	return out
}

func find(t *testing.T, list []resolve.Target, id string) resolve.Target {
	t.Helper()

	for _, tgt := range list {
		if tgt.ID == id {
			return tgt
		}
	}

	t.Fatalf("packed designation %q missing from %d parsed rows", id, len(list))

	return resolve.Target{}
}

// TestReadParsesEveryColumn checks the six elements and the magnitude block
// against one row read by hand.
//
// The columns are the whole risk in this parser. Every field is a number in a
// fixed position with no separator guaranteed, so a boundary off by one still
// parses — it just returns a different number, and an inclination read as a
// node produces an orbit that propagates perfectly to the wrong place.
func TestReadParsesEveryColumn(t *testing.T) {
	t.Parallel()

	eros := find(t, readAll(t, fixture), "00433")

	if eros.Name != "(433) Eros" {
		t.Errorf("Name = %q, want %q — the readable designation runs to column 194", eros.Name, "(433) Eros")
	}

	if !eros.HasElements {
		t.Fatal("HasElements is false; nothing downstream will look at the elements")
	}

	for _, tc := range []struct {
		name string
		got  float64
		want float64
	}{
		{"semi-major axis (AU)", eros.SemiMajorAxis, 1.4582437},
		{"eccentricity", eros.Eccentricity, 0.2228780},
		{"inclination (deg)", eros.Inclination.Degrees(), 10.82855},
		{"ascending node (deg)", eros.AscendingNode.Degrees(), 304.26797},
		{"argument of perihelion (deg)", eros.ArgPeriapsis.Degrees(), 178.91814},
		{"mean anomaly (deg)", eros.MeanAnomaly.Degrees(), 62.51145},
		{"H", eros.H, 10.39},
		{"G", eros.G, 0.15},
	} {
		testutil.AssertNear(t, tc.name, tc.got, tc.want, 1e-9)
	}

	if eros.Kind != resolve.KindAsteroid {
		t.Errorf("Kind = %q, want %q", eros.Kind, resolve.KindAsteroid)
	}

	if eros.Catalog != "mpcorb" {
		t.Errorf("Catalog = %q, want mpcorb", eros.Catalog)
	}
}

// TestReadDistinguishesAMissingMagnitudeFromZero is the error-vs-absence case
// in this file.
//
// A few rows carry no H at all — one in the whole of Distant.txt. Defaulting
// it to zero would make an object nobody has measured the brightest thing in
// the solar system, and every magnitude-limited query would return it.
func TestReadDistinguishesAMissingMagnitudeFromZero(t *testing.T) {
	t.Parallel()

	list := readAll(t, fixture)

	blank := find(t, list, "K04PB2C")
	if blank.HasH {
		t.Errorf("HasH is true with H = %v, but the row's magnitude columns are blank", blank.H)
	}

	// And a row that does have one must set the flag, or the distinction is
	// being made by always answering "absent".
	if measured := find(t, list, "00433"); !measured.HasH {
		t.Error("HasH is false for 433 Eros, whose H is 10.39")
	}

	// An eccentricity of exactly zero is a published value, not a gap. It has
	// no flag of its own, so the only protection is that the parser never
	// invents one.
	circular := find(t, list, "J94T00G")
	if circular.Eccentricity != 0 {
		t.Errorf("eccentricity = %v, want exactly 0 as published", circular.Eccentricity)
	}

	if !circular.HasElements {
		t.Error("a circular orbit is still an orbit; HasElements must be true")
	}
}

// TestReadKeepsThePackedDesignationAndTheReadableOne covers the two names a row
// carries, including the base-62 form the MPC uses above asteroid 620000.
//
// The packed designation is kept as the ID and never decoded: the readable
// column is right there in the same row, so decoding "~000v" would be
// reimplementing a mapping the file already contains — and getting it wrong
// silently, since a wrong decode still looks like a designation.
func TestReadKeepsThePackedDesignationAndTheReadableOne(t *testing.T) {
	t.Parallel()

	list := readAll(t, fixture)

	for _, tc := range []struct{ id, name string }{
		{"00433", "(433) Eros"},
		{"00944", "(944) Hidalgo"},
		{"K04PB2C", "2004 PC112"},
		{"~000v", "(620057) 2005 CG81"},
		{"J94T00G", "1994 TG"},
	} {
		got := find(t, list, tc.id)
		if got.Name != tc.name {
			t.Errorf("%s: Name = %q, want %q", tc.id, got.Name, tc.name)
		}

		if got.Designation != tc.name {
			t.Errorf("%s: Designation = %q, want %q", tc.id, got.Designation, tc.name)
		}
	}
}

// TestParseEpochDecodesThePackedDate covers the MPC's five-character date,
// which is the one field in the record with no plain-text alternative.
//
// The cases are the boundaries of the packing rather than a sample of it: the
// two century markers that occur in the real files, the letter months, and the
// letter days up to V for the 31st.
func TestParseEpochDecodesThePackedDate(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		packed string
		y      int
		m      time.Month
		d      int
	}{
		{"K2669", 2026, time.June, 9},       // from the fixture rows
		{"K048N", 2004, time.August, 23},    // letter day
		{"J949P", 1994, time.September, 25}, // 1900s century marker
		{"K25BL", 2025, time.November, 21},  // letter month and letter day
		{"K023R", 2002, time.March, 27},     // a leading-zero year
		{"I801V", 1880, time.January, 31},   // 1800s marker; the year is 80, not 01
		{"K19C1", 2019, time.December, 1},   // December as C
		{"J86A9", 1986, time.October, 9},    // October as A
	} {
		got, err := mpcorb.ParseEpoch(tc.packed)
		if err != nil {
			t.Errorf("ParseEpoch(%q): %v", tc.packed, err)

			continue
		}

		// Compared as a Julian Date against the same calendar date built
		// through time's own constructor, rather than by reading fields back:
		// the epoch is on the TT scale and a field-by-field comparison would
		// pass for a value labelled with the wrong scale.
		want := time.Date(tc.y, tc.m, tc.d, 0, 0, 0, 0, time.LocationUTC).JD()

		if got.JD() != want {
			t.Errorf("ParseEpoch(%q).JD() = %.6f, want %.6f (%d-%02d-%02d)",
				tc.packed, got.JD(), want, tc.y, tc.m, tc.d)
		}

		if got.Scale() != time.TT {
			t.Errorf("ParseEpoch(%q) scale = %v, want TT — MPCORB epochs are TT and "+
				"reading them as UTC is wrong by delta-T", tc.packed, got.Scale())
		}
	}
}

// TestParseEpochRejectsWhatItCannotDecode pins that a date it does not
// understand is an error rather than a plausible wrong day.
//
// Every one of these would otherwise produce a Time: a missing character
// shifts the fields, an unknown century marker could default to 2000, and '0'
// as a month is only invalid if something checks.
func TestParseEpochRejectsWhatItCannotDecode(t *testing.T) {
	t.Parallel()

	for _, packed := range []string{
		"",       // empty
		"K266",   // one short
		"K26690", // one long
		"L2669",  // century marker the MPC does not use
		"K2X69",  // non-numeric year
		"K2609",  // month zero
		"K2660",  // day zero
		"K26W9",  // month W decodes to 32
		"K266W",  // day W decodes to 32
	} {
		if _, err := mpcorb.ParseEpoch(packed); !errors.Is(err, mpcorb.ErrMalformedEpoch) {
			t.Errorf("ParseEpoch(%q) err = %v, want ErrMalformedEpoch", packed, err)
		}
	}
}

// TestParseEpochNamesTheOffendingCharacter pins which of two guards rejects a
// zero month or day.
//
// Both refuse it, so the sentinel alone cannot tell them apart and a mutation
// that let '0' through unpackDigit survived the test above. The difference is
// what the caller is told: the character that is wrong, or a decoded date that
// was never in the file.
func TestParseEpochNamesTheOffendingCharacter(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct{ packed, want string }{
		{"K2609", `"0" is not 1-9 or A-Z`}, // month
		{"K2660", `"0" is not 1-9 or A-Z`}, // day
	} {
		_, err := mpcorb.ParseEpoch(tc.packed)
		if err == nil {
			t.Fatalf("ParseEpoch(%q) succeeded", tc.packed)
		}

		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("ParseEpoch(%q) err = %q, want it to contain %q — a decoded "+
				"month 0 names a date the file never held", tc.packed, err, tc.want)
		}
	}
}

// TestReadSkipsTheHeaderWithoutCountingLines covers MPCORB.DAT's preamble,
// which the subset files do not have.
//
// Skipping a fixed number of lines, or scanning for the rule of dashes, both
// encode a fact about today's header. A row is recognised by being a row —
// wide enough to reach the readable designation, with an eccentricity where
// the eccentricity goes — so a header that grows, shrinks or loses its rule
// changes nothing.
func TestReadSkipsTheHeaderWithoutCountingLines(t *testing.T) {
	t.Parallel()

	header := strings.Join([]string{
		"Orbit information for numbered and multi-opposition objects.",
		"",
		"The following file gives orbital elements in the MPCORB export format.",
		"",
		strings.Repeat("-", 202),
		"",
	}, "\n") + "\n"

	list := readAll(t, header+fixture)

	if len(list) != 5 {
		t.Fatalf("parsed %d rows from a header plus 5 element rows, want 5", len(list))
	}

	// The rule of dashes is 202 characters, exactly a row's width, so it
	// reaches the readable-designation column and is only rejected because
	// its eccentricity field is not a number.
	if got := find(t, list, "00433"); got.Name != "(433) Eros" {
		t.Errorf("first data row = %q, want (433) Eros", got.Name)
	}
}

// TestReadYieldsABadRowAndKeepsGoing pins the deliberate difference from this
// project's other MPC reader.
//
// plan's observatory-code list refuses to load at all on a malformed row,
// because it is 2,700 rows and is the authority on which codes exist. This
// file is over 1.5 million rows — 317 MB at 203 bytes each — and one
// unreadable object is not a reason to deny a caller the rest of them. It is
// a reason to say so, rather than to drop it silently.
func TestReadYieldsABadRowAndKeepsGoing(t *testing.T) {
	t.Parallel()

	// Corrupt Hidalgo's inclination, leaving its width and every other field
	// intact so the row is still recognised as a row.
	broken := strings.Replace(fixture, "   42.52433  0.6620703", "   4x.52433  0.6620703", 1)
	if broken == fixture {
		t.Fatal("the fixture no longer contains the substring this test corrupts")
	}

	var (
		good []resolve.Target
		errs []error
	)

	for tgt, err := range mpcorb.Read(strings.NewReader(broken)) {
		if err != nil {
			errs = append(errs, err)

			continue
		}

		good = append(good, tgt)
	}

	if len(errs) != 1 {
		t.Fatalf("got %d errors, want exactly 1: %v", len(errs), errs)
	}

	if !errors.Is(errs[0], mpcorb.ErrMalformedRow) {
		t.Errorf("err = %v, want ErrMalformedRow", errs[0])
	}

	if !strings.Contains(errs[0].Error(), "00944") {
		t.Errorf("err = %v, want the offending object's packed designation", errs[0])
	}

	if len(good) != 4 {
		t.Errorf("got %d good rows alongside the bad one, want 4", len(good))
	}
}

// TestReadStopsWhenTheCallerStops covers the early break, which is the whole
// reason this is an iterator: MPCORB.DAT is 317 MB and a caller wanting fifty
// objects should read fifty rows.
func TestReadStopsWhenTheCallerStops(t *testing.T) {
	t.Parallel()

	seen := 0

	for range mpcorb.Read(strings.NewReader(fixture)) {
		seen++

		if seen == 2 {
			break
		}
	}

	if seen != 2 {
		t.Errorf("read %d rows after breaking at 2", seen)
	}
}

// TestReadDetectsGzipFromTheStream covers MPCORB.DAT.gz, which is a third the
// size of the file it holds.
//
// Sniffed from the first two bytes rather than from a name, so a caller
// holding a compressed copy under any name still reads it and one holding an
// uncompressed copy named .gz is not broken by the suffix.
func TestReadDetectsGzipFromTheStream(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write([]byte(fixture)); err != nil {
		t.Fatalf("gzip write: %v", err)
	}

	if err := gz.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}

	var list []resolve.Target

	for tgt, err := range mpcorb.Read(&buf) {
		if err != nil {
			t.Fatalf("Read: %v", err)
		}

		list = append(list, tgt)
	}

	if len(list) != 5 {
		t.Fatalf("parsed %d rows from the gzip stream, want 5", len(list))
	}
}

// TestParsedElementsPropagate is the check that the numbers mean what the
// column map says they mean.
//
// Every assertion above compares a parsed float against the text it came from,
// which a consistently wrong column map would also satisfy. This one hands the
// elements to the propagator that will actually consume them: an element set
// with the node in the inclination's place fails kepler's own validation, and
// one that survives it still has to produce a heliocentric distance inside the
// range its own a and e allow.
func TestParsedElementsPropagate(t *testing.T) {
	t.Parallel()

	eros := find(t, readAll(t, fixture), "00433")

	el, err := kepler.NewElements(eros.Epoch, eros.SemiMajorAxis, eros.Eccentricity,
		eros.Inclination, eros.AscendingNode, eros.ArgPeriapsis, eros.MeanAnomaly)
	if err != nil {
		t.Fatalf("kepler.NewElements from a parsed row: %v", err)
	}

	pos, _, err := el.StateAt(eros.Epoch)
	if err != nil {
		t.Fatalf("StateAt: %v", err)
	}

	// a(1-e) <= r <= a(1+e) for any point on the ellipse. Eros: 1.133 AU to
	// 1.783 AU. A column map off by one field puts r outside this.
	//
	// StateAt returns astronomical units, not metres — its perifocal vector is
	// built straight from the semi-major axis it was given.
	r := pos.Norm()

	perihelion := eros.SemiMajorAxis * (1 - eros.Eccentricity)
	aphelion := eros.SemiMajorAxis * (1 + eros.Eccentricity)

	if r < perihelion-1e-6 || r > aphelion+1e-6 {
		t.Errorf("heliocentric distance %.6f AU is outside [%.6f, %.6f] AU, which its own "+
			"a and e make impossible", r, perihelion, aphelion)
	}

	t.Logf("433 Eros at its own epoch: %.4f AU (perihelion %.4f, aphelion %.4f)", r, perihelion, aphelion)
}
