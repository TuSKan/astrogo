---
type: Changed
pr: 1086
---
**`logging`'s tests now cover every entry point**, raising the package from 71.4% to 100% statement coverage: all four
record their caller's line, `logging.InfoContext` and `logging.WarnContext` pass their context to the handler, and the
default logger keeps dropping progress lines through `With` and `WithGroup`.
