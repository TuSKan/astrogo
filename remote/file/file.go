// Package file is astrogo's file-access layer: one type, Bucket, uniform
// across every storage backend, backed by gocloud.dev/blob.
//
// It is internal to remote and is not importable from outside remote/ —
// TestSubpackagesAreNotImportedDirectly enforces that. Everything here is
// re-exported from remote, which is where a caller reaches it, and which is
// also the only place that answers whether bytes may move at all. What is
// left here is moving them.
//
// Addressing is entirely a bucket URL plus a key. Nothing here is assumed
// local — the data directory, a cache, and every source may live on S3,
// SFTP, GCS, Azure, or anywhere a driver exists — so keys are always
// "/"-separated (use path.Join, never filepath.Join), and no API here takes
// an OS filesystem path at all. The module's one path-to-URL conversion is
// remote's own default-cache-dir resolver, which has to start from
// os.UserCacheDir and is deliberately not general.
//
// Scheme dispatch is gocloud's own: blob.OpenBucket resolves a URL through
// a registry that each driver package populates from its init(). This
// package blank-imports the two drivers every astrogo build needs —
// fileblob for file://, httpblob for http:// and https:// — and nothing
// else. Any further scheme is an opt-in blank import of its own subpackage
// (remote/file/s3 for s3://, and gcs, azure and sftp beside it), which is why
// adding one needs no change here.
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
	"sync"

	"gocloud.dev/blob"
	_ "gocloud.dev/blob/fileblob" // file://
	"gocloud.dev/gcerrors"

	_ "github.com/TuSKan/gocloud-ext/blob/httpblob" // http://, https://
)

var (
	bucketsMu sync.Mutex
	buckets   = map[string]*Bucket{}
)

// Bucket is *blob.Bucket, re-exported under astrogo's own name so consumer
// packages never import gocloud.dev/blob directly.
type Bucket = blob.Bucket

// Open resolves a bucket URL — "file:///var/cache/astrogo",
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
	// Serialised against any other writer of this key in this process, because
	// fileblob's staging file is named from a clock that does not advance on
	// Windows. See [WriteLock].
	defer writeLock(bucket, key)()

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

// IsNotFound reports whether err says the object does not exist.
//
// It exists so a caller can distinguish "not cached yet" from "the store is
// broken" without importing gocloud.dev/gcerrors — the one distinction every
// cache-miss path in the module has to make, and the only reason any package
// outside this one ever reached for the driver's error package. A miss handled
// as a failure turns a first run into an error; a failure handled as a miss
// re-downloads a kernel every time and reports nothing.
func IsNotFound(err error) bool {
	return gcerrors.Code(err) == gcerrors.NotFound
}

// SavePartial writes r to key with the source ETag a later [ResumePoint] reads
// back, so an interrupted transfer can be continued rather than restarted.
//
// It is the one write in this package that carries metadata, which is why it
// is not a parameter on [Save]: a caller either is recording resume state or
// is not, and every other write would have to pass a nil it never uses.
func SavePartial(ctx context.Context, bucket *Bucket, key string, r io.Reader, sourceETag string) error {
	unlock := writeLock(bucket, key)
	defer unlock()

	body, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("remote/file: read body for %s: %w", key, err)
	}

	var opts *blob.WriterOptions
	if sourceETag != "" {
		opts = &blob.WriterOptions{Metadata: map[string]string{SourceETagKey: sourceETag}}
	}

	if err := bucket.WriteAll(ctx, key, body, opts); err != nil {
		return fmt.Errorf("remote/file: write %s: %w", key, err)
	}

	return nil
}
