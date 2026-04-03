package fits

import "io"

// HDUType represents the standard classifications of FITS Header Data Units.
type HDUType int

const (
	HDUTypeImage   HDUType = iota // Primary or Image Extension
	HDUTypeASCII                  // TABLE
	HDUTypeBinary                 // BINTABLE
	HDUTypeUnknown                // Unknown extension type
)

// HDU (Header Data Unit) is the foundational FITS structure.
// Every HDU contains exactly one Header and an associated data payload.
type HDU interface {
	// Header returns the parsed 80-byte records forming the metadata.
	Header() *Header

	// Type identifies the underlying payload kind (Image, Binary Table, etc.).
	Type() HDUType

	// Load parses the heavy binary payload into memory using the given Reader at the stored offset.
	Load(r io.ReaderAt) error
}

type basicHDU struct {
	header      *Header
	hType       HDUType
	DataOffset  int64
	PayloadSize int64
}

func (h *basicHDU) Header() *Header          { return h.header }
func (h *basicHDU) Type() HDUType            { return h.hType }
func (h *basicHDU) Load(r io.ReaderAt) error { return nil } // Stub
