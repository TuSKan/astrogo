package remote

import (
	"context"
	"fmt"
	"sync"
	"time"

	gofs "github.com/ungerik/go-fs"
)

// Transport implements the actual byte-transfer step for one Endpoint.Kind
// — resolving id+path to bytes on the wire. remote's own built-in
// transport covers KindAPI and KindFile using only stdlib net/http (see
// httpTransport); a caller needing a different Kind (e.g. KindS3)
// registers its own implementation via RegisterTransport rather than
// remote importing a protocol-specific SDK itself, keeping remote's own
// dependency footprint at stdlib + cenkalti/backoff/v5 + ungerik/go-fs
// regardless of which transports a particular build actually uses — see
// remote/s3's doc comment for the KindS3 implementation and why it lives
// in its own subpackage.
type Transport interface {
	// FetchInto downloads endpoint id's content at path into dest. The
	// caller (GetFile) has already performed the download-consent check
	// via CheckDownload against the endpoint's registered ApproxSize;
	// an implementation must still call CheckDownload again once it
	// knows the real transfer size, the same way httpTransport does,
	// since the registered ApproxSize is only ever an estimate. validate
	// and progress behave exactly as GetFile's own doc comment
	// describes (WithValidate/WithProgress).
	FetchInto(ctx context.Context, id EndpointID, path string, dest gofs.File, timeout time.Duration, validate func([]byte) error, progress func(downloaded, total int64)) error

	// Probe returns a lightweight freshness Signature for endpoint id's
	// content at path without transferring the body — used by GetFile to
	// decide whether a Mutable endpoint's cached copy is still valid.
	Probe(ctx context.Context, id EndpointID, path string) (Signature, error)
}

var (
	transportsMu sync.RWMutex
	transports   = map[Kind]Transport{
		KindAPI:  httpTransport{},
		KindFile: httpTransport{},
	}
)

// RegisterTransport plugs in a Transport implementation for Kind k,
// replacing remote's default (none, for any Kind other than
// KindAPI/KindFile) or an earlier registration for the same Kind. Safe to
// call more than once — the last registration wins — and safe for
// concurrent use. There is no automatic (init-time) registration for any
// non-built-in Kind: a caller wanting KindS3 support must explicitly
// import remote/s3 and call its Register function once before the first
// GetFile call against a KindS3 endpoint, matching this codebase's own
// "no hidden global mutation or init() side effects" convention.
//
// Reset does not clear or restore transport registrations. Transport
// registration is build-time wiring ("I linked remote/s3, so KindS3
// works"), not per-test configuration — resetting it on every Reset call
// would silently break a TestMain that registered S3 once for a whole
// test binary, the same class of bug this package already hit once for
// download consent.
func RegisterTransport(k Kind, t Transport) {
	transportsMu.Lock()
	defer transportsMu.Unlock()

	transports[k] = t
}

// transportFor returns the registered Transport for k, or ErrNoTransport
// wrapping k if none has been registered.
func transportFor(k Kind) (Transport, error) {
	transportsMu.RLock()
	defer transportsMu.RUnlock()

	t, ok := transports[k]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrNoTransport, k)
	}

	return t, nil
}
