package fits

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/klauspost/pgzip"
)

const (
	// BlockSize is the size of a FITS block.
	BlockSize = 2880
	// CardSize is the size of a FITS card.
	CardSize = 80
)

var (
	// ErrInvalidBlock is returned when a block size is not 2880 bytes.
	ErrInvalidBlock = errors.New("fits: block size is not 2880 bytes")
	// ErrNoPrimaryHDU is returned when a primary HDU is not found.
	ErrNoPrimaryHDU = errors.New("fits: missing primary HDU")
)

// File represents a full FITS dataset containing multiple HDUs.
type File struct {
	HDUs []HDU
}

// Open reads a FITS file from a disk path.
// Transparently handles `.gz` and `.fits.gz` extension streams.
func Open(path string) (_ *File, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("fits: open: %w", err)
	}
	defer func() {
		cerr := f.Close()
		if cerr != nil {
			err = errors.Join(err, cerr)
		}
	}()

	if strings.HasSuffix(strings.ToLower(path), ".gz") {
		gzReader, err := pgzip.NewReader(f)
		if err != nil {
			return nil, fmt.Errorf("fits: gzip reader: %w", err)
		}
		// Notice: pgzip.Reader does not support io.Seeker.
		// The underlying fits.Read loop will gracefully fallback to streaming.
		defer func() {
			cerr := gzReader.Close()
			if cerr != nil {
				err = errors.Join(err, cerr)
			}
		}()

		return Read(gzReader)
	}

	return Read(f)
}

// Read processes a FITS file, decoding table extensions and skipping image
// payloads.
func Read(r io.Reader) (*File, error) {
	br := NewBlockReader(r)

	f := &File{
		HDUs: make([]HDU, 0),
	}

	// Try to assert seeker for fast skipping
	seeker, canSeek := r.(io.Seeker)

	for {
		header, err := ReadHeader(br)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				if len(f.HDUs) > 0 {
					break // Successfully finished reading file
				}
			}

			return nil, err
		}

		// A table extension is decoded rather than skipped. Reading a FITS
		// file for its headers alone makes every table consumer silently
		// empty: ReadBintable was unreachable from here, so a caller asking
		// a BINTABLE for a column got no rows and no error.
		if xtension, err := header.GetString("XTENSION"); err == nil {
			switch strings.TrimSpace(xtension) {
			case "BINTABLE":
				hdu, err := ReadBintable(header, br)
				if err != nil {
					return nil, fmt.Errorf("fits: read BINTABLE: %w", err)
				}

				f.HDUs = append(f.HDUs, hdu)

				continue
			case "TABLE":
				hdu, err := ReadASCIITable(header, br)
				if err != nil {
					return nil, fmt.Errorf("fits: read TABLE: %w", err)
				}

				f.HDUs = append(f.HDUs, hdu)

				continue
			}
		}

		// An image is decoded rather than skipped, for the same reason a table
		// is. Appending a basicHDU and seeking past the payload leaves a
		// caller type-asserting to *ImageHDU with no image and no error, which
		// is exactly how ReadBintable was unreachable from here before.
		//
		// It costs what the image weighs. A caller who wants headers alone
		// pays for pixels they will not read, which is the price of the
		// alternative being a silent empty result.
		if isImagePayload(header) {
			hdu, err := ReadImage(header, br)
			if err != nil {
				return nil, fmt.Errorf("fits: read image: %w", err)
			}

			f.HDUs = append(f.HDUs, hdu)

			continue
		}

		f.HDUs = append(f.HDUs, &basicHDU{header: header, hType: HDUTypeImage})

		// Calculate data payload and skip it
		size, err := payloadSize(header)
		if err != nil {
			return nil, err
		}

		if size > 0 {
			if canSeek {
				_, err = seeker.Seek(size, io.SeekCurrent)
				if err != nil {
					return nil, fmt.Errorf("fits: seek payload: %w", err)
				}
			} else {
				_, err = io.CopyN(io.Discard, r, size)
				if err != nil {
					return nil, fmt.Errorf("fits: skip payload: %w", err)
				}
			}
		}
	}

	if len(f.HDUs) == 0 {
		return nil, ErrNoPrimaryHDU
	}

	return f, nil
}

