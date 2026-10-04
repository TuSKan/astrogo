package fits

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// headerOf builds a header from keyword/value pairs, in order.
func headerOf(pairs ...string) *Header {
	h := NewHeader()
	for i := 0; i+1 < len(pairs); i += 2 {
		h.Append(Card{Keyword: pairs[i], Value: pairs[i+1]})
	}

	return h
}

// TestASCIITableNeedsItsAxes: NAXIS1 and NAXIS2 are mandatory in a TABLE
// header (FITS 4.0 §7.2.1). Missing, malformed or negative, each is an error
// naming the keyword; NAXIS2 used to read as zero rows, with no error (#460).
func TestASCIITableNeedsItsAxes(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name, keyword, naxis1, naxis2 string
	}{
		{"malformed NAXIS2", "NAXIS2", "10", "'two'"},
		{"missing NAXIS2", "NAXIS2", "10", ""},
		{"negative NAXIS2", "NAXIS2", "10", "-2"},
		{"malformed NAXIS1", "NAXIS1", "1O", "2"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			pairs := []string{"XTENSION", "'TABLE   '", "BITPIX", "8", "NAXIS", "2", "NAXIS1", c.naxis1}
			if c.naxis2 != "" {
				pairs = append(pairs, "NAXIS2", c.naxis2)
			}

			pairs = append(pairs, "PCOUNT", "0", "GCOUNT", "1", "TFIELDS", "1",
				"TTYPE1", "'NAME'", "TBCOL1", "1", "TFORM1", "'A10'")

			_, err := ReadASCIITable(headerOf(pairs...), bytes.NewReader(blankPayload("")))
			if err == nil || !strings.Contains(err.Error(), c.keyword) {
				t.Errorf("ReadASCIITable: err %v, want one naming %s", err, c.keyword)
			}
		})
	}
}

// TestPayloadSizeNeedsItsStructure: the size of an HDU that is skipped rather
// than decoded comes from BITPIX, NAXIS and NAXISn, which are mandatory, and
// GCOUNT and PCOUNT, which default only when absent. A malformed one used to
// size the payload as zero, so the next header was read out of its data
// (#460).
func TestPayloadSizeNeedsItsStructure(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name, keyword string
		header        *Header
	}{
		{"malformed BITPIX", "BITPIX", headerOf("BITPIX", "'eight'", "NAXIS", "0")},
		{"missing NAXIS", "NAXIS", headerOf("BITPIX", "8")},
		{"negative NAXIS1", "NAXIS1", headerOf("BITPIX", "8", "NAXIS", "1", "NAXIS1", "-4")},
		{"malformed GCOUNT", "GCOUNT", headerOf("BITPIX", "8", "NAXIS", "1", "NAXIS1", "4", "GCOUNT", "'one'")},
		{"malformed PCOUNT", "PCOUNT", headerOf("BITPIX", "8", "NAXIS", "1", "NAXIS1", "4", "PCOUNT", "'none'")},
		{"negative GCOUNT", "GCOUNT", headerOf("BITPIX", "8", "NAXIS", "1", "NAXIS1", "4", "GCOUNT", "-1")},
		{"negative PCOUNT", "PCOUNT", headerOf("BITPIX", "8", "NAXIS", "1", "NAXIS1", "4", "PCOUNT", "-4")},
		{"NAXIS above 999", "exceeds 999", headerOf("BITPIX", "8", "NAXIS", "1000")},
	} {
		if _, err := payloadSize(c.header); err == nil || !strings.Contains(err.Error(), c.keyword) {
			t.Errorf("%s: err %v, want one naming %s", c.name, err, c.keyword)
		}
	}

	// Sizes that cannot be held are errDataSize, never a wrapped number: two
	// axes of 2^32 wrapped to zero, so the payload was not skipped at all.
	for _, c := range []struct {
		name   string
		header *Header
	}{
		{"axes", headerOf("BITPIX", "8", "NAXIS", "2", "NAXIS1", "4294967296", "NAXIS2", "4294967296")},
		{"bytes", headerOf("BITPIX", "-64", "NAXIS", "1", "NAXIS1", "4611686018427387904")},
		{"PCOUNT", headerOf("BITPIX", "8", "NAXIS", "1", "NAXIS1", "9223372036854775807", "PCOUNT", "1")},
		{"padding", headerOf("BITPIX", "8", "NAXIS", "1", "NAXIS1", "9223372036854775807")},
	} {
		if _, err := payloadSize(c.header); !errors.Is(err, errDataSize) {
			t.Errorf("%s overflowing: err %v, want errDataSize", c.name, err)
		}
	}

	// Absent, GCOUNT and PCOUNT take their defaults: 1000 bytes of BITPIX 8
	// in one group, padded to a block.
	size, err := payloadSize(headerOf("BITPIX", "8", "NAXIS", "1", "NAXIS1", "1000"))
	if err != nil || size != BlockSize {
		t.Errorf("payloadSize without GCOUNT or PCOUNT: %d, err %v; want %d", size, err, BlockSize)
	}
}
