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

	// CreateExclFS is a filesystem that can create a name only when nothing is
	// there, as one indivisible step, reporting [fs.ErrExist] when something
	// is. It is the primitive astrogo's cross-process download lock is built
	// on, and the reason it is a separate interface from [CreateFS] is that
	// exclusivity is a real capability a backend either has or has not.
	//
	// # This is stronger than what it replaces
	//
	// The lock used to be gocloud's WriterOptions.IfNotExist, described in
	// astrogo's own code as "on S3 a genuinely atomic conditional PUT, on
	// fileblob a Stat-then-Rename that is best-effort and can admit a second
	// holder". The residual race was tracked as #241 and papered over by a
	// double-check after acquiring.
	//
	// There is no need to concede it. O_CREATE|O_EXCL is atomic in the kernel
	// on every platform astrogo supports, and [os.Root] provides it inside a
	// confined tree, so the local backend's implementation is exact rather than
	// best-effort. A backend that cannot offer the guarantee does not implement
	// this interface and is refused the lock, instead of appearing to hold one.
	CreateExclFS interface {
		fs.FS
		CreateExcl(name string) (io.WriteCloser, error)
	}

	// RemoveFS is a filesystem that can delete. The signature matches
	// s3iofs's deliberately, so an implementation written for one satisfies
	// the other.
	RemoveFS interface {
		fs.FS
		Remove(name string) error
	}

	// AbortWriter is a write that can be thrown away instead of committed.
	//
	// # Why Close is not enough
	//
	// Because an io.WriteCloser's Close cannot tell a finished write from an
	// abandoned one. A staged write commits on Close, which is what makes a
	// reader never see half an object — and it is exactly wrong when the copy
	// feeding it failed partway, because committing then replaces a good object
	// with a truncated one.
	//
	// The old answer was to skip Close, which the gocloud implementation did
	// and documented as leaking "the writer's temp resource on that rare path".
	// On Windows that leak is not benign: the unclosed handle keeps the
	// directory undeletable, which is how this surfaced.
	//
	// So a writer says which one it means. Abort releases everything Close
	// would and leaves the destination exactly as it was. Every writer this
	// package returns implements it; [WriteFile] falls back to skipping Close
	// for one that does not, accepting the leak rather than the corruption.
	AbortWriter interface {
		io.WriteCloser
		Abort() error
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