// payloadSize is the size of an HDU's data, padded to whole blocks, from the
// structural keywords: BITPIX, NAXIS and NAXISn mandatory, GCOUNT and PCOUNT
// defaulting only when absent (FITS 4.0 §4.4.1).
func payloadSize(h *Header) (int64, error) {
	bitpix, err := requiredInt(h, "BITPIX")
	if err != nil {
		return 0, err
	}

	_, elements, err := dataAxes(h)
	if err != nil {
		return 0, err
	}

	if bitpix < 0 {
		bitpix = -bitpix
	}

	gcount, err := optionalCount(h, "GCOUNT", 1)
	if err != nil {
		return 0, err
	}

	pcount, err := optionalCount(h, "PCOUNT", 0)
	if err != nil {
		return 0, err
	}

	if elements > math.MaxInt64-int64(pcount) {
		return 0, fmt.Errorf("%w: PCOUNT %d + %d elements overflows", errDataSize, pcount, elements)
	}

	bytes, err := mulSize(int64(bitpix)/8*int64(gcount), int64(pcount)+elements)
	if err != nil {
		return 0, err
	}

	remainder := bytes % int64(BlockSize)
	if remainder != 0 {
		if bytes > math.MaxInt64-int64(BlockSize) {
			return 0, fmt.Errorf("%w: %d bytes cannot be padded to a block", errDataSize, bytes)
		}

		bytes += int64(BlockSize) - remainder
	}

	return bytes, nil
}

// maxNAXIS is the most axes an HDU may have: "a non-negative integer no
// greater than 999" (FITS 4.0 §4.4.1.1).
const maxNAXIS = 999

// dataAxes reads NAXIS and NAXIS1 … NAXISn, returning the lengths in header
// order and the number of elements they describe: their product, or zero when
// NAXIS is zero.
//
// The product is checked. Unchecked, two axes of 2^32 wrapped to zero, and an
// image declaring them read as empty with no error; a product wrapping negative
// sized a buffer, and panicked (#460).
func dataAxes(h *Header) (axes []int, elements int64, err error) {
	naxis, err := requiredAxis(h, "NAXIS")
	if err != nil {
		return nil, 0, err
	}

	if naxis > maxNAXIS {
		return nil, 0, fmt.Errorf("%w: NAXIS = %d exceeds %d", errDataSize, naxis, maxNAXIS)
	}

	if naxis == 0 {
		return nil, 0, nil
	}

	axes = make([]int, naxis)
	elements = 1

	for i := range axes {
		axes[i], err = requiredAxis(h, "NAXIS"+strconv.Itoa(i+1))
		if err != nil {
			return nil, 0, err
		}

		elements, err = mulSize(elements, int64(axes[i]))
		if err != nil {
			return nil, 0, err
		}
	}

	return axes, elements, nil
}

// mulSize multiplies two non-negative sizes, refusing a product an int64
// cannot hold rather than wrapping.
func mulSize(a, b int64) (int64, error) {
	if b != 0 && a > math.MaxInt64/b {
		return 0, fmt.Errorf("%w: %d × %d overflows", errDataSize, a, b)
	}

	return a * b, nil
}

// Two failsafes bound [ReadHeader] against a file with no END card, or with an
// END card an absurd distance in.
//
// They count different things on purpose. maxHeaderBlocks bounds what is read
// and thrown away — blank padding is skipped, so a stream of it costs one
// 2880-byte buffer no matter how long it runs. maxHeaderCards bounds what is
// kept, and that is the quantity that actually grows: every non-blank card is
// retained as three strings plus an entry in the header's keyword map.
//
// Bounding only the blocks, which is what this did, bounds the cheap thing.
// 10,000 blocks is 360,000 cards, and 360,000 distinct cards — which is what
// random or hostile bytes look like — was measured at 144 MB resident from a
// 28.8 MB input, a fivefold amplification on a file astrogo did not produce.
// See #185.
const (
	// maxHeaderBlocks caps the header at 28.8 MB of input.
	maxHeaderBlocks = 10000

	// maxHeaderCards caps the header at 20,000 retained cards, which is about
	// 7 MB and 556 blocks of input.
	//
	// The number is a policy choice, so here is where it comes from: a
	// conforming primary header is a few dozen cards, and the largest thing a
	// real pipeline produces is a long HISTORY block, which this clears by
	// orders of magnitude. It is eighteen times below the ceiling the block
	// limit implies. If a legitimate file ever trips it, this constant is the
	// thing to raise — the cost is linear and about 320 bytes a card.
	maxHeaderCards = 20000
)

