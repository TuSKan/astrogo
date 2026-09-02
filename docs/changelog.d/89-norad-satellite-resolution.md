---
type: Fixed
pr: 89
---
**`Resolve("ISS")` returned the wrong satellite.** CelesTrak's NAME query is a
substring match, so it gave UME (ISS) — NORAD 8709 — instead of the station,
and a bare catalog number like `25544` found nothing because it was sent as a
name. Numbers now use CATNR, and name matches are ranked.
