package plan

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
)

// mpcFixture is a verbatim excerpt of the IAU Minor Planet Center's
// ObsCodes.html, fetched 2026-09-08, kept as a literal rather than a checked-in
// file for the same reason the fuzz corpora are (see CLAUDE.md): it runs as an
// ordinary offline test and a reviewer can see the bytes being parsed.
//
// The rows are chosen for what each one breaks:
//
//   - 000, 568, 950 — space-separated, five-decimal constants. The ordinary case.
//   - 005, 309, 474 — no separator at all between fields. "005   2.231000.659891"
//     is longitude 2.23100, ρcosφ′ 0.659891; whitespace-splitting reads it as one
//     number and drops the row while looking like it worked.
//   - 250, 500 — no parallax constants: Hubble, and the geocentre.
//   - Z99 — a lettered code, and the last row of the file.
//   - 807 — negative ρsinφ′, southern hemisphere.
//
// The rows are deliberately NOT in code order. Written sorted, the fixture
// would pass TestParseMPCObsCodesSortsByCode with the sort removed — the check
// would be reading the input back rather than testing anything.
const mpcFixture = `<pre>
Code  Long.   cos      sin    Name
Z99 359.978740.595468+0.800687Clixby Observatory, Cleethorpes
309 289.595690.909943-0.414336Cerro Paranal
000   0.0000 0.62411 +0.77873 Greenwich
950 342.1176 0.87764 +0.47847 La Palma
005   2.231000.659891+0.748875Meudon
807 289.1941 0.86560 -0.49980 Cerro Tololo Observatory, La Serena
250                           Hubble Space Telescope
568 204.5278 0.94171 +0.33725 Maunakea
474 170.464960.720773-0.691079Mount John Observatory, Lake Tekapo
500   0.0000 0.00000 +0.00000 Geocentric
</pre>
`

func parseFixture(t *testing.T) []MPCObservatory {
	t.Helper()

	list, err := parseMPCObsCodes(strings.NewReader(mpcFixture))
	if err != nil {
		t.Fatalf("parseMPCObsCodes: %v", err)
	}

	return list
}

func findObs(t *testing.T, list []MPCObservatory, code string) MPCObservatory {
	t.Helper()

	for _, o := range list {
		if o.Code == code {
			return o
		}
	}

	t.Fatalf("code %q missing from the parsed list of %d", code, len(list))

	return MPCObservatory{}
}

// TestParseMPCObsCodesReadsFieldsWithNoSeparator is the test that matters most
// in this file.
//
// The MPC's columns are fixed-width and run together whenever a value fills its
// field, so a third of the register looks like "005   2.231000.659891+0.748875".
// Splitting on whitespace parses that as a single token, and the natural
// recovery — skip the row — loses a third of the observatories silently while
// every remaining row still checks out.
func TestParseMPCObsCodesReadsFieldsWithNoSeparator(t *testing.T) {
	t.Parallel()

	list := parseFixture(t)

	const wantRows = 10
	if len(list) != wantRows {
		t.Fatalf("parsed %d rows, want %d — a run-together row was dropped", len(list), wantRows)
	}

	// Meudon: longitude 2.23100 E, and a latitude near Paris's.
	meudon := findObs(t, list, "005")
	if meudon.Location == nil {
		t.Fatal("005 Meudon has no position; its fields ran together and were not read")
	}

	testutil.AssertNear(t, "005 longitude", meudon.Location.Lon().Degrees(), 2.23100, 1e-5)
	testutil.AssertNear(t, "005 latitude", meudon.Location.Lat().Degrees(), 48.805, 0.01)

	if meudon.Name != "Meudon" {
		t.Errorf("005 name = %q, want Meudon — the name column starts where the last constant ends", meudon.Name)
	}
}

