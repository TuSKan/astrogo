package fits

import (
	"bytes"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"testing"
)

// distinctCards builds n well-formed cards with distinct keywords, padded to a
// whole number of blocks. Distinct keywords are the point: they are what random
// or hostile bytes look like to ParseCard, and what makes every card retained
// rather than skipped.
func distinctCards(t *testing.T, n int, trailer string) []byte {
	t.Helper()

	var b strings.Builder

	for i := range n {
		card := fmt.Sprintf("K%07d= %20d / filler", i, i)
		b.WriteString(card + strings.Repeat(" ", CardSize-len(card)))
	}

	b.WriteString(trailer + strings.Repeat(" ", CardSize-len(trailer)))

	out := b.String()
	if pad := len(out) % BlockSize; pad != 0 {
		out += strings.Repeat(" ", BlockSize-pad)
	}

	return []byte(out)
}

// TestReadHeaderAcceptsTheCardLimit: the limit is a ceiling, not a target. A
// header holding exactly maxHeaderCards must still parse, or the constant means
// one less than it says.
func TestReadHeaderAcceptsTheCardLimit(t *testing.T) {
	data := distinctCards(t, maxHeaderCards, "END")

	h, err := ReadHeader(NewBlockReader(bytes.NewReader(data)))
	if err != nil {
		t.Fatalf("a header of exactly %d cards was rejected: %v", maxHeaderCards, err)
	}

	if len(h.Cards) != maxHeaderCards {
		t.Errorf("kept %d cards, want %d", len(h.Cards), maxHeaderCards)
	}
}

// TestReadHeaderRejectsOneCardTooMany is the other side of the boundary.
func TestReadHeaderRejectsOneCardTooMany(t *testing.T) {
	data := distinctCards(t, maxHeaderCards+1, "END")

	_, err := ReadHeader(NewBlockReader(bytes.NewReader(data)))
	if !errors.Is(err, ErrNoEndCard) {
		t.Fatalf("err = %v, want ErrNoEndCard", err)
	}

	// The message has to name the limit that tripped, since the two failsafes
	// mean different things: too many cards is a hostile or corrupt file, too
	// many blocks is one with no END at all.
	if !strings.Contains(err.Error(), "cards") {
		t.Errorf("the error does not say which failsafe tripped: %v", err)
	}
}

// TestBlankPaddingDoesNotCountAsCards keeps the two failsafes honest about
// counting different things. Blank cards are skipped rather than retained, so a
// long run of padding must be bounded by the block limit and not by the card
// limit — a header legitimately padded to a block boundary is ordinary, and
// charging it against the card budget would reject valid files.
func TestBlankPaddingDoesNotCountAsCards(t *testing.T) {
	// Far more blank cards than maxHeaderCards, then a real header.
	blanks := (maxHeaderCards + 1000) * CardSize

	var b strings.Builder

	b.WriteString(strings.Repeat(" ", blanks))
	b.WriteString("END" + strings.Repeat(" ", CardSize-3))

	out := b.String()
	if pad := len(out) % BlockSize; pad != 0 {
		out += strings.Repeat(" ", BlockSize-pad)
	}

	h, err := ReadHeader(NewBlockReader(bytes.NewReader([]byte(out))))
	if err != nil {
		t.Fatalf("blank padding was charged against the card limit: %v", err)
	}

	if len(h.Cards) != 0 {
		t.Errorf("kept %d cards from blank padding, want 0", len(h.Cards))
	}
}

// TestReadHeaderAllocationDoesNotScaleWithInput is the property #185 is
// actually about. Before the card bound, feeding ten times the input allocated
// ten times the memory — 144 MB from a 28.8 MB file. The card limit is reached
// at 556 blocks, so past that point the cost is flat and the caller's file size
// stops choosing astrogo's memory use.
func TestReadHeaderAllocationDoesNotScaleWithInput(t *testing.T) {
	alloc := func(blocks int) float64 {
		data := distinctCards(t, blocks*(BlockSize/CardSize), "END")

		var m0, m1 runtime.MemStats

		runtime.GC()
		runtime.ReadMemStats(&m0)

		_, _ = ReadHeader(NewBlockReader(bytes.NewReader(data)))

		runtime.ReadMemStats(&m1)

		return float64(m1.TotalAlloc-m0.TotalAlloc) / 1e6
	}

	small := alloc(1000)  // 2.9 MB of input
	large := alloc(10000) // 28.8 MB of input

	// Ten times the input, and the generous factor is for the read buffer and
	// the cards up to the limit — not for a tenfold rise. Measured on the
	// unbounded version: 14.9 MB and 144.1 MB, a ratio of 9.7.
	if large > 2*small {
		t.Errorf("allocation still scales with input: %.1f MB for 1000 blocks, %.1f MB for 10000 (ratio %.1f)",
			small, large, large/small)
	}

	// An absolute bound too, so a future change that made both large would
	// still be caught by something.
	if large > 32 {
		t.Errorf("ReadHeader allocated %.1f MB for a 28.8 MB hostile header; the card limit should hold it near 8 MB", large)
	}
}

// blankStream is an endless source of blank cards — the corrupt file the block
// failsafe exists for, where nothing is ever retained and nothing ever says
// END. A generator rather than 28.8 MB of materialised spaces, since the point
// is that the stream has no end for ReadHeader to reach.
type blankStream struct{}

func (blankStream) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = ' '
	}

	return len(p), nil
}

// TestBlockFailsafeStopsAnEndlessStream is the other failsafe, and the case
// the card limit cannot cover: blank cards are skipped, so this stream retains
// nothing and would otherwise be read for ever.
func TestBlockFailsafeStopsAnEndlessStream(t *testing.T) {
	_, err := ReadHeader(NewBlockReader(blankStream{}))
	if !errors.Is(err, ErrNoEndCard) {
		t.Fatalf("err = %v, want ErrNoEndCard", err)
	}

	if !strings.Contains(err.Error(), "blocks") {
		t.Errorf("an endless blank stream should trip the block failsafe, not the card one: %v", err)
	}
}
