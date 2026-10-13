---
type: Fixed
pr: 747
---
**Three downloaded-file readers no longer crash on malformed input.** `catalog/mpcorb` panicked on a comet row ending before its name, and `starlight.Load` panicked on a band header with invalid UTF-8 and tried to allocate 200 TB for one row with a huge pixel index. Seventeen parser entry points across fifteen packages are now fuzzed, which is how all three were found.
