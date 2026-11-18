---
type: Changed
pr: 1240
---
**`catalog/fink` and `skybrightness/dataset/starlight` read Parquet without arrow-go's `pqarrow`, so no library package links gRPC or protobuf.** `catalog/fink` reads FINK's SSOFT table from memory, with no temporary file. A malformed Gaia answer is now an error from `starlight` rather than a panic, and a file whose columns do not hold the rows it declares is refused by both.
