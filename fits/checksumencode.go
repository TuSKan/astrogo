package fits

import (
	"bytes"
	"strconv"
)

// The FITS checksum convention: DATASUM over the data, CHECKSUM over the whole
// HDU, both as header cards.
//
// Defined by Seaman, Pence & Rots, "Checksum Keyword Convention" — a registered
// FITS convention, implemented by cfitsio's fits_write_chksum and by astropy's
// checksum= option. The point of it is that a file carries its own integrity
// check, so a truncated transfer or a flipped bit is detectable by the reader
// rather than by whoever eventually looks at the pixels.
//
// The arrangement is:
//
//   - DATASUM is the 1's complement 32-bit sum of the data bytes, written as a
//     decimal string.
//   - CHECKSUM is a 16-character ASCII encoding chosen so that the 1's
//     complement sum of the entire HDU — header including the CHECKSUM card
//     itself, plus data — comes out all ones.
//
// That last property is what an external verifier tests, and what
// TestChecksumSatisfiesTheConventionsOwnTest asserts here.

// checksumPlaceholder is the CHECKSUM value written before the sum is known.
//
// Sixteen ASCII zeros, not blanks, and the difference is the whole arithmetic.
// The encoding builds each character as 0x30 plus a share of the value, so the
// field it replaces has to already contain 0x30s: then the sum rises by exactly
// the encoded value, and adding the complement of the original sum brings the
// total to all ones. Sixteen blanks would leave the total short by
// 4*(0x30-0x20) per word, and the file would not verify anywhere.
//
// The length matters for a second reason: the checksum covers the header that
// contains it, so a card that changed length afterwards would invalidate the
// number it holds.
const checksumPlaceholder = "'0000000000000000'"

// checksumExcluded are the ASCII punctuation characters the encoding steps
// around.
//
// They are excluded so the resulting string survives being written, read and
// compared as a FITS string value — a quote or a slash inside it would end the
// value or start a comment.
var checksumExcluded = [13]byte{
	0x3a, 0x3b, 0x3c, 0x3d, 0x3e, 0x3f, 0x40, // : ; < = > ? @
	0x5b, 0x5c, 0x5d, 0x5e, 0x5f, 0x60, // [ \ ] ^ _ `
}

// encodeChecksum turns a 32-bit complemented checksum into the convention's
// 16-character ASCII form.
//
// A transcription of the reference implementation's char_encode. Each of the
// four bytes is spread over four characters as a quotient and a remainder, so
// that the characters sum back to the byte; characters that would land on
// excluded punctuation are stepped in pairs, one up and one down, which moves
// them without changing the sum. The final rotation by one byte is what makes
// the string start on a character boundary of the value rather than mid-byte.
func encodeChecksum(value uint32) string {
	var (
		asc   [16]byte
		masks = [4]uint32{0xff000000, 0x00ff0000, 0x0000ff00, 0x000000ff}
	)

	for i := range 4 {
		b := int((value & masks[i]) >> ((3 - i) * 8))

		quotient := b/4 + 0x30
		remainder := b % 4

		var ch [4]int
		for j := range 4 {
			ch[j] = quotient
		}

		ch[0] += remainder

		// Step off the excluded punctuation, in pairs so the sum is
		// preserved: one character up and its partner down. The pairs are
		// (0,1) and (2,3), written out rather than computed so the indices are
		// visibly inside the array.
		for again := true; again; {
			again = false

			for _, bad := range checksumExcluded {
				for _, pair := range [2][2]int{{0, 1}, {2, 3}} {
					lo, hi := pair[0], pair[1]

					if asciiByte(ch[lo]) == bad || asciiByte(ch[hi]) == bad {
						ch[lo]++
						ch[hi]--
						again = true
					}
				}
			}
		}

		for j := range 4 {
			asc[4*j+i] = asciiByte(ch[j])
		}
	}

	// Rotate right one character. This compensates for where the value sits
	// in its card: "CHECKSUM= '" is eleven characters, so the sixteen bytes
	// begin at offset 11, which is one past a four-byte word boundary. The
	// rotation is what lines the encoded bytes up with the words the sum is
	// taken over.
	var out [16]byte
	for i := range 16 {
		out[(i+1)%16] = asc[i]
	}

	return string(out[:])
}

// datasumValue renders a data checksum the way the convention writes it: a
// decimal string, quoted, because DATASUM is a character-valued keyword even
// though it holds a number.
func datasumValue(sum uint32) string {
	return quoteValue(strconv.FormatUint(uint64(sum), 10))
}

// applyChecksum fills in an HDU's DATASUM and CHECKSUM once its bytes are
// known.
//
// header is the serialised header, already padded and already carrying a
// placeholder CHECKSUM card; data is the padded payload. The CHECKSUM value is
// written into header in place, which is why the placeholder has to be exactly
// as long as the value that replaces it.
//
// Returns false when the header carries no placeholder, which is how a caller
// that did not ask for checksums gets its header back untouched.
func applyChecksum(header, data []byte) bool {
	at := bytes.Index(header, []byte("CHECKSUM= "+checksumPlaceholder))
	if at < 0 {
		return false
	}

	// The value field starts after "CHECKSUM= " and the opening quote.
	valueAt := at + len("CHECKSUM= ") + 1

	sum := CalcChecksum(header)
	sum = addChecksums(sum, CalcChecksum(data))

	// The encoded value must bring the total to all ones, so what is encoded
	// is the complement of the sum taken with the field still holding zeros.
	copy(header[valueAt:valueAt+16], encodeChecksum(^sum))

	return true
}

// addChecksums combines two 1's complement sums.
//
// Not a plain addition: 1's complement arithmetic carries the overflow back
// into the low bits, which is what makes the sum independent of where the data
// was split.
func addChecksums(a, b uint32) uint32 {
	sum := uint64(a) + uint64(b)
	for sum>>32 > 0 {
		sum = (sum & 0xFFFFFFFF) + (sum >> 32)
	}

	return uint32(sum)
}

// asciiByte narrows an encoding character to the byte it is.
//
// The values are bounded by construction — a quotient of at most 0x30+63 plus
// a remainder of at most 3, stepped by at most a few — so this cannot lose
// information. It exists because that bound is an argument rather than
// something the type system carries, and a silent truncation here would
// produce a checksum that verifies nowhere.
func asciiByte(v int) byte {
	if v < 0 || v > 0xFF {
		return 0 // unreachable: see above
	}

	return byte(v)
}
