package leakcheck

import "errors"

// ErrProfileUnavailable indicates the runtime has no goroutine-leak profile,
// or one whose header does not say how many goroutines leaked, so nothing
// checked whether goroutines leaked.
//
// A sentinel rather than a bare message because the distinction it carries is
// the one this package is about: "no leaks found" and "nothing looked" are
// different answers, and only one of them is good news.
var ErrProfileUnavailable = errors.New("leakcheck: goroutine-leak profile unavailable")
