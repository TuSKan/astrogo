package testutil

import (
	"crypto/x509"
	"errors"
)

// certificateFailure reports whether err is a TLS handshake that failed for a
// reason belonging to the service rather than to astrogo, and names it.
//
// # Why this is not covered by the checks beside it
//
// It falls between the two guards a network test has. [RequireReachable] asks
// whether anything is listening, and something is: the TCP connection opens
// and the handshake fails afterwards, so the pre-check passes and the test
// proceeds. [Unreachable] counts a timeout, a DNS failure and a refused or
// unroutable dial, and a certificate error is none of those.
//
// It stopped being hypothetical on 2026-09-20, when api.open-elevation.com's
// certificate expired and TestNewSiteEarthAddress_Live began failing on every
// run — a red build nothing in this repository could fix, which is the case
// the network-test policy exists to prevent.
//
// # What counts, and what deliberately does not
//
// Counted:
//
//   - An expired or not-yet-valid certificate. The operator's renewal lapsed
//     or their clock is wrong. Nothing about the request can be corrected.
//   - A certificate signed by an authority the machine does not trust. That is
//     a property of the endpoint's chain or of the runner's trust store, and
//     in neither case is it a statement about astrogo's request.
//
// Not counted, and this is the distinction worth keeping:
//
//   - A certificate valid for a different name ([x509.HostnameError]). The
//     service is answering with proof of who it is, and astrogo asked for
//     somebody else — which usually means the endpoint registry points at the
//     wrong host, or a URL moved. That is exactly the defect these tests exist
//     to catch, so it stays fatal.
//
// It is the same line [SkipOnUpstreamFailure] draws between a 403 and a 401:
// the service declining is the service's business, and astrogo being wrong
// about the service is not.
//
// # This changes tests, not the library
//
// testutil is imported only by tests. A certificate failure still reaches a
// real caller as an error from remote — nothing here makes astrogo accept a
// certificate it should refuse.
func certificateFailure(err error) (string, bool) {
	if err == nil {
		return "", false
	}

	// Checked before the two below it: a HostnameError is a valid certificate
	// presented for another name, and must not be laundered into a skip by a
	// broader check that follows.
	//
	// By value, not by pointer. VerifyHostname returns `HostnameError{c, h}`,
	// so a match against *x509.HostnameError never fires — which is how this
	// was first written. It happened to give the right answer anyway, because
	// neither arm below claims a HostnameError either, so the function fell
	// through to the final false. The guard was inert rather than wrong, which
	// is the worse of the two: it states an invariant it is not enforcing, and
	// the first arm added below it would silently start skipping the case this
	// whole function exists to keep fatal.
	if _, ok := errors.AsType[x509.HostnameError](err); ok {
		return "", false
	}

	if invalid, ok := errors.AsType[x509.CertificateInvalidError](err); ok {
		if invalid.Reason == x509.Expired {
			return "the service's TLS certificate is expired or not yet valid", true
		}

		// Every other reason — a bad constraint, a wrong key usage, an
		// unhandled critical extension — says the chain is malformed rather
		// than stale. Left fatal: it is rare enough that seeing it is worth
		// more than skipping past it.
		return "", false
	}

	if _, ok := errors.AsType[x509.UnknownAuthorityError](err); ok {
		return "the service's TLS certificate is signed by an untrusted authority", true
	}

	return "", false
}
