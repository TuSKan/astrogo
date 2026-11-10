---
type: Fixed
pr: 1031
---
**`viirs.Open` reads a year upstream reprocessed from the new archive.** It used to download the new archive, a gigabyte, and then serve the composite extracted from the old one. The composite now sits beside its archive under `atlas/`, not at the root of the data directory, and is extracted again whenever the cached archive changes. The copy older versions left at the root is removed.
