---
type: Changed
pr: 864
---
The Benchmarks CI job runs on four runners at once, in shards that
`.github/scripts/bench-shards.sh` defines: about 17 minutes down to 5.5, with
the same samples and the same delta. CI can also be run by hand.
