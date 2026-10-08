---
type: Fixed
pr: 647
---
**Five functions carried another function's doc comment**, among them `plan`'s `raOfDateDifference`, which opened with `wrap180`'s, and `skybrightness/dataset/starlight`'s `parquetRows.Has`, documented as reading a column as a float. Each doc is back on its own function, and docsguard's `TestNoFuncDocOpensWithAnotherFunc` fails when a function's doc opens with the name of another function in the same file.
