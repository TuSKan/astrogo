---
type: Added
pr: 224
---
`catalog/mpcorb` reads the Minor Planet Center's own orbital-element files —
`MPCORB.DAT` and its NEA/Distant/PHA/Unusual cuts — as a streaming
`iter.Seq2[resolve.Target, error]`, at the full published precision SBDB
rounds to three significant figures. gzip is detected from the stream, the
packed epoch is decoded through the exported `ParseEpoch`, and a Target feeds
`plan.FromCatalog` unchanged. New `remote.MPCORB` endpoint (#128).
