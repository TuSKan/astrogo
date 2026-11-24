package plan

import (
	"fmt"

	"github.com/TuSKan/astrogo/unit"
)

// checkFinite refuses a duration that is not a number of seconds, saying
// which one it is.
//
// plan's durations are unit.Duration, a float64, so a step, a block's length
// or a setup time can be NaN or infinite where a time.Duration could not. A
// comparison with zero lets NaN through, and a NaN step turns a sampling loop
// into one sample at a NaN instant.
func checkFinite(what string, d unit.Duration) error {
	if finite(float64(d)) {
		return nil
	}

	return fmt.Errorf("%w: %s is %v s", ErrNonFiniteDuration, what, float64(d))
}

// checkBlocks refuses a block whose length or cadence interval is not a
// number of seconds, before any strategy places it.
func checkBlocks(blocks []*Block) error {
	for _, b := range blocks {
		if b == nil {
			continue
		}

		if err := checkFinite(fmt.Sprintf("block %q's Duration", b.ID), b.Duration); err != nil {
			return err
		}

		if b.Cadence != nil {
			if err := checkFinite(fmt.Sprintf("block %q's Cadence.MinInterval", b.ID), b.Cadence.MinInterval); err != nil {
				return err
			}
		}
	}

	return nil
}
