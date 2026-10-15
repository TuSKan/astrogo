---
type: Fixed
pr: 798
---
**`fits` refuses the header counts that crashed or hung a reader.** `TFIELDS` and `ExtractWCS`'s `NAXIS` are held to the standard's 999: `TFIELDS = -1` panicked, and a huge one asked for 8.8 TB, which ends the process. SIP coefficients are read from the header's cards, so a declared order no longer sets the work: `A_ORDER = 1000000` took about 19 hours. A SIP transform now gives the same answer on every call, where map-order summation let it differ in the last bit.