// TestParseMPCObsCodesRecoversKnownPositions checks the coordinate change
// against sites whose real coordinates this package already holds from an
// independent source.
//
// The tolerances are the measured disagreement, not a physical bound, and they
// are loose for a documented reason: the MPC publishes parallax constants to
// four to six decimals, and one unit in the last place of a five-decimal
// constant is 64 m on the ground. Where the two sources disagree by more than
// that — La Palma's 12″ — they are describing different points, the MPC coding
// one telescope and KnownSites naming the observatory.
func TestParseMPCObsCodesRecoversKnownPositions(t *testing.T) {
	t.Parallel()

	list := parseFixture(t)

	for _, tc := range []struct {
		code    string
		known   string
		latTolD float64
		lonTolD float64
		hTolM   float64
	}{
		{"000", "greenwich", 0.001, 0.001, 30},
		{"309", "paranal", 0.001, 0.01, 30},
		{"568", "mauna_kea", 0.001, 0.01, 100},
		{"950", "la_palma", 0.01, 0.02, 100},
	} {
		t.Run(tc.code, func(t *testing.T) {
			t.Parallel()

			obs := findObs(t, list, tc.code)
			if obs.Location == nil {
				t.Fatalf("%s has no recovered position", tc.code)
			}

			want, ok := KnownSites[tc.known]
			if !ok {
				t.Fatalf("KnownSites has no %q; this test's reference is gone", tc.known)
			}

			got := obs.Location
			ref := want.Location()

			// Longitude is compared as a wrapped difference: the MPC writes
			// east longitude in [0, 360) and coord reports it in (-180, 180],
			// so 289.6 and -70.4 are the same meridian and a raw subtraction
			// makes them 360 apart.
			dLon := math.Mod(got.Lon().Degrees()-ref.Lon().Degrees()+540, 360) - 180

			if math.Abs(dLon) > tc.lonTolD {
				t.Errorf("longitude %.5f vs %.5f (KnownSites), differ by %.5f deg > %.5f",
					got.Lon().Degrees(), ref.Lon().Degrees(), dLon, tc.lonTolD)
			}

			testutil.AssertNear(t, "latitude", got.Lat().Degrees(), ref.Lat().Degrees(), tc.latTolD)
			testutil.AssertNear(t, "height", got.Height(), ref.Height(), tc.hTolM)
		})
	}
}

// TestParseMPCObsCodesKeepsPositionlessCodes covers the thirty entries that are
// valid codes with nowhere to stand.
//
// Dropping them would be the easy reading of "no coordinates", and it would
// make NewMPCSite tell a caller asking about Hubble that there is no such code
// — which is false, and sends them looking for a typo.
func TestParseMPCObsCodesKeepsPositionlessCodes(t *testing.T) {
	t.Parallel()

	list := parseFixture(t)

	hubble := findObs(t, list, "250")
	if hubble.Location != nil {
		t.Errorf("250 Hubble has a position %v; the MPC publishes none", hubble.Location)
	}

	if hubble.Name != "Hubble Space Telescope" {
		t.Errorf("250 name = %q, want Hubble Space Telescope", hubble.Name)
	}

	// The geocentre publishes constants — all zero — so it is a *positioned*
	// row, not a positionless one. It is here to pin that the two cases are
	// told apart by what the file says rather than by whether the position
	// looks sensible.
	geo := findObs(t, list, "500")
	if geo.Location == nil {
		t.Fatal("500 Geocentric lost its position; its constants are present and zero")
	}
}

// TestParseMPCObsCodesRejectsAnEmptyDocument covers the failure a truncated or
// redirected fetch produces.
//
// A successful parse of a document with no rows is indistinguishable from a
// register that has none, and the second cannot happen. Returning an empty list
// would make every subsequent lookup answer ErrUnknownSite — a wrong answer
// delivered with confidence, rather than a fetch problem.
func TestParseMPCObsCodesRejectsAnEmptyDocument(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		body string
	}{
		{"empty", ""},
		{"wrapper only", "<pre>\nCode  Long.   cos      sin    Name\n</pre>\n"},
		{"an HTML error page", "<html><head><title>404</title></head></html>\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := parseMPCObsCodes(strings.NewReader(tc.body))
			if !errors.Is(err, ErrNoMPCObservatories) {
				t.Errorf("err = %v, want ErrNoMPCObservatories", err)
			}
		})
	}
}

