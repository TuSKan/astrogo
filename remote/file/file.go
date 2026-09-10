// Package file is astrogo's file-access layer: one type, Bucket, uniform
// across every storage backend, backed by gocloud.dev/blob.
//
// Addressing is entirely a bucket URL plus a key. Nothing here is assumed
// local — the data directory, a cache, and every source may live on S3,
// SFTP, GCS, or anywhere a driver exists — so keys are always
// "/"-separated (use path.Join, never filepath.Join), and no API takes an
// OS filesystem path. LocalURL/OSPath are the transitional exception; see
// localurl.go.
//
// Scheme dispatch is gocloud's own: blob.OpenBucket resolves a URL through
// a registry that each driver package populates from its init(). This
// package blank-imports the two drivers every astrogo build needs —
// fileblob for file://, httpblob for http:// and https:// — and nothing
// else. Any further scheme is an opt-in blank import of its own subpackage
// (remote/s3 for s3://), which is why adding one needs no change here.
//
// Bucket is *blob.Bucket, which already implements fs.FS and fs.SubFS, so
// reads go through the standard library's own optional-interface set
// (Open/ReadFile/Stat/Glob/Sub) and writes through Bucket's native methods
// (NewWriter/WriteAll/Delete/Copy). io/fs has no write contract by design
// (golang/go#45757), and os.Root settled on the same "read via fs.FS,
// write via a concrete type" shape; this package does not invent one.
package file

import (
	"context"
	"fmt"
	"io"
	"path"
	"sync"

	"gocloud.dev/blob"
	_ "gocloud.dev/blob/fileblob" // file://

	_ "github.com/TuSKan/gocloud-ext/blob/httpblob" // http://, https://
)

var (
	bucketsMu sync.Mutex
	buckets   = map[string]*Bucket{}

	stagingMu    sync.Mutex
	stagingLocks = map[string]*sync.Mutex{}
)

// LockStaging serialises writers whose staging files would collide, returning
// the function that releases it.
//
//	defer file.LockStaging(key)()
//
// # The collision
//
// fileblob does not write an object in place. It stages the bytes in a temp
// file and renames that over the key on Close, and it names the temp file
//
//	os.TempDir()/<basename of key>.<time.Now().UnixNano() in hex>.tmp
//
// on the stated reasoning that "nanosecond changes enough between each
// iteration to make a conflict unlikely". On Windows it does not change at
// all: the clock granularity is about 15.6 ms, and 2000 consecutive
// UnixNano reads on this machine returned **one** distinct value. Two writers
// therefore agree on the staging path, and the O_EXCL retry loop re-reads the
// same frozen clock and retries into the same name. One of them renames the
// file out from under the other, which surfaces as a bare "The system cannot
// find the file specified" against a key that was never touched.
//
// # Why the lock is keyed on the basename
//
// Because the staging path is. It contains no part of the bucket, so two
// writers of the same file name collide even when their buckets are different
// directories — which is what made this reproduce in parallel tests that each
// had their own [testing.T.TempDir]. Keying on the full key, or on
// bucket-and-key, would leave that case failing.
//
// Measured on Windows 11 / Go 1.27, 8 writers x 40 rounds of the same key:
//
//	                                     failures / 320
//	separate buckets, as today                 31-69
//	shared bucket, as today                    36-38
//	shared bucket, ?no_tmp_dir=1               53-76   <- worse
//	either, with this lock                         0
//
// The bucket-URL knob is in that table because it is the obvious fix and it is
// the wrong one: it moves staging into the bucket directory, which does fix
// separate buckets and makes the shared-bucket case — astrogo's actual cache —
// measurably worse. This lock fixes both and changes no URL.
//
// # What it does not cover
//
// Another process. The staging path is machine-wide, so two astrogo processes
// writing one file name still race, and what keeps them apart is the
// cross-process download lock in the parent package, whose own exclusivity is
// #245. This is the in-process half.
func LockStaging(key string) func() {
	base := path.Base(key)

	stagingMu.Lock()

	mu, ok := stagingLocks[base]
	if !ok {
		mu = &sync.Mutex{}
		stagingLocks[base] = mu
	}

	stagingMu.Unlock()

	mu.Lock()

	return mu.Unlock
}

// Bucket is *blob.Bucket, re-exported under astrogo's own name so consumer
// packages never import gocloud.dev/blob directly.
type Bucket = blob.Bucket

// Open resolves a gocloud.dev/blob bucket URL — "file:///var/cache/astrogo",
// "https://naif.jpl.nasa.gov/pub/naif/generic_kernels/spk/", "s3://eodata?region=..."
// — to a Bucket. The URL string is passed through to blob.OpenBucket
// verbatim; an unregistered scheme surfaces as that call's own error.
//
// One Bucket is opened per distinct URL and reused for the life of the
// process. *blob.Bucket is safe for concurrent use, so sharing is free, and
// one handle per URL avoids reopening a bucket on every fetch. Buckets are
// never closed, matching their process-lifetime role as astrogo's cache and
// source handles.
//
// This used to claim more: that sharing "is what makes fileblob's
// IfNotExist precondition meaningful within a process", because "that
// driver guards it with a per-Bucket mutex". It does not. In the pinned
// driver fileblob's bucket struct holds no mutex, and the one that exists is
// built per writer inside NewTypedWriter, so contenders never exclude each
// other however many Buckets they share. Measured, 8 goroutines on one
// Bucket over 200 rounds produced 51 rounds with two or more simultaneous
// lock holders. remote.acquireLock now owns that exclusion itself (#245).
func Open(ctx context.Context, bucketURL string) (*Bucket, error) {
	bucketsMu.Lock()
	defer bucketsMu.Unlock()

	if b, ok := buckets[bucketURL]; ok {
		return b, nil
	}

	b, err := blob.OpenBucket(ctx, bucketURL)
	if err != nil {
		return nil, fmt.Errorf("remote/file: open %s: %w", bucketURL, err)
	}

	buckets[bucketURL] = b

	return b, nil
}

// Save streams r into bucket at key.
//
// On a copy failure Close is deliberately not called. driver.Writer is
// only an io.WriteCloser: Close does not check whether an earlier Write
// failed, and fileblob's Close commits by renaming its temp file over key
// regardless — so closing after a failed copy replaces a good object with
// a truncated one. Skipping Close leaves key exactly as it was, at the
// cost of leaking the writer's temp resource on that rare path.
func Save(ctx context.Context, bucket *Bucket, key string, r io.Reader) error {
	defer LockStaging(key)()

	w, err := bucket.NewWriter(ctx, key, nil)
	if err != nil {
		return fmt.Errorf("remote/file: open writer %s: %w", key, err)
	}

	if _, err := io.Copy(w, r); err != nil {
		return fmt.Errorf("remote/file: write %s: %w", key, err)
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("remote/file: close %s: %w", key, err)
	}

	return nil
}
