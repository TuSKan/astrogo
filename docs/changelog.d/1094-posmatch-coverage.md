---
type: Changed
pr: 1094
---
**`catalog/internal/posmatch`'s tests now cover every statement**, up from 73.3%: each kinematic field reaches the
propagated position on its own, and a position SOFA cannot propagate (a finite parallax of 1e300″ underflows the
distance) is reported as `coord.ErrPropagationFailed` rather than as a separation of zero.
