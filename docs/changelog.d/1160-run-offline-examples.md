---
type: Changed
pr: 1160
---
CI runs every example the README lists as needing no network, against an empty cache, and fails if one crashes or reaches the network. Before, examples were only built.
