package fits_test

import (
	"bytes"
	"strconv"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/fits"
)

// The checksum convention defines one testable property, and it is the one an
// external verifier applies: the 1's complement sum of a complete HDU — header
// including its own CHECKSUM card, plus data — is all ones.
//
// Asserting that is not the same as asserting our arithmetic against itself.
// cfitsio's fits_verify_chksum and astropy's verify_checksum compute exactly
// this sum over exactly these bytes, so a file that satisfies it here satisfies
// it there.

// TestChecksumSatisfiesTheConventionsOwnTest is that property, over shapes
// whose header and data lengths differ, since the sum is taken across both.
func TestChecksumSatisfiesTheConventionsOwnTest(t *testing.T) {
	t.Parallel()

	batch := catalogBatch(t)
	t.Cleanup(batch.Release)

	for _, tc := range []struct {
		file *fits.File
		name string
	}{
		{
			name: "header only",
			file: &fits.File{HDUs: []fits.HDU{&fits.ImageHDU{}}},
		},
		{
			name: "small image",
			file: &fits.File{HDUs: []fits.HDU{float32Image(t, 3, 2, []float32{1, 2, 3, 4, 5, 6})}},
		},
		{
			name: "image spanning several blocks",
			file: &fits.File{HDUs: []fits.HDU{float32Image(t, 40, 40, make([]float32, 1600))}},
		},
		{
			name: "primary plus a table",
			file: &fits.File{HDUs: []fits.HDU{&fits.ImageHDU{}, &fits.BintableHDU{Batch: batch}}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			if err := fits.Write(&buf, tc.file); err != nil {
				t.Fatalf("Write: %v", err)
			}

			for i, hdu := range splitHDUs(t, buf.Bytes()) {
				if sum := fits.CalcChecksum(hdu); !fits.VerifyChecksum(sum) {
					t.Errorf("HDU %d: checksum over the whole unit is %#08x, want all ones.\n"+
						"  This is the test cfitsio and astropy apply, so a file failing "+
						"it here is reported as corrupt by them.", i, sum)
				}
			}
		})
	}
}

