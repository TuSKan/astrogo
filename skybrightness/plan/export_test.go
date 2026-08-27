package plan

// LimitingMagnitude exposes the electrons-to-magnitude conversion.
//
// Test-only. The public path to it needs an assembled dataset.Sky, which
// needs a network and 145 MB of reference data — none of which makes the
// arithmetic more correct, and all of which would keep it from being checked
// on every commit.
var LimitingMagnitude = limitingMagnitude
