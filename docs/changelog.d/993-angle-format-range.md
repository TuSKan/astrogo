---
type: Fixed
pr: 993
---
**`Angle.DMSString` and `Angle.HMSString` print every angle at every precision.** A NaN or infinite angle printed MinInt64 digits on amd64 and a zero-looking string on arm64, so a NaN coordinate's `String` did too. It now formats as `String` does. `DMSString`'s degrees overflowed from 2⁶³°. And from precision 15 the seconds drifted in their last digits, from 307 they carried a spurious minute, and from 309 they printed `NaN`. Below those bounds every angle prints what it did before.
