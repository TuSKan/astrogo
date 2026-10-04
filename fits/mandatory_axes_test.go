package fits_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/fits"
)

// TestBintableNeedsItsAxes reads synthetic's three-row BINTABLE with one
// structural card spoiled. Each must be an error naming the keyword. A missing
// or malformed NAXIS2 used to give a table of no rows and no error, losing all
// three; a negative one panicked; a malformed NAXIS1 was caught only by the
// TFORM check, which said NAXIS1 "declares 0"; and a malformed PCOUNT read as
// zero (#460).
func TestBintableNeedsItsAxes(t *testing.T) {
	t.Parallel()

	good := synthetic(t)

	for _, c := range []struct {
		name, keyword, from, to string
	}{
		{"malformed NAXIS2", "NAXIS2", card("NAXIS2", "3"), card("NAXIS2", "'three'")},
		{"missing NAXIS2", "NAXIS2", card("NAXIS2", "3"), card("COMMENT", "'no row count'")},
		{"negative NAXIS2", "NAXIS2", card("NAXIS2", "3"), card("NAXIS2", "-3")},
		{"malformed NAXIS1", "NAXIS1", card("NAXIS1", "30"), card("NAXIS1", "3O")},
		{"malformed PCOUNT", "PCOUNT", card("PCOUNT", "0"), card("PCOUNT", "'none'")},
		{"negative PCOUNT", "PCOUNT", card("PCOUNT", "0"), card("PCOUNT", "-1")},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			raw := bytes.Replace(good, []byte(c.from), []byte(c.to), 1)
			if bytes.Equal(raw, good) {
				t.Fatalf("the fixture has no %q card to spoil", c.from)
			}

			if _, err := fits.Read(bytes.NewReader(raw)); err == nil || !strings.Contains(err.Error(), c.keyword) {
				t.Errorf("Read: err %v, want one naming %s", err, c.keyword)
			}
		})
	}
}

// hduBytes is one HDU as it sits in a file: its cards and END, padded to a
// block, then its data, padded to a block.
func hduBytes(data []byte, cards ...string) []byte {
	return append(pad([]byte(strings.Join(cards, "")+"END")), pad(data)...)
}

// emptyPrimary is a primary HDU with no data.
func emptyPrimary() []byte {
	return hduBytes(nil, card("SIMPLE", "T"), card("BITPIX", "8"), card("NAXIS", "0"), card("EXTEND", "T"))
}

// TestImageNeedsItsAxes: an image's size is the product of its axes, and
// every way that product can be unusable must be an error rather than a
// panic or an empty image. Before #460, a negative NAXIS1 panicked, two
// negative axes multiplied to a positive size, two axes of 2^32 wrapped to
// zero pixels and read as an empty image with no error, and a product
// wrapping negative panicked.
func TestImageNeedsItsAxes(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name, bitpix, want string
		cards              []string
	}{
		{"negative NAXIS1", "8", "NAXIS1", []string{card("NAXIS", "1"), card("NAXIS1", "-5")}},
		{"two negative axes", "8", "NAXIS1", []string{card("NAXIS", "2"), card("NAXIS1", "-2"), card("NAXIS2", "-3")}},
		{"malformed NAXIS1", "8", "NAXIS1", []string{card("NAXIS", "1"), card("NAXIS1", "'five'")}},
		// The axis list is sized from NAXIS before any NAXISn is read.
		{"NAXIS above 999", "8", "exceeds 999", []string{card("NAXIS", "1125899906842624")}},
		{"axes overflowing to zero", "8", "overflows", []string{
			card("NAXIS", "2"), card("NAXIS1", "4294967296"), card("NAXIS2", "4294967296"),
		}},
		{"axes overflowing negative", "8", "overflows", []string{
			card("NAXIS", "2"), card("NAXIS1", "4294967296"), card("NAXIS2", "3221225472"),
		}},
		// The pixel count fits; its size in bytes does not.
		{"bytes overflowing", "-64", "overflows", []string{card("NAXIS", "1"), card("NAXIS1", "4611686018427387904")}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			cards := append([]string{card("SIMPLE", "T"), card("BITPIX", c.bitpix)}, c.cards...)
			raw := hduBytes(make([]byte, 6), cards...)

			if _, err := fits.Read(bytes.NewReader(raw)); err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("Read: err %v, want one naming %s", err, c.want)
			}
		})
	}
}

