package fits_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/fits"
)

// The two registered conventions, tested from both ends: a card this package
// writes has to come back as the same card, and a card written by somebody
// else — spelled out here as the bytes ESO and the OGIP convention prescribe —
// has to be read correctly.
//
// The second half is what matters most. A round trip only proves the writer and
// the reader agree; these files come from instruments, and the layouts below are
// quoted from the conventions rather than produced by this package.

// TestReadsHIERARCHFromESOStyleBytes reads a header written the way an ESO
// instrument writes one.
//
// Without the convention these parse as a keyword of "HIERARCH" with the whole
// rest as a comment — so every one of an ESO header's dozens of instrument
// keywords is lost, silently, and a caller asking for one gets ErrKeyNotFound.
func TestReadsHIERARCHFromESOStyleBytes(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		card    string
		keyword string
		value   string
		comment string
	}{
		{
			card:    "HIERARCH ESO DET CHIP1 ID = 'CCD44-82' / Detector chip identification",
			keyword: "ESO DET CHIP1 ID",
			value:   "CCD44-82",
			comment: "Detector chip identification",
		},
		{
			card:    "HIERARCH ESO INS GRAT1 WLEN = 600.0 / [nm] Grating wavelength",
			keyword: "ESO INS GRAT1 WLEN",
			value:   "600.0",
			comment: "[nm] Grating wavelength",
		},
		{
			// Extra spacing between tokens is normalised, so a header that
			// aligns its keywords does not produce a different one.
			card:    "HIERARCH ESO  DET   EXPTIME = 1200 / seconds",
			keyword: "ESO DET EXPTIME",
			value:   "1200",
			comment: "seconds",
		},
	} {
		t.Run(tc.keyword, func(t *testing.T) {
			t.Parallel()

			got := fits.ParseCard([]byte(pad80(tc.card)))

			if got.Keyword != tc.keyword {
				t.Errorf("keyword = %q, want %q", got.Keyword, tc.keyword)
			}

			if got.Comment != tc.comment {
				t.Errorf("comment = %q, want %q", got.Comment, tc.comment)
			}

			h := fits.NewHeader()
			h.Append(got)

			if v, err := h.GetString(tc.keyword); err != nil || v != tc.value {
				t.Errorf("GetString(%q) = %q (%v), want %q", tc.keyword, v, err, tc.value)
			}
		})
	}
}

