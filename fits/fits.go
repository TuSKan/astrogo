package fits

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/klauspost/pgzip"
)

const (
	BlockSize = 2880
	CardSize  = 80
)

var (
	ErrInvalidBlock  = errors.New("fits: block size is not 2880 bytes")
	ErrNoPrimaryHDU  = errors.New("fits: missing primary HDU")
	ErrUnimplemented = errors.New("fits: feature not yet implemented")
)

// File represents a full FITS dataset containing multiple HDUs.
type File struct {
	HDUs []HDU
}

// Open reads a FITS file from a disk path.
// Transparently handles `.gz` and `.fits.gz` extension streams.
func Open(path string) (*File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if strings.HasSuffix(strings.ToLower(path), ".gz") {
		gzReader, err := pgzip.NewReader(f)
		if err != nil {
			return nil, err
		}
		// Notice: pgzip.Reader does not support io.Seeker.
		// The underlying fits.Read loop will gracefully fallback to streaming.
		defer gzReader.Close()
		return Read(gzReader)
	}

	return Read(f)
}

// Read processes a FITS file and parses its structure without reading data payloads.
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

		f.HDUs = append(f.HDUs, &basicHDU{header: header, hType: HDUTypeImage})

		// Calculate data payload and skip it
		size := payloadSize(header)
		if size > 0 {
			if canSeek {
				_, err = seeker.Seek(size, io.SeekCurrent)
				if err != nil {
					return nil, err
				}
			} else {
				_, err = io.CopyN(io.Discard, r, size)
				if err != nil {
					return nil, err
				}
			}
		}
	}

	if len(f.HDUs) == 0 {
		return nil, ErrNoPrimaryHDU
	}

	return f, nil
}

func payloadSize(h *Header) int64 {
	bitpix, _ := h.GetInt("BITPIX")
	naxis, _ := h.GetInt("NAXIS")

	if bitpix < 0 {
		bitpix = -bitpix
	}

	var total int64 = 1
	for i := 1; i <= naxis; i++ {
		dim, _ := h.GetInt(fmt.Sprintf("NAXIS%d", i))
		total *= int64(dim)
	}

	if naxis == 0 {
		total = 0
	}

	gcount, err := h.GetInt("GCOUNT")
	if err != nil {
		gcount = 1
	}
	pcount, err := h.GetInt("PCOUNT")
	if err != nil {
		pcount = 0
	}

	bytes := (int64(bitpix) / 8) * int64(gcount) * (int64(pcount) + total)

	remainder := bytes % int64(BlockSize)
	if remainder != 0 {
		bytes += int64(BlockSize) - remainder
	}
	return bytes
}

func ReadHeader(br *BlockReader) (*Header, error) {
	h := NewHeader()
	buf := make([]byte, BlockSize)
	maxBlocks := 10000 // 28MB max header size failsafe
	blocksRead := 0

	for {
		if blocksRead > maxBlocks {
			return nil, fmt.Errorf("fits: header exceeded %d blocks without END card (corrupt or compressed file?)", maxBlocks)
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

			// Exclude completely blank cards
			if len(c.Keyword) > 0 || len(c.Value) > 0 || len(c.Comment) > 0 {
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

// ReadBlock fills the provided buffer with exactly 2880 bytes.
func (b *BlockReader) ReadBlock(buf []byte) error {
	if len(buf) != BlockSize {
		return ErrInvalidBlock
	}
	_, err := io.ReadFull(b.r, buf)
	return err
}

// ReadBigEndian is a zero-reflection utility to read binary values from FITS arrays
func ReadBigEndian(r io.Reader, data any) error {
	return binary.Read(r, binary.BigEndian, data)
}

// Write scaffolds writing a basic HDU to a FITS file.
func Write(path string, data []float64) error {
	return ErrUnimplemented
}