// TestTruncatedUndecodedDataIsAnError: stepping over a table's undecoded data
// is a read like any other, and a file that ends inside it, or inside the
// padding after it, is an error rather than a table read short.
func TestTruncatedUndecodedDataIsAnError(t *testing.T) {
	t.Parallel()

	// No fields, so nothing is decoded; 36 rows of 80 bytes, so one block.
	header := func(xtension, naxis2 string) []byte {
		return pad([]byte(card("XTENSION", xtension) + card("BITPIX", "8") + card("NAXIS", "2") +
			card("NAXIS1", "80") + card("NAXIS2", naxis2) + card("PCOUNT", "0") + card("GCOUNT", "1") +
			card("TFIELDS", "0") + "END"))
	}

	for _, c := range []struct {
		name, want string
		hdu        []byte
	}{
		{"BINTABLE ending in its rows", "skip bintable data",
			append(header("'BINTABLE'", "36"), make([]byte, 100)...)},
		{"TABLE ending in its rows", "skip asciitable rows",
			append(header("'TABLE   '", "36"), make([]byte, 100)...)},
		{"TABLE ending in its padding", "extension padding",
			append(header("'TABLE   '", "1"), make([]byte, 80)...)},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			raw := append(emptyPrimary(), c.hdu...)

			if _, err := fits.Read(bytes.NewReader(raw)); err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("Read: err %v, want one mentioning %q", err, c.want)
			}
		})
	}
}

// TestReadRefusesASkippedHDUsStructure: an HDU that Read neither decodes nor
// understands is stepped over by the size its structural keywords give, so
// those keywords must be usable even though nothing else of it is read. They
// used to size a malformed one as zero, and the next header was read from its
// data (#460).
func TestReadRefusesASkippedHDUsStructure(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name, want string
		raw        []byte
	}{
		{"primary with a malformed BITPIX", "BITPIX",
			hduBytes(nil, card("SIMPLE", "T"), card("BITPIX", "'eight'"), card("NAXIS", "0"))},
		{"unknown extension with a malformed NAXIS1", "NAXIS1", append(emptyPrimary(),
			hduBytes(nil, card("XTENSION", "'FOREIGN '"), card("BITPIX", "8"), card("NAXIS", "1"),
				card("NAXIS1", "'x'"), card("PCOUNT", "0"), card("GCOUNT", "1"))...)},
	} {
		if _, err := fits.Read(bytes.NewReader(c.raw)); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err %v, want one naming %s", c.name, err, c.want)
		}
	}
}

// emptyImageExtension is an IMAGE extension with no data, named name.
func emptyImageExtension(name string) []byte {
	return hduBytes(nil, card("XTENSION", "'IMAGE   '"), card("BITPIX", "8"), card("NAXIS", "0"),
		card("PCOUNT", "0"), card("GCOUNT", "1"), card("EXTNAME", "'"+name+"'"))
}

// TestUndecodedTableDataIsConsumed: a table with no fields, or no rows, has
// nothing to decode, but its rows and heap are still in the file. Returning
// without stepping over them left the next HDU's header to be read from this
// one's data (#460).
//
// Each table's data is one block that happens to read as a header, an IMAGE
// named INSIDE, and the table is followed by an IMAGE named AFTER. A reader
// that steps over the data sees three HDUs, the last AFTER; one that does not
// finds INSIDE as well. Data that does not parse as a header is no test: the
// header reader skips it and finds AFTER either way.
func TestUndecodedTableDataIsConsumed(t *testing.T) {
	t.Parallel()

	inside := emptyImageExtension("INSIDE")
	if len(inside) != fits.BlockSize {
		t.Fatalf("the decoy is %d bytes, want one block", len(inside))
	}

	for _, c := range []struct {
		name  string
		table []byte
	}{
		{"BINTABLE with no fields", hduBytes(inside,
			card("XTENSION", "'BINTABLE'"), card("BITPIX", "8"), card("NAXIS", "2"),
			card("NAXIS1", "80"), card("NAXIS2", "36"), card("PCOUNT", "0"), card("GCOUNT", "1"),
			card("TFIELDS", "0"))},
		{"BINTABLE with no rows and a heap", hduBytes(inside,
			card("XTENSION", "'BINTABLE'"), card("BITPIX", "8"), card("NAXIS", "2"),
			card("NAXIS1", "4"), card("NAXIS2", "0"), card("PCOUNT", "2880"), card("GCOUNT", "1"),
			card("TFIELDS", "1"), card("TFORM1", "'J       '"))},
		{"TABLE with no fields", hduBytes(inside,
			card("XTENSION", "'TABLE   '"), card("BITPIX", "8"), card("NAXIS", "2"),
			card("NAXIS1", "80"), card("NAXIS2", "36"), card("PCOUNT", "0"), card("GCOUNT", "1"),
			card("TFIELDS", "0"))},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			raw := append(append(emptyPrimary(), c.table...), emptyImageExtension("AFTER")...)

			f, err := fits.Read(bytes.NewReader(raw))
			if err != nil {
				t.Fatalf("Read: %v", err)
			}

			if len(f.HDUs) != 3 {
				t.Fatalf("Read: %d HDUs, want 3", len(f.HDUs))
			}

			name, err := f.HDUs[2].Header().GetString("EXTNAME")
			if err != nil || strings.TrimSpace(name) != "AFTER" {
				t.Errorf("third HDU EXTNAME %q, err %v; want AFTER", name, err)
			}
		})
	}
}
