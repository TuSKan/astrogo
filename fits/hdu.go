package fits

import "io"

// HDUType represents the standard classifications of FITS Header Data Units.
type HDUType int

const (
	// HDUTypeImage is a primary or image extension HDU.
	HDUTypeImage HDUType = iota // Primary or Image Extension
	// HDUTypeASCII is an ASCII table HDU.
	HDUTypeASCII // TABLE
	// HDUTypeBinary is a binary table HDU.
	HDUTypeBinary // BINTABLE
	// HDUTypeUnknown is an unknown extension type.
	HDUTypeUnknown // Unknown extension type
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

// Header returns the HDU's header, creating an empty one on first use.
//
// The lazy creation is what makes a zero-valued HDU usable for writing. An
// image built in memory — the whole point of having a writer — starts with no
// header, and a caller reaching for one to set OBJECT or a WCS would otherwise
// be handed nil and find out by panic.
func (h *basicHDU) Header() *Header {
	if h.header == nil {
		h.header = NewHeader()
	}

	return h.header
}
func (h *basicHDU) Type() HDUType            { return h.hType }
func (h *basicHDU) Load(_ io.ReaderAt) error { return nil } // Stub
