---
type: Fixed
pr: 466
---
**`fits.Read` allocates the data that arrives, not the size a header claims.**
A 2880-byte file declaring a 400 MB image allocated all of it before reaching
EOF; it now costs what it holds and ends in `io.ErrUnexpectedEOF` (#461).
