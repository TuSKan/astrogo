package core

import (
	"context"
	"errors"
	"sync/atomic"

	"github.com/TuSKan/astrogo/time"
)

// The kernel-backed sources are reached through a registered backend rather
// than by the root package importing one.
//
// # Why a registration rather than an import
//
// Asking where Mars is costs 14 MB and 424 packages, of which 12 MB and 131
// are an object-storage client the answer never touches: ephemeris imports
// ephemeris/jpl, which imports remote, which imports gocloud.dev/blob, which
// pulls 64 packages of gRPC for an error-code enum and 34 of OpenTelemetry
// because it instruments unconditionally. A pure-SOFA call — no kernel, no
// network, no file — paid for all of it because one branch of one factory
// mentioned the type.
//
// The same pattern remote/s3 already uses: a subpackage that is nothing but a
// registration, blank-imported by a build that needs the capability. See #112.
//
// # What it costs
//
// A compile-time failure becomes a runtime one. That is the trade, and it is
// why [ErrNoKernelBackend] names the import to add rather than merely reporting
// that something is missing.

// KernelRequest is everything a kernel-backed provider needs to build one.
//
// A struct rather than a parameter list because it crosses a registration
// boundary: a backend registered by one package and called by another cannot
// be kept in step by the compiler when an argument is added, and a silently
// dropped time interval is a provider that answers for the wrong window.
type KernelRequest struct {
	// Source is the kernel-backed source being asked for.
	Source Source

	// Kernel names the primary kernel — "de440", "433", and so on.
	Kernel string

	// Start and End restrict the coverage window for a generated small-body
	// kernel. Both zero means the backend's own default.
	Start, End time.Time

	// ExtraKernels are additional kernels to load after the primary one.
	ExtraKernels []string
}

// KernelBackend builds a provider for the kernel-backed sources.
type KernelBackend func(ctx context.Context, req KernelRequest) (Provider, error)

// ErrNoKernelBackend is returned for a kernel-backed source in a build that
// registered no backend.
//
// The message names the import because that is the whole remedy, and because
// the alternative — "unsupported source" — describes the symptom of a build
// decision as though it were a limit of the library.
var ErrNoKernelBackend = errors.New(
	"eph: this build has no kernel backend; add " +
		`import _ "github.com/TuSKan/astrogo/ephemeris/jpl"`)

// kernelBackend holds the registered backend, or nil.
//
// Atomic rather than mutex-guarded for the same reason time's leap-second
// registry is: it is read on a path callers hit repeatedly and written at most
// once, by an init.
var kernelBackend atomic.Pointer[KernelBackend]

// RegisterKernelBackend installs the backend the kernel-backed sources use.
//
// Called by ephemeris/jpl's init, which is what a blank import of that package
// runs. Nothing else should call it, and nothing needs to: there is one
// implementation and the registration exists to make importing it a decision
// rather than a default.
func RegisterKernelBackend(b KernelBackend) {
	if b == nil {
		kernelBackend.Store(nil)
		return
	}

	kernelBackend.Store(&b)
}

// KernelProvider builds a provider through the registered backend, or reports
// [ErrNoKernelBackend].
func KernelProvider(ctx context.Context, req KernelRequest) (Provider, error) {
	b := kernelBackend.Load()
	if b == nil {
		return nil, ErrNoKernelBackend
	}

	return (*b)(ctx, req)
}

// HasKernelBackend reports whether this build can serve the kernel-backed
// sources, for a caller that would rather check than fail.
func HasKernelBackend() bool { return kernelBackend.Load() != nil }