// TestParseMPCObsCodesRejectsAMalformedRow pins that a row with a broken number
// fails the whole parse rather than disappearing from the list.
//
// A dropped row is the worse outcome: the register is the authority on which
// codes exist, so an observatory missing from it reads as an observatory that
// does not exist.
func TestParseMPCObsCodesRejectsAMalformedRow(t *testing.T) {
	t.Parallel()

	broken := strings.Replace(mpcFixture,
		"000   0.0000 0.62411 +0.77873 Greenwich",
		"000   0.0000 0.6x411 +0.77873 Greenwich", 1)

	_, err := parseMPCObsCodes(strings.NewReader(broken))
	if !errors.Is(err, ErrMalformedMPCRow) {
		t.Fatalf("err = %v, want ErrMalformedMPCRow", err)
	}

	if !strings.Contains(err.Error(), "000") {
		t.Errorf("err = %v, want the offending code in the message", err)
	}
}

// TestParseMPCObsCodesSortsByCode pins the documented order, which is what
// makes the list usable for a binary search or a stable display.
func TestParseMPCObsCodesSortsByCode(t *testing.T) {
	t.Parallel()

	list := parseFixture(t)

	for i := 1; i < len(list); i++ {
		if list[i-1].Code >= list[i].Code {
			t.Fatalf("codes out of order at %d: %q then %q", i, list[i-1].Code, list[i].Code)
		}
	}
}

// TestLookupMPCSiteTellsTheThreeOutcomesApart is the error-vs-absence check for
// this lookup.
//
// "There is no such code" and "that code names something that is not on Earth"
// are different facts, and only one of them means the caller made a mistake. A
// single ErrUnknownSite for both sends someone hunting for a typo in "250",
// which is Hubble and is spelled correctly.
func TestLookupMPCSiteTellsTheThreeOutcomesApart(t *testing.T) {
	t.Parallel()

	list := parseFixture(t)

	t.Run("a site on the ground", func(t *testing.T) {
		t.Parallel()

		site, err := lookupMPCSite(list, "568")
		testutil.AssertNoError(t, err)

		if site.MPCCode() != "568" {
			t.Errorf("MPCCode = %q, want 568 — a site built this way must say where it came from", site.MPCCode())
		}

		if site.Name() != "Maunakea" {
			t.Errorf("Name = %q, want the MPC's own designation", site.Name())
		}

		if site.Location() == nil {
			t.Fatal("site has no location")
		}
	})

	t.Run("a code with no ground position", func(t *testing.T) {
		t.Parallel()

		_, err := lookupMPCSite(list, "250")
		if !errors.Is(err, ErrSiteNotOnEarth) {
			t.Fatalf("err = %v, want ErrSiteNotOnEarth", err)
		}

		if errors.Is(err, ErrUnknownSite) {
			t.Error("a valid code reported as unknown; the caller would go looking for a typo")
		}

		if !strings.Contains(err.Error(), "Hubble") {
			t.Errorf("err = %v, want the site's name so the caller can see what they asked for", err)
		}
	})

	t.Run("a code that does not exist", func(t *testing.T) {
		t.Parallel()

		_, err := lookupMPCSite(list, "ZZZ")
		if !errors.Is(err, ErrUnknownSite) {
			t.Fatalf("err = %v, want ErrUnknownSite", err)
		}

		if errors.Is(err, ErrSiteNotOnEarth) {
			t.Error("an unknown code reported as a valid one with no position")
		}
	})
}

