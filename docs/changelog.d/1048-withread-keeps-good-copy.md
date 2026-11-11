---
type: Fixed
pr: 1048
---
**A dataset update `remote.WithRead` refuses no longer deletes the copy that worked.** The download replaced the cached copy before it was read, so when a mutable source such as OpenNGC, VIIRS or CAMS started serving something the provider could not parse, the refusal left nothing cached and every later call failed, offline ones included. An update is now read before it replaces the cached copy.
