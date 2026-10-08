---
type: Fixed
pr: 606
---
**`catalog/gaia` and `catalog/vizier` cone searches returned an arbitrary subset of a crowded cone**, since `TOP n` came with no ordering. Now a capped result is the `n` sources nearest the center, nearest first. TOP 5 of a 5° cone around the Trapezium had returned stars 4.6° to 5° out.