// TestLookupMPCSiteNormalizesTheCode covers the spellings a caller actually
// types. MPC codes are printed lowercase in plenty of places and arrive with
// stray spaces from configuration files.
func TestLookupMPCSiteNormalizesTheCode(t *testing.T) {
	t.Parallel()

	list := parseFixture(t)

	for _, spelling := range []string{"Z99", "z99", " Z99 ", "\tz99\n"} {
		site, err := lookupMPCSite(list, spelling)
		if err != nil {
			t.Errorf("lookupMPCSite(%q): %v", spelling, err)

			continue
		}

		if site.MPCCode() != "Z99" {
			t.Errorf("lookupMPCSite(%q) resolved to %q, want Z99", spelling, site.MPCCode())
		}
	}
}

// TestLookupMPCSiteLeavesTheTimeZoneUnset pins a deliberate omission.
//
// The MPC list has no time-zone column, and deriving one from a longitude is
// wrong across most of the world — every country that does not use its
// nautical zone, which is most of them. A Site defaulting to UTC is honest
// about knowing nothing; a Site claiming UTC-5 for a site in Peru is not.
func TestLookupMPCSiteLeavesTheTimeZoneUnset(t *testing.T) {
	t.Parallel()

	list := parseFixture(t)

	site, err := lookupMPCSite(list, "807")
	testutil.AssertNoError(t, err)

	if got := site.TimeZone().String(); got != "UTC" {
		t.Errorf("TimeZone = %q, want UTC — the MPC list carries no zone to read", got)
	}
}

// TestMPCObservatoryResolutionTracksThePublishedDecimals covers the field that
// exists so a caller can tell a 3-metre row from a 3-kilometre one.
//
// The register mixes both — 1,322 rows at six decimals and 38 at three — and
// nothing in a recovered latitude and height says which you are holding. A
// resolution that did not track the decimals would be worse than none: it would
// look like a guarantee.
func TestMPCObservatoryResolutionTracksThePublishedDecimals(t *testing.T) {
	t.Parallel()

	list := parseFixture(t)

	// Half a unit in the last place, scaled by the equatorial radius:
	// 6 decimals -> 0.0000005 * 6378137 m = 3.2 m, and a factor of ten per
	// decimal below that.
	for _, tc := range []struct {
		code  string
		wantM float64
	}{
		{"005", 3.19},  // 0.659891 / +0.748875 — six decimals
		{"000", 31.89}, // 0.62411 / +0.77873  — five
		{"568", 31.89}, // 0.94171 / +0.33725  — five
		{"Z99", 3.19},  // 0.595468 / +0.800687 — six
	} {
		obs := findObs(t, list, tc.code)
		testutil.AssertNear(t, tc.code+" ResolutionM", obs.ResolutionM, tc.wantM, 0.01)
	}

	// A row with no constants has no resolution to report, and zero is the
	// honest answer rather than the finest one.
	if got := findObs(t, list, "250").ResolutionM; got != 0 {
		t.Errorf("250 Hubble ResolutionM = %v, want 0 — it publishes no constants", got)
	}
}

// TestMPCResolutionTakesTheCoarserConstant pins that a row whose two constants
// disagree in precision is reported at the worse of them.
//
// Taking the finer would describe a position by its better half, which is the
// direction that overstates what is known.
func TestMPCResolutionTakesTheCoarserConstant(t *testing.T) {
	t.Parallel()

	// Six decimals of longitude-side constant against three of the other.
	mixed := "007   2.336750.659470+0.749   Mixed precision\n"

	list, err := parseMPCObsCodes(strings.NewReader("<pre>\n" + mixed + "</pre>\n"))
	testutil.AssertNoError(t, err)

	if len(list) != 1 {
		t.Fatalf("parsed %d rows, want 1", len(list))
	}

	// 3 decimals: 0.0005 * 6378137 m = 3189 m, not the 3.19 m the six-decimal
	// constant on its own would claim.
	testutil.AssertNear(t, "ResolutionM", list[0].ResolutionM, 3189.07, 0.1)
}
