---
type: Changed — BREAKING
pr: 593
---
**`atmosphere.CleanMountainAOD550` is 0.027, Paranal's measured median aerosol (Patat et al. 2011)**, in place of an unsourced 0.03 whose comment placed Mauna Kea inside a range its own median (0.016) falls below. Anything built on it carries 10% less aerosol; pass 0.03 explicitly to keep the old value.
