package fits

import (
	"bytes"
	"math"
	"testing"

	"github.com/apache/arrow-go/v18/arrow/array"
)

// An ASCII table stores values as text in fixed-width columns positioned by
// TBCOLn. These tests read the layout FITS 4.0 §7.2 prescribes, written out as
// bytes, rather than only round-tripping through this package's own writer.

// asciiTableHeader builds a TABLE header for the fixtures below.
func asciiTableHeader(rowSize int, columns ...[3]string) *Header {
	h := NewHeader()
	h.Append(Card{Keyword: "XTENSION", Value: "'TABLE   '"})
	h.Append(Card{Keyword: "BITPIX", Value: "8"})
	h.Append(Card{Keyword: "NAXIS", Value: "2"})
	h.Append(Card{Keyword: "NAXIS1", Value: itoa(rowSize)})
	h.Append(Card{Keyword: "NAXIS2", Value: "2"})
	h.Append(Card{Keyword: "PCOUNT", Value: "0"})
	h.Append(Card{Keyword: "GCOUNT", Value: "1"})
	h.Append(Card{Keyword: "TFIELDS", Value: itoa(len(columns))})

	for i, c := range columns {
		n := itoa(i + 1)
		h.Append(Card{Keyword: "TTYPE" + n, Value: "'" + c[0] + "'"})
		h.Append(Card{Keyword: "TBCOL" + n, Value: c[1]})
		h.Append(Card{Keyword: "TFORM" + n, Value: "'" + c[2] + "'"})
	}

	return h
}

// itoa is strconv.Itoa, spelled short because these fixtures use it a lot.
func itoa(v int) string {
	return string(appendInt(nil, v))
}

// appendInt renders v without importing strconv into a test file that needs
// nothing else from it.
func appendInt(dst []byte, v int) []byte {
	if v == 0 {
		return append(dst, '0')
	}

	var digits [20]byte

	i := len(digits)

	for v > 0 {
		i--
		digits[i] = byte('0' + v%10)
		v /= 10
	}

	return append(dst, digits[i:]...)
}

// TestReadASCIITableDecodesItsColumns is the behaviour that was missing: the
// reader used to consume the payload and return a header, so every ASCII table
// in every archive arrived empty.
func TestReadASCIITableDecodesItsColumns(t *testing.T) {
	t.Parallel()

	h := asciiTableHeader(28,
		[3]string{"NAME", "1", "A8"},
		[3]string{"RA", "10", "F9.4"},
		[3]string{"COUNT", "20", "I9"},
	)

	// Columns at 1, 10 and 20, one-based, exactly as TBCOLn declares: an
	// eight-column name, a gap, nine columns of RA, a gap, nine of count.
	rows := "Vega    " + " " + " 279.2347" + " " + "    91262" +
		"Betelgeu" + " " + "  88.7929" + " " + "    27989"

	payload := blankPayload(rows)

	hdu, err := ReadASCIITable(h, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("ReadASCIITable: %v", err)
	}

	if hdu.Batch == nil {
		t.Fatal("no batch decoded; the reader is still a stub")
	}

	names, ok := hdu.Batch.Column(0).(*array.String)
	if !ok {
		t.Fatalf("NAME is %T, want *array.String", hdu.Batch.Column(0))
	}

	if got := names.Value(0); got != "Vega" {
		t.Errorf("NAME row 0 = %q, want Vega", got)
	}

	ra, ok := hdu.Batch.Column(1).(*array.Float64)
	if !ok {
		t.Fatalf("RA is %T, want *array.Float64", hdu.Batch.Column(1))
	}

	if got := ra.Value(0); math.Abs(got-279.2347) > 1e-9 {
		t.Errorf("RA row 0 = %v, want 279.2347", got)
	}

	counts, ok := hdu.Batch.Column(2).(*array.Float64)
	if !ok {
		t.Fatalf("COUNT is %T, want *array.Float64", hdu.Batch.Column(2))
	}

	if got := counts.Value(0); got != 91262 {
		t.Errorf("COUNT row 0 = %v, want 91262", got)
	}
}

// TestASCIIBlankFieldIsNullNotZero covers the format's own way of saying "no
// value".
//
// There is no TNULL in an ASCII table: a field of all blanks is undefined. A
// reader that parses it as zero turns a missing measurement into a real one,
// which is the same failure the binary table's TNULL handling exists to avoid.
func TestASCIIBlankFieldIsNullNotZero(t *testing.T) {
	t.Parallel()

	h := asciiTableHeader(18,
		[3]string{"NAME", "1", "A8"},
		[3]string{"FLUX", "10", "F9.3"},
	)

	// Eight columns of name, a gap, nine of flux. The second row's flux field
	// is blank, which is how the format says the value is undefined.
	rows := "Vega    " + " " + "    1.234" +
		"Unknown " + " " + "         "

	payload := blankPayload(rows)

	hdu, err := ReadASCIITable(h, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("ReadASCIITable: %v", err)
	}

	flux := hdu.Batch.Column(1)

	if flux.IsNull(0) {
		t.Error("row 0 has a value and came back null")
	}

	if !flux.IsNull(1) {
		t.Errorf("a blank field read as %s, want null — a blank is undefined, not zero",
			flux.ValueStr(1))
	}
}

// TestReadASCIITableRefusesAMalformedHeader keeps the reader from guessing at
// a layout it was not given. Column positions are the only thing separating
// one field from the next.
func TestReadASCIITableRefusesAMalformedHeader(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		header *Header
		name   string
	}{
		{
			name:   "no TFIELDS",
			header: NewHeader(),
		},
		{
			name: "column runs past the row",
			header: asciiTableHeader(10,
				[3]string{"NAME", "1", "A20"},
			),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if _, err := ReadASCIITable(tc.header, bytes.NewReader(make([]byte, 2880))); err == nil {
				t.Error("a malformed TABLE header was accepted")
			}
		})
	}
}

// blankPayload puts rows into a block padded with spaces.
//
// Spaces, not zeros: an ASCII table's padding is blank text, and a NUL byte in
// a numeric field is not a blank — it is a character that will not parse.
func blankPayload(rows string) []byte {
	payload := make([]byte, 2880)
	for i := range payload {
		payload[i] = ' '
	}

	copy(payload, rows)

	return payload
}
