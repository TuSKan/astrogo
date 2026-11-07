package testutil

import "sync"

// Recorded holds the last value a test server's handler recorded, such as the
// query it was sent, for the test to read once its call returns.
//
// A plain variable is not enough. A request that times out is retried, and the
// handler serving the first attempt can still be running, and still writing,
// after the retry has been answered and the test has gone on to read: a data
// race, and a value that may belong to either attempt (#1012). Recorded makes
// the write and the read safe; which attempt's value is read stays the
// handler's to decide, by what it records.
//
// The zero value is ready to use.
type Recorded[T any] struct {
	mu sync.Mutex
	v  T
}

// Set records v, replacing any earlier value.
func (r *Recorded[T]) Set(v T) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.v = v
}

// Get returns the last value Set recorded, or T's zero value if none was.
func (r *Recorded[T]) Get() T {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.v
}
