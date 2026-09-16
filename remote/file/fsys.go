package file

import (
	"context"
	"io"
	"io/fs"
)

// File is an open object.
//
// [fs.File] supplies Read, Stat and Close. The two additions are what a caller
// needs to reach into the middle of a three-gigabyte SPK kernel without reading
// the 2.9 GB in front of it: astrogo's whole random-access story is ReaderAt,
// and Seeker is what lets an io.Reader-shaped decoder work over a remote object
// without buffering it.
//
// Every backend in this package returns a File. A backend that cannot seek —
// there is no such thing here, since even the HTTP one ranges — would have to
// say so by not implementing the interface, which is a compile-time answer
// rather than a runtime surprise.
type File interface {
	fs.File
	io.ReaderAt
	io.Seeker
}

// The optional interfaces a filesystem may implement beyond [io/fs].
//
// # Why astrogo defines these at all
//
// This package's doc comment used to say, correctly, that "io/fs has no write
// contract by design (golang/go#45757), and os.Root settled on the same 'read
// via fs.FS, write via a concrete type' shape; this package does not invent
// one." That was right while writes went through *blob.Bucket's own concrete
// methods.
//
// It stops being available once the concrete type goes. astrogo picks a backend
// by URL scheme at run time, so the thing the registry returns has to be
// writable polymorphically or not at all — and "not at all" would mean the
// cache could not be written, which is the entire point of the layer.
//
// So these are defined, and the shape of them is the standard library's own
// idiom rather than an invention: small, single-method, discovered by type
// assertion, exactly as [fs.StatFS], [fs.ReadDirFS] and [fs.SubFS] are. They
// are local to astrogo and make no claim to be the general answer #45757 is
// still looking for. github.com/wolfeidau/s3iofs reaches the same two
// conclusions independently, with the same names for two of them.
type (
	// CreateFS is a filesystem that can be written to, one stream at a time.
	//
	// Streaming, and not the WriteFile([]byte) that s3iofs offers: a DE440
	// kernel is three gigabytes, and astrogo's download path exists precisely
	// so that one never has to fit in memory. WithValidate runs against the
	// staged object rather than a buffer for the same reason.
	CreateFS interface {
		fs.FS
		Create(name string) (io.WriteCloser, error)
	}

	// RemoveFS is a filesystem that can delete. The signature matches
	// s3iofs's deliberately, so an implementation written for one satisfies
	// the other.
	RemoveFS interface {
		fs.FS
		Remove(name string) error
	}

	// ContextFS is a filesystem whose operations can be cancelled.
	//
	// # The gap this closes, and why it is not optional in practice
	//
	// fs.FS.Open takes no context. For a local disk that is correct and
	// unremarkable. For a three-gigabyte download over somebody else's network
	// it means a caller who gives up cannot say so, and astrogo's entire API
	// takes ctx first specifically so that they can.
	//
	// The gap is not theoretical. s3iofs, the reference implementation for the
	// S3 backend, calls context.TODO or context.Background in nine places, so
	// as published every request it makes runs to completion or to the
	// transport's own timeout, whichever comes first.
	//
	// The answer is that the filesystem value carries the context and is
	// constructed per operation. WithContext returns a filesystem equivalent to
	// the receiver in every respect except that its operations honour ctx; the
	// receiver is unchanged, so one registered filesystem serves any number of
	// concurrent callers with different deadlines.
	//
	// A backend serving a Downloadable endpoint must implement this — see
	// [RequireContext] — because a multi-gigabyte fetch that cannot be
	// cancelled is not a thing to discover at run time.
	ContextFS interface {
		fs.FS
		WithContext(ctx context.Context) fs.FS
	}
)

// WithContext returns fsys bound to ctx when it can be, and fsys unchanged when
// it cannot.
//
// The unchanged case is deliberate rather than an error: a local filesystem has
// nothing to cancel, and requiring every backend to implement ContextFS would
// mean writing a no-op method on the one kind that genuinely does not need it.
// Where the distinction matters — a download that may run for minutes — the
// check is [RequireContext], which is explicit about it.
func WithContext(ctx context.Context, fsys fs.FS) fs.FS {
	if cfs, ok := fsys.(ContextFS); ok {
		return cfs.WithContext(ctx)
	}

	return fsys
}

// RequireContext returns fsys bound to ctx, or an error if fsys cannot be.
//
// Used on the paths that may transfer for minutes — a kernel download, a
// catalogue fetch — where "the caller pressed ctrl-C and nothing happened" is a
// defect rather than an inconvenience.
func RequireContext(ctx context.Context, fsys fs.FS) (fs.FS, error) {
	cfs, ok := fsys.(ContextFS)
	if !ok {
		return nil, &fs.PathError{
			Op:   "withcontext",
			Path: "/",
			Err:  ErrNoContext,
		}
	}

	return cfs.WithContext(ctx), nil
}
