package fits_test

import (
	"bytes"
	"testing"

	"github.com/TuSKan/astrogo/fits"
)

// This file fuzzes the FITS header parser against corrupted, truncated and
// adversarial input. Every seed is a hand-built byte literal — no checked-in
// binary fixture — so the seed corpus runs as an ordinary test under
// `go test ./...` and is part of every CI run for free. Extended fuzzing is a
// manual, periodic step; see CLAUDE.md for the invocation, and note the
// -fuzzminimizetime flag it carries, which is not optional here (#140).
//
// Property under test: parsing bytes the library did not produce must never
// panic or hang — it must return an error, or a header, and nothing else.
// Correctness on well-formed files is covered elsewhere in this package.
//
// fits is the sharpest case in the module for this, because it is the one
// place astrogo opens an arbitrary user-supplied file rather than data from a
// registered endpoint — CLAUDE.md carves fits.Open out as a deliberate os.*
// exception for exactly that reason.

// block pads s to a full 2880-byte FITS block with spaces, the way a real
// file is laid out.
func block(s string) []byte {
	buf := bytes.Repeat([]byte{' '}, fits.BlockSize)
	copy(buf, s)

	return buf
}

// fuzzCard pads a single 80-column card. Named apart from bintable_decode_test.go's
// card(keyword, value), which builds one from its two halves; this takes a whole
// line, so a seed can be deliberately malformed.
func fuzzCard(s string) string {
	if len(s) >= fits.CardSize {
		return s[:fits.CardSize]
	}

	return s + string(bytes.Repeat([]byte{' '}, fits.CardSize-len(s)))
}

func FuzzParseCard(f *testing.F) {
	f.Add([]byte(fuzzCard("SIMPLE  =                    T / conforms to FITS standard")))
	f.Add([]byte(fuzzCard("BITPIX  =                  -32")))
	f.Add([]byte(fuzzCard("OBJECT  = 'M31     '           / target")))
	f.Add([]byte(fuzzCard("COMMENT   free text with = and / inside")))
	f.Add([]byte(fuzzCard("END")))
	f.Add([]byte(fuzzCard("")))

	// A card whose value never closes its quote, and one that is all
	// separators: the two shapes a hand-written parser most often indexes
	// past the end of.
	f.Add([]byte(fuzzCard("OBJECT  = 'unterminated")))
	f.Add([]byte(fuzzCard("='/='/='/='/='/='/='/=")))

	// Short and over-long, since ParseCard takes a slice rather than a fixed
	// array and a caller can hand it either.
	f.Add([]byte("SIMPLE"))
	f.Add([]byte{})
	f.Add(bytes.Repeat([]byte{0}, 200))

	f.Fuzz(func(_ *testing.T, raw []byte) {
		// The property is that this returns, whatever it is given. A Card is
		// three strings and cannot be wrong in a way worth asserting here;
		// correctness lives in header_test.go against real cards.
		_ = fits.ParseCard(raw)
	})
}

func FuzzReadHeader(f *testing.F) {
	f.Add(block(fuzzCard("SIMPLE  =                    T") + fuzzCard("END")))
	f.Add(block(fuzzCard("END")))
	f.Add(bytes.Repeat([]byte{' '}, fits.BlockSize)) // a block with no END

	// Truncated mid-block, which is what a cut-off download looks like.
	f.Add(block(fuzzCard("SIMPLE  =                    T"))[:1000])

	// Two blocks where the END lands in the second, exercising the loop.
	f.Add(append(
		block(fuzzCard("SIMPLE  =                    T")),
		block(fuzzCard("END"))...,
	))

	f.Add([]byte{})
	f.Add(bytes.Repeat([]byte{0}, fits.BlockSize))

	f.Fuzz(func(_ *testing.T, data []byte) {
		br := fits.NewBlockReader(bytes.NewReader(data))

		h, err := fits.ReadHeader(br)
		if err != nil {
			return
		}

		// A header that parsed must also be traversable: the lookup path and
		// every card it kept. Reading them back is what any caller does next,
		// and it is where an index built during parsing would go wrong.
		for _, c := range h.Cards {
			if got, err := h.Get(c.Keyword); err == nil {
				_ = got.Value + got.Comment
			}
		}
	})
}

func FuzzRead(f *testing.F) {
	// A minimal well-formed primary HDU: no data, so NAXIS is zero.
	f.Add(append(
		block(fuzzCard("SIMPLE  =                    T")+
			fuzzCard("BITPIX  =                    8")+
			fuzzCard("NAXIS   =                    0")+
			fuzzCard("END")),
		[]byte{}...,
	))

	// The same header claiming data that is not there — the shape that makes a
	// reader trust a length field over the bytes it actually has.
	f.Add(block(fuzzCard("SIMPLE  =                    T") +
		fuzzCard("BITPIX  =                  -32") +
		fuzzCard("NAXIS   =                    2") +
		fuzzCard("NAXIS1  =                 9999") +
		fuzzCard("NAXIS2  =                 9999") +
		fuzzCard("END")))

	f.Add([]byte{})

	f.Fuzz(func(_ *testing.T, data []byte) {
		// Read walks every HDU and dispatches on the header's own type
		// keywords, so this reaches the image, ASCII-table and bintable paths
		// as the fuzzer learns to produce their XTENSION cards.
		_, _ = fits.Read(bytes.NewReader(data))
	})
}
