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
	"sync"

	"gocloud.dev/blob"
	_ "gocloud.dev/blob/fileblob" // file://

	_ "github.com/TuSKan/gocloud-ext/blob/httpblob" // http://, https://
)

//nolint:gochecknoglobals // process-wide open-bucket cache; see Open
var (
	bucketsMu sync.Mutex
	buckets   = map[string]*Bucket{}
)

// Bucket is *blob.Bucket, re-exported under astrogo's own name so consumer
// packages never import gocloud.dev/blob directly.
type Bucket = blob.Bucket

// Open resolves a gocloud.dev/blob bucket URL — "file:///var/cache/astrogo",
// "https://naif.jpl.nasa.gov/pub/naif/generic_kernels/spk/", "s3://eodata?region=..."
// — to a Bucket. The URL string is passed through to blob.OpenBucket
// verbatim; an unregistered scheme surfaces as that call's own error.
//
// One Bucket is opened per distinct URL and reused for the life of the
// process. *blob.Bucket is safe for concurrent use, so sharing is free,
// and reuse is what makes fileblob's IfNotExist precondition meaningful
// within a process: that driver guards it with a per-Bucket mutex, so
// separate Bucket values over the same directory would not exclude each
// other. Buckets are never closed, matching their process-lifetime role as
// astrogo's cache and source handles.
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
