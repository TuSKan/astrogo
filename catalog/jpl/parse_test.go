package jpl

import (
	"context"
	"errors"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/internal/testutil"
)

// The tables under testdata are Horizons' "result" text, verbatim and
// untrimmed, for the query each is named after (fetched 2026-10-07). The
// fixtures they replace were real responses too, but trimmed to the rows the
// old parser could read: the major-body one kept no row whose alias column
// was filled in without a COSPAR designation, and the small-body one was the
// five-column table. That trimming is how ISS resolving to Larissa, and
// Halley to nothing, went unnoticed.

// readFixture returns the Horizons result text stored in testdata/name.
func readFixture(t *testing.T, name string) string {
	t.Helper()

	b, err := os.ReadFile("testdata/" + name)
	testutil.AssertNoError(t, err)

	return string(b)
}

// statedMatchesRe reads the match count Horizons prints under a table:
// "Number of matches =  5 ." for major bodies, "(31 matches." for small ones.
var statedMatchesRe = regexp.MustCompile(`Number of matches =\s*(\d+)|\((\d+) matches\.`)

// statedMatches returns the number of matches result says it lists.
func statedMatches(t *testing.T, result string) int {
	t.Helper()

	m := statedMatchesRe.FindStringSubmatch(result)
	if m == nil {
		t.Fatal("fixture states no match count")
	}

	n, err := strconv.Atoi(m[1] + m[2])
	testutil.AssertNoError(t, err)

	return n
}

// resolveAll collects everything ResolveObject yields for query against a
// Horizons that answers result.
func resolveAll(t *testing.T, query, result string) []resolve.Target {
	t.Helper()

	prov := newMockProvider(t, jsonResultPayload(t, result))

	var got []resolve.Target

	prov.ResolveObject(context.Background(), resolve.ObjectRequest{Query: query})(func(tg resolve.Target, err error) bool {
		testutil.AssertNoError(t, err)

		got = append(got, tg)

		return true
	})

	return got
}

// findID returns the target with the given ID.
func findID(t *testing.T, targets []resolve.Target, id string) resolve.Target {
	t.Helper()

	for _, tg := range targets {
		if tg.ID == id {
			return tg
		}
	}

	t.Fatalf("no target with ID %q among %d", id, len(targets))

	return resolve.Target{}
}

// wantRow is one table row's expected fields. A nil aliases is not checked,
// since several alias cells hold more than one name separated by spaces,
// and an empty one asserts there are none.
type wantRow struct {
	id, name, designation string
	aliases               []string
}

func TestMajorBodyTablesReadByColumn(t *testing.T) {
	tests := []struct {
		fixture, query string
		rows           []wantRow
	}{
		{"iss.txt", "ISS", []wantRow{
			{"807", "Larissa", "", []string{"NVII"}},
			{"-74", "Mars Reconnaissance Orbiter (spacec", "2005-029A", []string{"MRO"}},
			{"-125544", "International Space Station (spacec", "1998-067A", []string{"ISS"}},
			{"2000016", "Psyche (mission target)", "", []string{}},
		}},
		{"moon.txt", "Moon", []wantRow{
			{"3", "Earth-Moon Barycenter", "", []string{"EMB"}},
			{"301", "Moon", "", []string{"Luna"}},
		}},
		{"titan.txt", "Titan", []wantRow{
			{"606", "Titan", "", []string{"SVI"}},
			{"703", "Titania", "", []string{"UIII"}},
			{"-102770", "Titan-3C RB (spacecraft)", "1967-040F", []string{}},
		}},
		{"mars.txt", "Mars", []wantRow{
			{"4", "Mars Barycenter", "", []string{}},
			{"499", "Mars", "", []string{}},
			{"-76", "Mars Science Laboratory (spacecraft", "2011-070A", nil},
		}},
		{"ace.txt", "ACE", []wantRow{
			{"-92", "ACE (spacecraft)", "1997-045A", nil},
		}},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			result := readFixture(t, tt.fixture)
			got := resolveAll(t, tt.query, result)

			if want := statedMatches(t, result); len(got) != want {
				t.Fatalf("read %d rows, Horizons lists %d", len(got), want)
			}

			for _, w := range tt.rows {
				tg := findID(t, got, w.id)
				testutil.AssertEqual(t, w.id+" Name", tg.Name, w.name)
				testutil.AssertEqual(t, w.id+" Designation", tg.Designation, w.designation)
				testutil.AssertEqual(t, w.id+" SPKID", tg.SPKID, w.id)

				if w.aliases != nil && !slices.Equal(tg.Aliases, w.aliases) {
					t.Errorf("%s Aliases = %q, want %q", w.id, tg.Aliases, w.aliases)
				}
			}
		})
	}
}

