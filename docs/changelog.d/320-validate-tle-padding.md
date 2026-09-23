---
type: Changed
pr: 320
---
`satellite.ValidateTLE` accepts trailing whitespace and anything appended past
column 69, where it used to require exactly 69 characters. Feeds emit CRLF and
padding constantly and neither loses data; a line *shorter* than 69 does, and is
still refused. The old behavior was a side effect of a length equality check
rather than a decision. [#310]
