package jpl_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/jpl"
	"github.com/TuSKan/astrogo/time"
)

// TestBodyIDAboveInt32IsRefused covers the conversion CodeQL flagged as
// go/incorrect-integer-conversion, and the reason it is worth more than a
// bounds check.
//
// [core.ID] is uint32. A NAIF id is signed 32-bit. So the top half of the
// unsigned range is not a large id — it is not an id at all, and converting it
// does not lose information in the usual harmless way. It *wraps to a negative
// number*, and negative NAIF ids are meaningful: that is how spacecraft are
// numbered, Cassini being -82.
//
// The failure mode is therefore not a crash or a zero. core.ID(0xFFFFFFFF)
// becomes -1, which is a syntactically valid id, and the provider goes looking
// for it. A caller's typo or a bad designation from a catalogue arrives as a
// different real body, or as "no coverage" for one — an answer, not an error.
// That is the class this repository keeps finding and refusing to ship.
//
// Reported rather than clamped, because there is no id to fall back to:
// clamping to MaxInt32 would invent a body the caller never asked for.
func TestBodyIDAboveInt32IsRefused(t *testing.T) {
	t.Parallel()

	// No kernel is needed: the guard runs before any segment lookup, which is
	// itself worth pinning — an id that cannot be represented should not first
	// cost a download.
	p := &jpl.Provider{}

	epoch := time.Date(2026, time.June, 21, 0, 0, 0, 0, time.LocationUTC)

	for _, id := range []core.ID{
		math.MaxInt32 + 1, // the first value that wraps, to -2147483648
		math.MaxUint32,    // wraps to -1
		math.MaxUint32 - 81,
	} {
		_, err := p.State(id, epoch)
		if !errors.Is(err, jpl.ErrBodyIDOutOfRange) {
			t.Errorf("State(%d) returned %v, want ErrBodyIDOutOfRange.\n"+
				"  Converted rather than refused, this id becomes a negative NAIF number — "+
				"a spacecraft — and the caller gets a body they never named.", id, err)
		}
	}
}

// TestBodyIDAtTheBoundaryIsAccepted checks the guard did not move the edge.
//
// MaxInt32 is representable and must stay usable; a guard that rejected it
// would be trading one silent wrong answer for a loud wrong refusal. It fails
// later for want of a kernel, which is the right reason.
func TestBodyIDAtTheBoundaryIsAccepted(t *testing.T) {
	t.Parallel()

	p := &jpl.Provider{}
	epoch := time.Date(2026, time.June, 21, 0, 0, 0, 0, time.LocationUTC)

	_, err := p.State(core.ID(math.MaxInt32), epoch)
	if errors.Is(err, jpl.ErrBodyIDOutOfRange) {
		t.Error("State(MaxInt32) was refused as out of range.\n" +
			"  It is the largest id that survives the conversion intact and has to " +
			"remain askable.")
	}
}
