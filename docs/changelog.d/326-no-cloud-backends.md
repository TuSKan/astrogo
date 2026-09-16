---
type: Removed
pr: 326
---
**The `s3://`, `gs://`, `azblob://` and `sftp://` schemes no longer resolve.**
They were gocloud.dev drivers and went with it; replacements were built,
measured and dropped — `docs/storage.md` §10 records the two silent-wrong-answer
defects found in the obvious library and why writing one directly is not worth
it for a single optional endpoint. `remote.CopernicusEODATA` is that endpoint:
it stays registered and now fails early, naming the schemes that are registered,
and `cams.RegistrationAdvice` says so rather than naming a package that does not
exist.