// ReadHeader reads a FITS header from a block reader, up to the END card.
//
// A header that reaches neither an END card nor the end of the stream within
// [maxHeaderBlocks] of input or [maxHeaderCards] of retained cards fails with
// [ErrNoEndCard], naming which of the two it exceeded. Both exist because this
// is the one package in astrogo that opens a file astrogo did not write.
func ReadHeader(br *BlockReader) (*Header, error) {
	h := NewHeader()
	buf := make([]byte, BlockSize)
	blocksRead := 0

	for {
		if blocksRead > maxHeaderBlocks {
			return nil, fmt.Errorf("%w: exceeded %d blocks", ErrNoEndCard, maxHeaderBlocks)
		}

		err := br.ReadBlock(buf)
		if err != nil {
			return nil, err
		}

		blocksRead++

		for i := 0; i < BlockSize; i += CardSize {
			rawCard := buf[i : i+CardSize]
			c := ParseCard(rawCard)

			if c.Keyword == "END" {
				return h, nil
			}

			// A CONTINUE card is not a card of its own: it carries the next
			// segment of the string the previous card began. Joining it here
			// is what makes a long value arrive whole rather than truncated
			// at the "&" that marks the split.
			if c.Keyword == continueKeyword && len(h.Cards) > 0 {
				if joinContinuation(&h.Cards[len(h.Cards)-1], c) {
					continue
				}
			}

			// Exclude completely blank cards
			if len(c.Keyword) > 0 || len(c.Value) > 0 || len(c.Comment) > 0 {
				// Checked before the append rather than after, so the limit is
				// the number of cards that can be held and not one more.
				if len(h.Cards) >= maxHeaderCards {
					return nil, fmt.Errorf("%w: exceeded %d cards", ErrNoEndCard, maxHeaderCards)
				}

				h.Append(c)
			}
		}
	}
}

// BlockReader guarantees reading exactly 2880 bytes at a time.
type BlockReader struct {
	r io.Reader
}

// NewBlockReader creates a specialized BlockReader handling 2880 byte frames.
// We DO NOT wrap in bufio anymore so the underlying io.Seeker offset stays exact.
func NewBlockReader(r io.Reader) *BlockReader {
	return &BlockReader{r: r}
}

// Read delegates to the underlying stream, so a BlockReader can be handed
// to a payload decoder that reads byte counts rather than whole blocks.
// Header cards still go through ReadBlock; the two share one position.
func (b *BlockReader) Read(p []byte) (int, error) {
	n, err := b.r.Read(p)
	if err != nil && !errors.Is(err, io.EOF) {
		return n, fmt.Errorf("fits: read: %w", err)
	}

	return n, err //nolint:wrapcheck // io.EOF must pass through unwrapped
}

// ReadBlock fills the provided buffer with exactly 2880 bytes.
func (b *BlockReader) ReadBlock(buf []byte) error {
	if len(buf) != BlockSize {
		return ErrInvalidBlock
	}

	_, err := io.ReadFull(b.r, buf)
	if err != nil {
		return fmt.Errorf("fits: read block: %w", err)
	}

	return nil
}

// ReadBigEndian is a zero-reflection utility to read binary values from FITS arrays
func ReadBigEndian(r io.Reader, data any) error {
	err := binary.Read(r, binary.BigEndian, data)
	if err != nil {
		return fmt.Errorf("fits: read big-endian: %w", err)
	}

	return nil
}

// isImagePayload reports whether an HDU carries image pixels this package can
// decode.
//
// NAXIS of zero is a header-only HDU — the usual shape of a primary header in
// a file whose data lives in extensions — and has no payload to read. An
// XTENSION this package has no reader for is left to the skip path rather than
// guessed at.
func isImagePayload(header *Header) bool {
	naxis, err := header.GetInt("NAXIS")
	if err != nil || naxis <= 0 {
		return false
	}

	xtension, err := header.GetString("XTENSION")
	if err != nil {
		return true // no XTENSION card: the primary HDU
	}

	return strings.TrimSpace(xtension) == "IMAGE"
}