// TestReadsCONTINUEFromOGIPStyleBytes reads a long string spelled out as the
// convention prescribes.
//
// Without it the value ends at the first card and keeps the "&" that marks the
// split — a path or a URL silently truncated to sixty-odd characters, with the
// truncation marker left in place as though it were data.
func TestReadsCONTINUEFromOGIPStyleBytes(t *testing.T) {
	t.Parallel()

	raw := headerBlock(t,
		"SIMPLE  =                    T / conforms to FITS standard",
		"BITPIX  =                    8",
		"NAXIS   =                    0",
		"LONGSTRN= 'OGIP 1.0'           / the OGIP long-string convention is in use",
		"FILENAME= 'a-very-long-path/that-does-not-fit-in-one-eighty-byte-record/at&'",
		"CONTINUE  'all-of-it-so-it-continues-onto-a-second-and-then-a-third-card&'",
		"CONTINUE  '-ending-here.fits' / the file this was reduced from",
		"END",
	)

	f, err := fits.Read(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	want := "a-very-long-path/that-does-not-fit-in-one-eighty-byte-record/at" +
		"all-of-it-so-it-continues-onto-a-second-and-then-a-third-card" +
		"-ending-here.fits"

	got, err := f.HDUs[0].Header().GetString("FILENAME")
	if err != nil {
		t.Fatalf("GetString: %v", err)
	}

	if got != want {
		t.Errorf("FILENAME =\n %q\nwant\n %q", got, want)
	}

	if strings.Contains(got, "&") {
		t.Error("the continuation marker survived into the value")
	}

	// The comment on the final card belongs to the whole value.
	card, err := f.HDUs[0].Header().Get("FILENAME")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if card.Comment != "the file this was reduced from" {
		t.Errorf("comment = %q, want the final card's", card.Comment)
	}
}

// TestAStrayCONTINUEIsNotGluedOntoItsNeighbour keeps the join from inventing a
// value. A CONTINUE card after something that was not waiting for one is
// malformed, and swallowing it would silently corrupt the previous keyword.
func TestAStrayCONTINUEIsNotGluedOntoItsNeighbour(t *testing.T) {
	t.Parallel()

	raw := headerBlock(t,
		"SIMPLE  =                    T",
		"BITPIX  =                    8",
		"NAXIS   =                    0",
		"OBJECT  = 'M31'                / complete, no continuation marker",
		"CONTINUE  'orphaned'",
		"END",
	)

	f, err := fits.Read(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	if got, err := f.HDUs[0].Header().GetString("OBJECT"); err != nil || got != "M31" {
		t.Errorf("OBJECT = %q (%v), want M31 — a stray CONTINUE was joined onto it", got, err)
	}
}

// TestLongKeywordRoundTripsThroughHIERARCH is the writer's half.
func TestLongKeywordRoundTripsThroughHIERARCH(t *testing.T) {
	t.Parallel()

	const (
		keyword = "ESO DET CHIP1 ID"
		value   = "CCD44-82"
	)

	src := float32Image(t, 2, 2, []float32{1, 2, 3, 4})
	src.Header().Append(fits.Card{
		Keyword: keyword,
		Value:   "'" + value + "'",
		Comment: "Detector chip identification",
	})

	var buf bytes.Buffer
	if err := fits.Write(&buf, &fits.File{HDUs: []fits.HDU{src}}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// The card has to be written as HIERARCH, not as a truncated keyword.
	if !bytes.Contains(buf.Bytes()[:fits.BlockSize], []byte("HIERARCH "+keyword+" = ")) {
		t.Fatalf("no HIERARCH card in the header:\n%s", firstBlock(buf.Bytes()))
	}

	got, err := fits.Read(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("Read back: %v", err)
	}

	if v, err := got.HDUs[0].Header().GetString(keyword); err != nil || v != value {
		t.Errorf("%s = %q (%v), want %q", keyword, v, err, value)
	}
}

// TestLongStringRoundTripsThroughCONTINUE is the other writer half, and checks
// the two things a reader depends on: the "&" marker on every card but the
// last, and LONGSTRN announcing that the convention is in use.
func TestLongStringRoundTripsThroughCONTINUE(t *testing.T) {
	t.Parallel()

	// Long enough to need three cards.
	value := "/data/raw/2026-09-11/" + strings.Repeat("segment-of-a-long-path/", 6) + "file.fits"

	src := float32Image(t, 2, 2, []float32{1, 2, 3, 4})
	src.Header().Append(fits.Card{
		Keyword: "FILENAME",
		Value:   "'" + value + "'",
		Comment: "source file",
	})

	var buf bytes.Buffer
	if err := fits.Write(&buf, &fits.File{HDUs: []fits.HDU{src}}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	header := buf.Bytes()[:fits.BlockSize]

	if !bytes.Contains(header, []byte("CONTINUE")) {
		t.Fatalf("a %d-character value was written without CONTINUE cards:\n%s",
			len(value), firstBlock(buf.Bytes()))
	}

	if !bytes.Contains(header, []byte("LONGSTRN= 'OGIP 1.0'")) {
		t.Error("LONGSTRN is missing, so a reader without the convention has no way " +
			"to know the value it read ends early")
	}

	got, err := fits.Read(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("Read back: %v", err)
	}

	card, err := got.HDUs[0].Header().Get("FILENAME")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if v := mustGetString(t, got.HDUs[0].Header(), "FILENAME"); v != value {
		t.Errorf("FILENAME =\n %q\nwant\n %q", v, value)
	}

	if card.Comment != "source file" {
		t.Errorf("comment = %q, want it carried onto the final card", card.Comment)
	}
}

// A quote inside a string value is escaped by doubling it, and must not
// accumulate over a round trip.
func TestQuoteInAStringValueSurvivesUndoubled(t *testing.T) {
	t.Parallel()

	const value = "O'Brien's field"

	src := float32Image(t, 2, 2, []float32{1, 2, 3, 4})
	src.Header().Append(fits.Card{Keyword: "OBSERVER", Value: fits.QuoteValue(value)})

	var buf bytes.Buffer
	if err := fits.Write(&buf, &fits.File{HDUs: []fits.HDU{src}}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := fits.Read(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("Read back: %v", err)
	}

	if v := mustGetString(t, got.HDUs[0].Header(), "OBSERVER"); v != value {
		t.Errorf("OBSERVER = %q, want %q", v, value)
	}
}

// headerBlock assembles cards into a padded FITS block.
func headerBlock(t *testing.T, cards ...string) []byte {
	t.Helper()

	var b bytes.Buffer

	for _, c := range cards {
		if len(c) > fits.CardSize {
			t.Fatalf("test fixture card is %d characters: %q", len(c), c)
		}

		b.WriteString(pad80(c))
	}

	for b.Len()%fits.BlockSize != 0 {
		b.WriteByte(' ')
	}

	return b.Bytes()
}

// firstBlock renders a header block as lines, for a failure message.
func firstBlock(raw []byte) string {
	var b strings.Builder

	for off := 0; off+fits.CardSize <= len(raw) && off < fits.BlockSize; off += fits.CardSize {
		line := strings.TrimRight(string(raw[off:off+fits.CardSize]), " ")
		if line == "" {
			continue
		}

		b.WriteString("  " + line + "\n")
	}

	return b.String()
}

// mustGetString reads a string keyword or fails the test.
func mustGetString(t *testing.T, h *fits.Header, keyword string) string {
	t.Helper()

	v, err := h.GetString(keyword)
	if err != nil {
		t.Fatalf("GetString(%q): %v", keyword, err)
	}

	return v
}
