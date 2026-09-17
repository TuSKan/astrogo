---
type: Added
pr: 347
---
`remote.ErrNotServingData` lets a caller tell "the archive is down" from "the
archive sent nonsense". Archives serve their own failures — a maintenance
notice, a load shedder, a login wall — as an HTML page with a **200**, so
nothing before the parser can tell, and both cases used to arrive as an opaque
error string. They want opposite handling: a web page should be retried later, a
malformed payload should not be retried at all, and without the distinction the
honest options were to retry everything or to retry nothing. `catalog/gaia`,
`catalog/simbad`, `catalog/vizier` and `skybrightness/dataset/starlight` now
report it, and `remote.LooksLikeHTML` is the shared leading-bytes check they use
— deliberately not a content-type sniffer, since a VOTable is XML too and
telling those apart is the whole point. The sentinel lives in `remote` rather
than beside a parser because the condition is a statement about what the
endpoint did, not about the payload, and because `skybrightness` cannot import
`catalog` without inverting the layering — a sentinel on `catalog/resolve` would
have served the providers and forced the dataset tier to invent a second name
for the same thing. Detection stays with the parsers, which are the only code
that knows what the payload should have looked like. Closes [#300].
