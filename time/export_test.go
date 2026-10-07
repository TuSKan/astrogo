package time

import "sync"

// ResetEOPWarning re-arms the one-time EOP-unavailable warning, so a test
// can observe whether a call emits it rather than whether an earlier test
// already spent it. Not safe alongside a parallel test that might warn.
func ResetEOPWarning() { warnEOPUnavailableOnce = sync.Once{} }
