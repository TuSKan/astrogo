---
type: Fixed
pr: 972
---
**Five tests no longer leave remote's data directory pointed at a deleted temporary directory.** Tests in `skybrightness/dataset` and `dataset/airglow` repointed the cache and never restored it, so the next cache write by any later test in the binary failed. docsguard's guard caught only a cleanup through `remote.Reset`. Its replacement fails on any test that sets the data directory without capturing it first or restoring the default.
