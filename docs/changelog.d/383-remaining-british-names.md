---
type: Changed — BREAKING
pr: 383
---
**Five more exported names lose their British spelling** — the ones #364 missed,
because the scan it relied on looked for `Metre` only at the start of a word:

| package | before | after |
| --- | --- | --- |
| `unit` | `Nanometre` | `Nanometer` |
| `skybrightness/dataset/crosssection` | `Nanometre` | `Nanometer` |
| `skybrightness/dataset/starlight` | `ColourTerm` | `ColorTerm` |
| `skybrightness/dataset/starlight` | `BrightStarCatalogueRadius` | `BrightStarCatalogRadius` |
| `atmosphere` | `SourceRef.Licence` | `SourceRef.License` |

The nanometer unit's printed name follows, from "nanometre" to "nanometer".
