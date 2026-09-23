---
type: Fixed
pr: 397
---
**49 test loops that moved on from any error now say which error they expect,
or fail.** A `continue` past whatever an iteration returned could not fail on
it, and passed outright when every iteration errored: an airmass audit passed a
function that errored on every input, and the NASA and AstroPixels comparisons
logged solver errors as skips. `TestLoopsDoNotPassOverAnUnidentifiedError`
enforces it, with no exemption list (#393).
