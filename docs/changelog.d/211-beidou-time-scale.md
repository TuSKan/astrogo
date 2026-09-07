---
type: Added
pr: 211
---
`time.BDT`, the BeiDou system time scale, and `Time.BDT()`. TAI − 33 s exactly,
so BDT − UTC is 4 s today and BDT − GPST a permanent 14 s. Four seconds reads as
a rounding difference and is 30 km of ISS ground track, which is the argument
for a name rather than an offset the caller subtracts. GPST and BDT also join
the scale round-trip matrix, which covered neither (#145).
