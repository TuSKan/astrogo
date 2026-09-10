package file

import "sync"

// writeLock serialises writes to one key within this process, returning the
// function that releases it.
//
// # Why a lock exists at all
//
// fileblob writes through a staging file and renames it into place, and it
// names that file from the clock:
//
//	os.TempDir()/<basename>.<time.Now().UnixNano() in hex>.tmp
//
// with the stated reasoning that "nanosecond changes enough between each
// iteration to make a conflict unlikely". On Windows it does not change at
// all. The system clock ticks about every 15.6 ms, and measured on Windows 11,
// `time.Now().UnixNano()` returned **one distinct value across 2000
// consecutive reads**. Every concurrent writer of the same key therefore picks
// the same staging name, and the O_EXCL retry re-reads the same frozen clock,
// so it retries into the same name. One writer's staging file is renamed out
// from under another (#241).
//
// Measured on Windows 11 / Go 1.27, eight goroutines writing one key, 40
// rounds:
//
//	configuration                        failures / 320
//	separate buckets, os.TempDir                     59
//	separate buckets, no_tmp_dir=1                    0
//	shared bucket, os.TempDir                        11
//	shared bucket, no_tmp_dir=1                      69
//	any of the above, plus this lock                  0
//
// The middle two rows are why the URL parameter is not the whole fix and was
// not applied on its own: moving staging into the bucket directory removes the
// cross-talk between *different* buckets and makes writers of one key collide
// harder, because the name they now share is the key's own path. astrogo does
// both — the parameter for the cross-bucket case, which spans processes and no
// in-process lock can reach, and this for writers of one key inside one
// process.
//
// # What it does not cover
//
// Two processes writing the same key into the same bucket directory. That is
// what remote's cross-process lock object is for, and it is a separate
// mechanism with its own limits.
//
// # Shape
//
// Reference-counted rather than a bare map of mutexes, so a process that
// writes many distinct keys does not accumulate one mutex per key for the rest
// of its life. Keyed by the bucket pointer as well as the key, since two
// buckets are two directories and a name in one cannot collide with a name in
// the other once staging is inside them.
func writeLock(bucket *Bucket, key string) (unlock func()) {
	id := lockID{bucket: bucket, key: key}

	writeLocksMu.Lock()

	entry, ok := writeLocks[id]
	if !ok {
		entry = &writeLockEntry{}
		writeLocks[id] = entry
	}

	entry.waiters++

	writeLocksMu.Unlock()

	entry.mu.Lock()

	return func() {
		entry.mu.Unlock()

		writeLocksMu.Lock()
		defer writeLocksMu.Unlock()

		entry.waiters--

		if entry.waiters == 0 {
			delete(writeLocks, id)
		}
	}
}

// lockID identifies one key in one bucket.
type lockID struct {
	bucket *Bucket
	key    string
}

// writeLockEntry is one key's mutex plus the count of goroutines holding or
// waiting for it, which is what lets the entry be removed when the last one
// leaves.
type writeLockEntry struct {
	mu      sync.Mutex
	waiters int
}

var (
	writeLocksMu sync.Mutex
	writeLocks   = map[lockID]*writeLockEntry{}
)
