---
type: Added
pr: 193
---
Tests for a **negative** leap second — permitted since 1972, never yet observed, and
projected for about 2030. They cover ΔAT stepping *down* through both the TAI and TT
branches, UTC↔TAI staying inverse across it, and that widening the record check from
"+1" to "|step| = 1" did not widen it into nothing (#147).
