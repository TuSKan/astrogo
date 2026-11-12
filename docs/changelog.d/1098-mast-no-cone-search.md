---
type: Removed
pr: 1098
---
**`mast.Provider.ConeSearch` was removed, with `mast.ErrNotImplemented`, and MAST no longer claims `resolve.CapConeSearch`**:
the method could only fail, yet `catalog.NewResolver` registered MAST as a cone searcher and called it on every
resolution. For a cone search, use `gaia` or `vizier`.
