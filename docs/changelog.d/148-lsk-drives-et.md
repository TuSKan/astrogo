---
type: Fixed
pr: 148
---
**The JPL provider computes ET from the kernel again, and now completely.**
`lsk.Reader` parses the Moyer (1981) constants it previously ignored
(`DELTA_T_A`, `K`, `EB`, `M`), so the conversion applies the full relativistic
model the LSK defines rather than a leap-second offset alone — matching the
convention Horizons uses, while keeping the scale normalisation that fixed the
69.184 s reinterpretation.
