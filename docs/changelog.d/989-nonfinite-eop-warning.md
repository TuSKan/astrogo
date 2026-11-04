---
type: Fixed
pr: 989
---
**A non-finite time no longer spends the one-time "EOP unavailable" warning.** Since `lookupEOP` refuses a non-finite epoch, `Time.EOP` and the UT1↔UTC conversion reported that refusal through the warning that fires once per process, though no data was missing. A later, real degradation then went unreported. A non-finite time now gets no notice, and its answer already carries a NaN `DUT1`.
