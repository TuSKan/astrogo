package fits

import (
	"encoding/binary"
	"fmt"
	"math"
	"strconv"

	"github.com/apache/arrow-go/v18/arrow"
)

// encodeImage builds an image HDU's header and payload.
func encodeImage(h *ImageHDU, primary, extensions bool) (*Header, []byte, error) {
	bitpix, pixelBytes, err := bitpixOf(h)
	if err != nil {
		return nil, nil, err
	}

	mandatory := imageKeywords(h, bitpix, primary, extensions)

	header := orderedHeader(h.Header(), mandatory)

	if err := appendScaling(header, h); err != nil {
		return nil, nil, err
	}

	payload, err := imagePayload(h, pixelBytes)
	if err != nil {
		return nil, nil, err
	}

	return header, payload, nil
}

// imageKeywords builds the structural cards, in the order the standard fixes.
func imageKeywords(h *ImageHDU, bitpix int, primary, extensions bool) []Card {
	naxis := len(h.Axes)

	first := Card{Keyword: "SIMPLE", Value: "T", Comment: "conforms to FITS standard"}
	if !primary {
		first = Card{Keyword: "XTENSION", Value: "'IMAGE   '", Comment: "image extension"}
	}

	cards := []Card{
		first,
		{Keyword: "BITPIX", Value: strconv.Itoa(bitpix), Comment: "bits per data pixel"},
		{Keyword: "NAXIS", Value: strconv.Itoa(naxis), Comment: "number of data axes"},
	}

	// Axes are held C-contiguous — slowest-varying first — which is the
	// reverse of the NAXISn order ReadImage flipped them out of. Flipping
	// back here rather than storing both is what keeps the two paths from
	// disagreeing about which end is which.
	for i := 1; i <= naxis; i++ {
		cards = append(cards, Card{
			Keyword: "NAXIS" + strconv.Itoa(i),
			Value:   strconv.FormatInt(h.Axes[naxis-i], 10),
			Comment: "length of data axis " + strconv.Itoa(i),
		})
	}

	// EXTEND announces that extensions follow, and the standard puts it
	// immediately after the last NAXISn in the primary header. It is not
	// decoration: a reader is entitled to stop at the primary HDU without it,
	// so a file carrying a table nobody can find is the failure mode. astropy
	// and cfitsio both write it.
	if primary && extensions {
		cards = append(cards, Card{
			Keyword: "EXTEND", Value: "T", Comment: "file contains extensions",
		})
	}

	// An extension declares its group structure; the primary HDU does not.
	// Both are fixed for an image: no random-group parameters, one group.
	if !primary {
		cards = append(cards,
			Card{Keyword: "PCOUNT", Value: "0", Comment: "number of parameters"},
			Card{Keyword: "GCOUNT", Value: "1", Comment: "number of groups"},
		)
	}

	return cards
}

// appendScaling writes the calibration keywords, and only the ones that say
// something.
//
// BSCALE 1 and BZERO 0 are the defaults a reader assumes, so writing them adds
// two cards that mean "nothing to see". BLANK is different: it is written only
// when the HDU actually carries one, because BLANK declares a pixel value to
// be read as undefined, and inventing one would reinterpret whatever pixels
// happen to hold that value.
func appendScaling(header *Header, h *ImageHDU) error {
	if h.BScale != 0 && h.BScale != 1 {
		v, err := formatFloat(h.BScale)
		if err != nil {
			return fmt.Errorf("BSCALE: %w", err)
		}

		setCard(header, "BSCALE", v, "linear scaling factor")
	}

	if h.BZero != 0 {
		v, err := formatFloat(h.BZero)
		if err != nil {
			return fmt.Errorf("BZERO: %w", err)
		}

		setCard(header, "BZERO", v, "zero point of scaling")
	}

	if h.HasBlank {
		setCard(header, "BLANK", strconv.FormatInt(h.Blank, 10), "undefined pixel value")
	}

	return nil
}

// bitpixOf returns the BITPIX to write and the width of one pixel.
//
// The HDU's own Bitpix is authoritative when set, since that is what a read
// produced and what a caller adjusting a header would change. When it is zero
// — a tensor built in memory rather than read from a file — it is derived from
// the tensor's element type, so the common case of constructing an image from
// Arrow data needs no FITS knowledge at the call site.
func bitpixOf(h *ImageHDU) (bitpix, pixelBytes int, err error) {
	bitpix = h.Bitpix

	if bitpix == 0 && h.Tensor != nil {
		bitpix, err = bitpixForType(h.Tensor.DataType())
		if err != nil {
			return 0, 0, err
		}
	}

	if len(h.Axes) == 0 && h.Tensor == nil {
		// A header-only HDU: the primary HDU of a file whose data lives in
		// extensions. BITPIX still has to be a legal value, and 8 is the
		// conventional choice for "there are no pixels".
		if bitpix == 0 {
			bitpix = BitpixUint8
		}
	}

	switch bitpix {
	case BitpixUint8:
		return bitpix, 1, nil
	case BitpixInt16:
		return bitpix, 2, nil
	case BitpixInt32, BitpixFloat32:
		return bitpix, 4, nil
	case BitpixInt64, BitpixFloat64:
		return bitpix, 8, nil
	default:
		return 0, 0, fmt.Errorf("%w: %d", ErrInvalidBitpix, bitpix)
	}
}