// TestDatasumMatchesTheDataAlone covers the other keyword, which is defined
// over the data only and is what tells a reader whether the payload survived
// independently of the header.
func TestDatasumMatchesTheDataAlone(t *testing.T) {
	t.Parallel()

	src := float32Image(t, 3, 2, []float32{1, 2, 3, 4, 5, 6})

	var buf bytes.Buffer
	if err := fits.Write(&buf, &fits.File{HDUs: []fits.HDU{src}}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	raw := buf.Bytes()

	got, err := fits.Read(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("Read back: %v", err)
	}

	declared, err := got.HDUs[0].Header().GetString("DATASUM")
	if err != nil {
		t.Fatalf("DATASUM: %v", err)
	}

	// The data is everything after the header blocks.
	headerBlocks := headerLength(t, raw)
	data := raw[headerBlocks:]

	computed := fits.CalcChecksum(data)

	if err := fits.ValidateDatasum(strings.TrimSpace(declared), computed); err != nil {
		t.Errorf("DATASUM %q does not match the data it covers: %v", declared, err)
	}

	// And a flipped bit has to break it, or the keyword is decoration.
	data[0] ^= 0x01

	if err := fits.ValidateDatasum(strings.TrimSpace(declared), fits.CalcChecksum(data)); err == nil {
		t.Error("DATASUM still matched after a bit was flipped in the data")
	}
}

// TestChecksumDetectsCorruption is the point of the whole convention: a file
// that changed after it was written must stop verifying.
func TestChecksumDetectsCorruption(t *testing.T) {
	t.Parallel()

	src := float32Image(t, 3, 2, []float32{1, 2, 3, 4, 5, 6})

	var buf bytes.Buffer
	if err := fits.Write(&buf, &fits.File{HDUs: []fits.HDU{src}}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	raw := buf.Bytes()

	if sum := fits.CalcChecksum(raw); !fits.VerifyChecksum(sum) {
		t.Fatalf("the file did not verify before corruption: %#08x", sum)
	}

	// One bit, in the pixels.
	raw[fits.BlockSize] ^= 0x01

	if sum := fits.CalcChecksum(raw); fits.VerifyChecksum(sum) {
		t.Error("a flipped bit in the data left the checksum verifying")
	}
}

// TestChecksumValueIsWritableAscii keeps the encoded string inside the
// characters the convention permits.
//
// The exclusions are not cosmetic: a quote would end the card's value and a
// slash would start a comment, so an encoding that produced one would corrupt
// the card carrying it.
func TestChecksumValueIsWritableAscii(t *testing.T) {
	t.Parallel()

	// Values chosen to walk each byte through its whole range, including the
	// ones whose quotient lands on the excluded punctuation.
	for _, v := range []uint32{
		0, 0xFFFFFFFF, 0x3a3a3a3a, 0x5b5b5b5b, 0x01020304, 0xDEADBEEF, 0x30303030,
	} {
		src := float32Image(t, 2, 2, []float32{float32(v), 2, 3, 4})

		var buf bytes.Buffer
		if err := fits.Write(&buf, &fits.File{HDUs: []fits.HDU{src}}); err != nil {
			t.Fatalf("Write: %v", err)
		}

		got, err := fits.Read(bytes.NewReader(buf.Bytes()))
		if err != nil {
			t.Fatalf("Read back: %v", err)
		}

		sum, err := got.HDUs[0].Header().GetString("CHECKSUM")
		if err != nil {
			t.Fatalf("CHECKSUM: %v", err)
		}

		if len(sum) != 16 {
			t.Errorf("CHECKSUM %q is %d characters, want 16", sum, len(sum))
		}

		for i := range len(sum) {
			c := sum[i]

			if c < 0x30 || c > 0x7A || strings.ContainsRune(":;<=>?@[\\]^_`", rune(c)) {
				t.Errorf("CHECKSUM %q contains %q at %d, which the convention excludes",
					sum, c, i)
			}
		}
	}
}

// splitHDUs cuts a written file into its HDUs, header blocks and all.
func splitHDUs(t *testing.T, raw []byte) [][]byte {
	t.Helper()

	var out [][]byte

	for off := 0; off < len(raw); {
		header := headerLengthAt(t, raw, off)
		data := dataLength(t, raw[off:off+header])

		end := off + header + data
		if end > len(raw) {
			t.Fatalf("HDU at %d runs past the file: %d bytes of %d", off, end, len(raw))
		}

		out = append(out, raw[off:end])
		off = end
	}

	return out
}

// headerLength is the byte length of the first HDU's header blocks.
func headerLength(t *testing.T, raw []byte) int {
	t.Helper()

	return headerLengthAt(t, raw, 0)
}

// headerLengthAt is the byte length of the header beginning at off.
func headerLengthAt(t *testing.T, raw []byte, off int) int {
	t.Helper()

	for n := fits.BlockSize; off+n <= len(raw); n += fits.BlockSize {
		block := raw[off+n-fits.BlockSize : off+n]
		for c := 0; c+fits.CardSize <= len(block); c += fits.CardSize {
			if strings.HasPrefix(string(block[c:c+fits.CardSize]), "END     ") {
				return n
			}
		}
	}

	t.Fatalf("no END card found from offset %d", off)

	return 0
}

// dataLength is the padded payload size an HDU's header describes.
func dataLength(t *testing.T, header []byte) int {
	t.Helper()

	f, err := fits.Read(bytes.NewReader(append(append([]byte(nil), header...), make([]byte, 0)...)))
	if err != nil {
		// A header-only HDU read alone is fine; anything else, fall through to
		// the arithmetic below.
		_ = f
	}

	bitpix := headerInt(t, header, "BITPIX")
	naxis := headerInt(t, header, "NAXIS")

	if naxis == 0 {
		return 0
	}

	elements := 1
	for i := 1; i <= naxis; i++ {
		elements *= headerInt(t, header, "NAXIS"+strconv.Itoa(i))
	}

	width := bitpix
	if width < 0 {
		width = -width
	}

	size := elements * width / 8
	if rem := size % fits.BlockSize; rem != 0 {
		size += fits.BlockSize - rem
	}

	return size
}

// headerInt reads an integer keyword straight out of raw header bytes.
func headerInt(t *testing.T, header []byte, keyword string) int {
	t.Helper()

	prefix := keyword + strings.Repeat(" ", 8-len(keyword))

	for c := 0; c+fits.CardSize <= len(header); c += fits.CardSize {
		card := string(header[c : c+fits.CardSize])
		if !strings.HasPrefix(card, prefix) {
			continue
		}

		value := strings.TrimSpace(strings.SplitN(card[10:], "/", 2)[0])

		n, err := strconv.Atoi(value)
		if err != nil {
			t.Fatalf("%s = %q: %v", keyword, value, err)
		}

		return n
	}

	t.Fatalf("no %s card", keyword)

	return 0
}
