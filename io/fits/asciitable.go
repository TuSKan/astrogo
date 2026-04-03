package fits

import (
	"fmt"
	"io"
)

// AsciiTableHDU represents a FITS ASCII Table extension (TABLE).
// ASCII Tables use string formatting natively.
type AsciiTableHDU struct {
	basicHDU
	Rows    int // NAXIS2
	Cols    int // TFIELDS
	RowSize int // NAXIS1
}

// ReadAsciiTable scaffolds the ingestion of ASCII TABLE payloads.
// Because it is lower priority than BINTABLE, it currently returns standard unloaded frames.
func ReadAsciiTable(h *Header, r io.Reader) (*AsciiTableHDU, error) {
	tfields, err := h.GetInt("TFIELDS")
	if err != nil {
		return nil, fmt.Errorf("missing TFIELDS: %w", err)
	}

	rows, _ := h.GetInt("NAXIS2")
	rowSize, _ := h.GetInt("NAXIS1")

	hdu := &AsciiTableHDU{
		basicHDU: basicHDU{header: h, hType: HDUTypeASCII},
		Rows:     rows,
		Cols:     tfields,
		RowSize:  rowSize,
	}

	if rows == 0 || tfields == 0 {
		return hdu, nil
	}

	// ASCII Tables don't have PCOUNT, but standard rules apply for trailing skips.
	totalPayloadBytes := int64(rowSize) * int64(rows)

	// In the P1 prototype, we assume it's read lazily or skipped over by fits.go
	// However, if we're forced to consume it here:
	skipBuf := make([]byte, totalPayloadBytes)
	if _, err := io.ReadFull(r, skipBuf); err != nil {
		return nil, fmt.Errorf("fits: failed reading asciitable payload: %w", err)
	}

	// Consume padding
	paddingBytes := int(totalPayloadBytes) % BlockSize
	if paddingBytes != 0 {
		padLen := BlockSize - paddingBytes
		padBuf := make([]byte, padLen)
		if _, err := io.ReadFull(r, padBuf); err != nil {
			return nil, fmt.Errorf("fits: failed reading asciitable padding: %w", err)
		}
	}

	return hdu, nil
}
