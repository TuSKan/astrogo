---
type: Fixed
pr: 1072
---
**`skybrightness` gives the same answer to the same question, to the bit.** At `Reference` fidelity the total
spectrum, and at any fidelity `Estimate.Composition` and `UncertaintyBudget.Dominant`, summed components in Go's
randomized map order, so repeated calls differed in the last bit. `Estimate.ComponentIDs` now returns the IDs sorted.
