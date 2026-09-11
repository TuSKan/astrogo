package fits

import (
	"bytes"
	"fmt"
	"io"
	"strings"
)

// Write encodes f as a FITS stream.
//
// It writes the HDUs in the order they appear: the first is the primary HDU
// and the rest are extensions, which is the structure [Read] produces, so a
// file read by this package and written straight back out is the same file.
//
// # Why there is no path-taking form
//
// [Open] takes a path because an arbitrary FITS file on the user's disk is one
// of the two places this module deliberately touches the filesystem. Writing
// does not need a second exception: a caller who wants a file writes
//
//	f, err := os.Create("out.fits")
//	...
//	err = fits.Write(f, file)
//
// and everything else — a bucket, a socket, a buffer, a gzip stream — works
// through the same function rather than through a variant per destination.
func Write(w io.Writer, f *File) error {
	if f == nil || len(f.HDUs) == 0 {
		return ErrNoPrimaryHDU
	}

	for i, hdu := range f.HDUs {
		if err := writeHDU(w, hdu, i == 0); err != nil {
			return fmt.Errorf("fits: write HDU %d: %w", i, err)
		}
	}

	return nil
}

// writeHDU writes one header and its payload, each padded to a block boundary.
func writeHDU(w io.Writer, hdu HDU, primary bool) error {
	header, payload, err := encodeHDU(hdu, primary)
	if err != nil {
		return err
	}

	if err := writeHeader(w, header); err != nil {
		return err
	}

	return writePayload(w, payload, hdu.Type())
}

// encodeHDU produces the header an HDU should carry and the bytes of its
// payload.
//
// The header is rebuilt from the HDU's own structure rather than trusted as
// stored, because the two can disagree: a caller who appends a row to a table
// or replaces an image's pixels has changed NAXIS2 or BITPIX without touching
// the header. Deriving the structural keywords from the data is what stops a
// file describing itself wrongly — the failure that makes a FITS file unusable
// while looking perfectly well-formed.
//
// Everything that is not structural — WCS, provenance, the caller's own
// keywords — is carried over untouched.
func encodeHDU(hdu HDU, primary bool) (*Header, []byte, error) {
	switch h := hdu.(type) {
	case *ImageHDU:
		return encodeImage(h, primary)
	case *BintableHDU:
		if primary {
			return nil, nil, fmt.Errorf("%w: a binary table cannot be the primary HDU", ErrNotWritable)
		}

		return encodeBintable(h)
	default:
		return nil, nil, fmt.Errorf("%w: %T", ErrNotWritable, hdu)
	}
}

// writeHeader renders every card, appends END, and pads to a block boundary
// with blanks.
func writeHeader(w io.Writer, h *Header) error {
	var buf bytes.Buffer

	for _, c := range h.Cards {
		// END is written below, after every other card, so a header that
		// already carries one from a read does not end early.
		if strings.EqualFold(strings.TrimSpace(c.Keyword), "END") {
			continue
		}

		card, err := formatCard(c)
		if err != nil {
			return err
		}

		buf.Write(card)
	}

	end, err := formatCard(Card{Keyword: "END"})
	if err != nil {
		return err
	}

	buf.Write(end)

	pad(&buf, ' ')

	_, err = w.Write(buf.Bytes())
	if err != nil {
		return fmt.Errorf("fits: write header: %w", err)
	}

	return nil
}

// writePayload writes a data payload padded to a block boundary.
//
// The padding byte is not the same for every HDU kind: the standard fills an
// ASCII table's last block with spaces and everything else with zeros. A zero
// byte inside an ASCII table's padding is not blank text, and readers that
// take the remainder of the block as a row have been seen to notice.
func writePayload(w io.Writer, payload []byte, kind HDUType) error {
	if len(payload) == 0 {
		return nil
	}

	var buf bytes.Buffer

	buf.Write(payload)

	fill := byte(0)
	if kind == HDUTypeASCII {
		fill = ' '
	}

	pad(&buf, fill)

	if _, err := w.Write(buf.Bytes()); err != nil {
		return fmt.Errorf("fits: write payload: %w", err)
	}

	return nil
}

// pad extends buf to the next block boundary. A buffer already on one is left
// alone — FITS pads to fill a block, it does not append an empty one.
func pad(buf *bytes.Buffer, fill byte) {
	if rem := buf.Len() % BlockSize; rem != 0 {
		buf.Write(bytes.Repeat([]byte{fill}, BlockSize-rem))
	}
}

// setCard writes keyword through a header, replacing any existing value.
//
// [Header.Append] already overwrites in place, which is what keeps a rebuilt
// structural keyword in the position it was read at rather than moving it to
// the end — and position matters for the first few, which the standard fixes
// in order.
func setCard(h *Header, keyword, value, comment string) {
	h.Append(Card{Keyword: keyword, Value: value, Comment: comment})
}

// orderedHeader returns a header carrying the mandatory keywords first, in the
// order the standard fixes them, followed by everything else from src.
//
// FITS does not merely prefer this order, it requires it: SIMPLE or XTENSION
// first, then BITPIX, then NAXIS, then NAXISn. [VerifyPrimaryHeader] enforces
// the first three on read, so a header this package wrote and could not read
// back would be a plain bug.
//
// Keywords the caller supplied that collide with a structural one are dropped
// in favour of the value derived from the data, for the reason given on
// [encodeHDU].
func orderedHeader(src *Header, mandatory []Card) *Header {
	out := NewHeader()

	structural := make(map[string]bool, len(mandatory))

	for _, c := range mandatory {
		out.Append(c)
		structural[strings.ToUpper(strings.TrimSpace(c.Keyword))] = true
	}

	if src == nil {
		return out
	}

	for _, c := range src.Cards {
		kw := strings.ToUpper(strings.TrimSpace(c.Keyword))
		if structural[kw] || kw == "END" {
			continue
		}

		out.Append(c)
	}

	return out
}
