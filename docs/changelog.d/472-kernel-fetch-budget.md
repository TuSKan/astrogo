---
type: Fixed
pr: 472
---
**A slow NAIF skips `plan`'s integration tests instead of timing out the
package.** Their kernel fetches are bounded by the test binary's own deadline
and an exhausted budget is read as the upstream's failure (#471).
