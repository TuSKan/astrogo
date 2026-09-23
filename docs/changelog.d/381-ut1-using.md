---
type: Added
pr: 381
---
`Time.UT1Using(dut1)` converts to UT1 with a UT1−UTC the caller already holds,
for code that caches Earth orientation parameters; `Time.UT1` is built on it. Adding
DUT1 to a UTC Julian Date by hand is wrong by up to a second on a day that ends
in a leap second, and only `time` knows which days those are.
