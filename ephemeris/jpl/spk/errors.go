package spk

import "errors"

// Sentinel errors for SPK operations.
var (
	// ErrHorizonsBadRequest indicates a 400 response from JPL Horizons.
	ErrHorizonsBadRequest = errors.New("jpl: horizons bad request (400): check keywords/content")
	// ErrHorizonsMethodNA indicates a 405 response from JPL Horizons.
	ErrHorizonsMethodNA = errors.New("jpl: horizons method not allowed (405)")
	// ErrHorizonsServerError indicates a 500 response from JPL Horizons.
	ErrHorizonsServerError = errors.New("jpl: horizons internal server error (500): database unavailable")
	// ErrHorizonsUnavailable indicates a 503 response from JPL Horizons.
	ErrHorizonsUnavailable = errors.New("jpl: horizons service unavailable (503): temporary overload/maintenance")
	// ErrHorizonsUnexpected indicates an unexpected HTTP status from JPL Horizons.
	ErrHorizonsUnexpected = errors.New("jpl: horizons unexpected status")

	// ErrHorizonsRefused is returned when Horizons answers a request and
	// declines it, carrying the service's own explanation.
	//
	// It exists because a refusal used to arrive as an empty kernel list and
	// a nil error, which reads as success: the provider carried on with only
	// its planetary base, and the caller learned about it much later as a
	// body that was simply missing. Horizons is specific about why — asked
	// for 101955 Bennu it answers "SPK creation is not available for
	// pre-computed objects in the major body index" — and discarding that
	// left the caller to guess at designation syntax that was never wrong.
	ErrHorizonsRefused = errors.New("jpl: horizons declined to generate an SPK")

	// ErrHorizonsInternalFault is a refusal that is not about the request.
	//
	// It wraps ErrHorizonsRefused as well, so every existing caller keeps
	// working; what it adds is the distinction between Horizons deciding it
	// will not answer and Horizons being unable to. Those look identical on
	// the wire — HTTP 200, a well-formed JSON body, an explanation in the
	// "error" field — and they mean opposite things to a caller. One is a
	// permanent property of what was asked for and should be reported. The
	// other is somebody else's outage and will fix itself.
	//
	// Observed 2026-09-15, for both 433 Eros and Apophis, in astrogo's own CI:
	//
	//	wldini(): missing required file LTKERNL; ;
	//	ERROR in VLRDC: Var not declared: IP_ADDR
	//
	// LTKERNL is a leap-second kernel on Horizons' server and IP_ADDR is one
	// of its internal variables. Nothing a caller can send produces that, and
	// the same two requests succeeded again within the hour.
	//
	// # On matching a third party's error text
	//
	// It is fragile, and it is the only signal available: the status is 200
	// and the JSON shape is identical either way. It is kept safe by being
	// deliberately narrow rather than clever — see horizonsInternalFault. A
	// fault mode this does not recognize stays an ordinary refusal and fails
	// loudly, which is the right default for a signal nobody has seen before.
	ErrHorizonsInternalFault = errors.New("jpl: horizons could not generate an SPK because of a fault on its own server")

	// ErrCorruptSPK indicates a malformed SPK binary kernel.
	ErrCorruptSPK = errors.New("jpl/spk: corrupt file")
	// ErrInvalidWordBounds indicates invalid double-precision word boundaries in an SPK record.
	ErrInvalidWordBounds = errors.New("jpl/spk: invalid double precision word bounds")
	// ErrNoCoverage indicates no ephemeris data covers the requested target/epoch.
	ErrNoCoverage = errors.New("jpl: no coverage for target")
	// ErrUnsupportedSegment indicates an SPK segment type that is not implemented.
	ErrUnsupportedSegment = errors.New("jpl: unsupported SPK segment type")
	// ErrInvalidRecordCount indicates an invalid record count in an SPK segment.
	ErrInvalidRecordCount = errors.New("jpl: invalid record count")
	// ErrRecordTooShort indicates an SPK record that is shorter than expected.
	ErrRecordTooShort = errors.New("jpl: record too short")
	// ErrInvalidOrder indicates a polynomial order outside the valid range.
	ErrInvalidOrder = errors.New("jpl: polynomial order out of valid range")

	// ErrHorizonsEmptyKernel indicates Horizons returned a syntactically
	// valid SPK (correct DAF file record, a well-formed comment area) whose
	// summary area is entirely absent — live-confirmed (not assumed) by
	// decoding a real Horizons response byte-for-byte: the file record's
	// own FWARD/BWARD claim a summary record exists, but every record from
	// the end of the comment area through EOF is all zero bytes, so
	// ReadSummaries legitimately finds nothing to read. This is JPL
	// Horizons' own server generating an unusable kernel for a specific
	// request, not a decode/write bug on astrogo's side — CacheAPI detects
	// it and refuses to cache the broken file rather than silently
	// succeeding with a kernel that covers nothing.
	ErrHorizonsEmptyKernel = errors.New("jpl: horizons returned an SPK with no segment summaries")
)

// TransientHorizonsFault reports whether err is a Horizons-side failure that
// will resolve itself — as opposed to a permanent fact about what was asked
// for.
//
// The distinction is not visible in the transport. Both arrive as HTTP 200
// with a well-formed body, and astrogo has now seen two separate ways for
// Horizons to answer successfully while being unable to do the work:
// ErrHorizonsEmptyKernel (a syntactically valid SPK containing nothing) and
// ErrHorizonsInternalFault (a refusal naming a fault on its own server). Both
// were live-confirmed, and both recovered on their own.
//
// A caller deciding whether to retry wants exactly this question answered, and
// so does a test deciding whether to skip or fail: astrogo's own untagged,
// live-network tests must never turn somebody else's outage into a red build,
// while a real refusal — "SPK creation is not available for pre-computed
// objects in the major body index" — has to stay loud, because it means the
// request will never work and the caller needs to know.
func TransientHorizonsFault(err error) bool {
	return errors.Is(err, ErrHorizonsEmptyKernel) || errors.Is(err, ErrHorizonsInternalFault)
}