func TestSmallBodyIndexTablesReadByHeader(t *testing.T) {
	tests := []struct {
		fixture, query string
		rows           []wantRow
	}{
		// Record #, Epoch-yr, Primary Desig, >MATCH NAME<. The asteroid's
		// epoch cell is empty.
		{"halley.txt", "Halley", []wantRow{
			{"2688", "Halley", "1982 HG1", nil},
			{"90000001", "Halley", "1P", nil},
			{"90000030", "Halley", "1P", nil},
		}},
		// Record #, Epoch-yr, >MATCH DESIG<, Primary Desig, Name.
		{"73p.txt", "73P", []wantRow{
			{"90000734", "Schwassmann-Wachmann 3", "73P", nil},
			{"90000767", "Schwassmann-Wachmann 3", "73P-AA", nil},
		}},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			result := readFixture(t, tt.fixture)
			got := resolveAll(t, tt.query, result)

			if want := statedMatches(t, result); len(got) != want {
				t.Fatalf("read %d rows, Horizons lists %d", len(got), want)
			}

			for _, w := range tt.rows {
				tg := findID(t, got, w.id)
				testutil.AssertEqual(t, w.id+" Name", tg.Name, w.name)
				testutil.AssertEqual(t, w.id+" Designation", tg.Designation, w.designation)
			}

			// A record number is not an SPK-ID: SBDB gives record 2688 the
			// SPK-ID 20002688.
			for _, tg := range got {
				if tg.SPKID != "" {
					t.Errorf("record %s has SPKID %q, want none", tg.ID, tg.SPKID)
				}
			}
		})
	}
}

// TestSearchRanksTheWholeTable: every query here names its body exactly.
// Before ranking, Resolve returned another body for ISS, Moon, Mars and
// ACE, Titan with its alias in its name, and nothing for Halley.
func TestSearchRanksTheWholeTable(t *testing.T) {
	tests := []struct {
		fixture, query, want string
	}{
		{"iss.txt", "ISS", "-125544"},    // was 807 Larissa
		{"moon.txt", "Moon", "301"},      // was 3, the Earth-Moon barycenter
		{"mars.txt", "Mars", "499"},      // was 4, the Mars barycenter
		{"titan.txt", "Titan", "606"},    // was 606 with "SVI" in its name
		{"ace.txt", "ACE", "-92"},        // 53rd of 255; was -2 Mariner 2
		{"halley.txt", "Halley", "2688"}, // all 31 named Halley: Horizons' order stands
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			prov := newMockProvider(t, jsonResultPayload(t, readFixture(t, tt.fixture)))

			got, err := prov.Search(context.Background(), tt.query)
			testutil.AssertNoError(t, err)

			if len(got) == 0 || len(got) > searchLimit {
				t.Fatalf("Search returned %d targets, want 1 to %d", len(got), searchLimit)
			}

			testutil.AssertEqual(t, "Search best ID", got[0].ID, tt.want)

			best, err := prov.Resolve(context.Background(), tt.query)
			testutil.AssertNoError(t, err)
			testutil.AssertEqual(t, "Resolve ID", best.ID, tt.want)
		})
	}
}

// TestChangedTableLayoutIsNotMisread: a table whose columns are not the
// ones this package knows surfaces ErrNotImplemented instead of yielding
// targets with fields read from the wrong column.
func TestChangedTableLayoutIsNotMisread(t *testing.T) {
	tests := []struct {
		name, fixture, old, new string
	}{
		// The major-body table drops its alias column.
		{"major body", "iss.txt", "-----------  ------------------- ", "----------- "},
		// The small-body table loses the column that names a record.
		{"small body", "halley.txt", "Record #", "Rec"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := readFixture(t, tt.fixture)
			if strings.Count(result, tt.old) != 1 {
				t.Fatalf("fixture %s does not contain %q exactly once", tt.fixture, tt.old)
			}

			prov := newMockProvider(t, jsonResultPayload(t, strings.Replace(result, tt.old, tt.new, 1)))

			var gotErr error

			prov.ResolveObject(context.Background(), resolve.ObjectRequest{Query: "x"})(func(_ resolve.Target, err error) bool {
				gotErr = err

				return err == nil
			})

			if !errors.Is(gotErr, ErrNotImplemented) {
				t.Fatalf("got %v, want ErrNotImplemented", gotErr)
			}
		})
	}
}

func TestSplitTrailingParenthetical(t *testing.T) {
	tests := []struct {
		in, rest, inner string
	}{
		{"Voyager 1 (spacecraft) (-31)", "Voyager 1 (spacecraft)", "-31"},
		{"1P/Halley", "1P/Halley", ""},
		{"spacecraft)", "spacecraft)", ""},
	}

	for _, tt := range tests {
		rest, inner := splitTrailingParenthetical(tt.in)
		testutil.AssertEqual(t, tt.in+" rest", rest, tt.rest)
		testutil.AssertEqual(t, tt.in+" inner", inner, tt.inner)
	}
}