// bitpixForType maps an Arrow element type to its FITS BITPIX.
//
// FITS has no unsigned integer types beyond 8-bit; a uint16 image is written
// as int16 with BZERO 32768, which this does not do silently — a caller with
// unsigned data has to say what they meant, because guessing would write
// values that read back as negative.
//
// The default rejects every Arrow type FITS has no BITPIX for; listing the
// forty it cannot store would say the same thing at much greater length.
//
//nolint:exhaustive // the default covers every unlisted type, as above
func bitpixForType(dt arrow.DataType) (int, error) {
	switch dt.ID() {
	case arrow.UINT8:
		return BitpixUint8, nil
	case arrow.INT16:
		return BitpixInt16, nil
	case arrow.INT32:
		return BitpixInt32, nil
	case arrow.INT64:
		return BitpixInt64, nil
	case arrow.FLOAT32:
		return BitpixFloat32, nil
	case arrow.FLOAT64:
		return BitpixFloat64, nil
	default:
		return 0, fmt.Errorf("%w: no BITPIX for Arrow type %s", ErrNotWritable, dt)
	}
}

// imagePayload returns the pixel bytes in FITS byte order.
//
// The tensor's buffer is copied rather than swapped in place: it belongs to
// the caller, who did not ask for their image to be byte-reversed as a side
// effect of writing it.
func imagePayload(h *ImageHDU, pixelBytes int) ([]byte, error) {
	if h.Tensor == nil {
		return nil, nil
	}

	want := int64(1)
	for _, n := range h.Axes {
		want *= n
	}

	buffers := h.Tensor.Data().Buffers()
	if len(buffers) < 2 || buffers[1] == nil {
		return nil, fmt.Errorf("%w: image tensor holds no data buffer", ErrNotWritable)
	}

	raw := buffers[1].Bytes()

	// The axes and the buffer have to agree, or the file will describe more
	// or fewer pixels than it carries — which no reader can detect, because
	// the header is the only statement of the shape.
	if int64(len(raw)) < want*int64(pixelBytes) {
		return nil, fmt.Errorf("%w: axes describe %d pixels (%d bytes) but the tensor holds %d bytes",
			ErrNotWritable, want, want*int64(pixelBytes), len(raw))
	}

	out := make([]byte, want*int64(pixelBytes))
	copy(out, raw)

	swapNativeToBigEndian(out, pixelBytes)

	return out, nil
}

// swapNativeToBigEndian converts a buffer from native to FITS byte order in
// place. It is the inverse of swapBigEndianToNative, and on a big-endian
// machine both are nothing.
func swapNativeToBigEndian(buf []byte, bytesPerPixel int) {
	switch bytesPerPixel {
	case 1:
		// A single byte has no order.
	case 2:
		for i := 0; i+2 <= len(buf); i += 2 {
			binary.BigEndian.PutUint16(buf[i:], binary.NativeEndian.Uint16(buf[i:]))
		}
	case 4:
		for i := 0; i+4 <= len(buf); i += 4 {
			binary.BigEndian.PutUint32(buf[i:], binary.NativeEndian.Uint32(buf[i:]))
		}
	case 8:
		for i := 0; i+8 <= len(buf); i += 8 {
			binary.BigEndian.PutUint64(buf[i:], binary.NativeEndian.Uint64(buf[i:]))
		}
	}
}

// formatFloat renders a float for a header card.
//
// 'G' with -1 precision gives the shortest representation that reads back as
// the same float64, which is what keeps a value written and re-read from
// drifting. FITS accepts E or D exponents; Go writes E, which is legal.
//
// NaN and the infinities are refused. A FITS header has no representation for
// them, and Go would render them as "NaN" and "+Inf" — text no reader can
// parse as a number, in a card that claims to be one. A BSCALE that arrived as
// NaN is a caller's bug worth reporting, not one to encode into a file that
// then fails somewhere else.
func formatFloat(v float64) (string, error) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return "", fmt.Errorf("%w: %v has no FITS representation", ErrCardNotFinite, v)
	}

	return strconv.FormatFloat(v, 'G', -1, 64), nil
}
