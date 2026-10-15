---
type: Fixed
pr: 770
---
**`fits.BintableHDU.GetFloatColumn` and `GetStringColumn` read 16-bit integer and byte columns.** Neither handled TFORM `I` or `B`, so every row came back NaN or blank with no error, and `catalog/fits` loaded a catalogue of 16-bit integer IDs with every ID empty. A logical column now reads as `T`/`F`, and a column a getter cannot convert, a vector or a logical asked for numbers, is the new `fits.ErrColumnType`.
