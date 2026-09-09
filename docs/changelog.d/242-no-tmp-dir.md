---
type: Fixed
pr: 242
---
Cache writes are staged beside the object rather than in the shared
`os.TempDir()`, so two goroutines caching the same kernel name no longer
collide on one staging path. Measured on Windows: 49 failures in 320
concurrent same-key writes before, 0 after (#242).
