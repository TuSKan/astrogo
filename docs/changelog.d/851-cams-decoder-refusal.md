---
type: Fixed
pr: 851
---
**A CAMS file the HDF5 decoder refused as corrupt was not reported as
`cams.ErrMalformed`**; only a decoder panic was. scigolib/hdf5 v0.14.2, which
this release moves to, now refuses such files itself, so the reader marks
either way the same.
