---
type: Fixed
pr: 795
---
**`atmosphere/dataset/cams` refuses a malformed file with `ErrMalformed` instead of letting the HDF5 decoder end the process.** One corrupted metadata byte made the decoder allocate 8.8 TB, a fatal error no recover can catch. Every read now checks first that the file could hold what it declares, and every call into the decoder is recovered. A corrupt local heap inside the decoder's own `Open` remains for upstream to bound.
