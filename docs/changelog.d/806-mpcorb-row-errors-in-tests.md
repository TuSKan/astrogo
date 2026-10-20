---
type: Fixed
pr: 806
---
Two `catalog/mpcorb` tests counted rows with a bare `for range` over `Read`'s `iter.Seq2`, discarding each row's error, so a row that failed to parse still passed as read. Both now fail on it. CI lints with golangci-lint v2.15, whose `errcheck` reports this pattern.
