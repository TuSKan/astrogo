---
type: Added
pr: 321
---
`docs/storage.md` — the design for replacing `gocloud.dev/blob` with an
astrogo-native storage layer, and the measurement behind it: importing
`astrogo/plan` links 433 packages, 107 of them gRPC, protobuf and OpenTelemetry
that `gocloud.dev/blob` pulls in unconditionally so it can emit traces nobody
consumes. `time`, `coord` and `ephemeris` link none.
