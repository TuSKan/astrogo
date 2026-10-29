---
type: Fixed
pr: 912
---
**`skybrightness.Estimate.TotalUncertainty` reports an unknown uncertainty as unknown, not 0.** No component fills the uncertainty
budget yet, so every estimate used to report an uncertainty of 0, a perfectly certain prediction. It now returns the new
`ErrUncertaintyUnknown`, naming each component that gives light without an entry.
