package testutil

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The three dates the fixtures below are built from, as offsets in years from
// the standard library's zero time.Time — year 1.
//
// Reached that way rather than from a clock because this package sits below
// astrogo/time and cannot import either spelling of the time package (see
// ReachableTimeout for the same constraint stated at a constant, and
// docsguard's TestNoStandardLibraryTimeOutsideTimePackage for the rule). A
// composite literal's field gives the zero value without naming the type.
//
// Fixed dates, not offsets from now, so these tests answer the same on any day
// they run. Real ones, not year 1 and year 3001: Windows verifies through the
// platform's certificate API, which rejects a date that far outside its range
// with "an error occurred during encode or decode operation" before it has an
// opinion on the certificate. That is worth having gone through, because the
// platform verifier is also what the live test meets, so these fixtures now
// exercise it rather than routing around it.
//
// The valid-until year is 2200. Nothing here survives that, but it is the sort
// of thing that should be written down rather than discovered.
// The two non-certificate errors the negative case below is built from, as
// sentinels because the repository's convention is that an error is a value
// with a name rather than a string built where it is needed.
var (
	errSomethingWentWrong = errors.New("something went wrong")
	errConnectionRefused  = errors.New("connection refused")
)

var (
	certValidFrom  = x509.Certificate{}.NotBefore.AddDate(1999, 0, 0) // 2000-01-01
	certExpiredAt  = x509.Certificate{}.NotBefore.AddDate(2019, 0, 0) // 2020-01-01
	certValidUntil = x509.Certificate{}.NotBefore.AddDate(2199, 0, 0) // 2200-01-01
)

// certFixture describes one bad handshake: which name the certificate is
// issued for, which name the client asks for, whether it has expired, and
// whether the client is handed it as a trusted root.
//
// trusted is what separates the cases rather than a convenience. A verifier
// reports the first thing it objects to, and the order is not the same
// everywhere: Go's own verifier checks the dates, then the name, then builds a
// chain, while Windows' platform API builds the chain first and never reaches
// the name. So a self-signed certificate for the wrong host reports the name on
// Linux and the chain on Windows, and the only way to ask about the name on all
// three CI platforms is to make the chain acceptable.
//
// It is also the more honest fixture. A real endpoint serving a certificate for
// a host astrogo did not ask for has a perfectly good certificate from a
// perfectly good CA — the defect is in astrogo's idea of where the service is.
type certFixture struct {
	certName string
	askFor   string
	expired  bool
	trusted  bool
}

// handshakeError does one HTTPS request against a throwaway server built to
// fx's description, and returns the error the client got back.
//
// A live handshake rather than hand-built error values. What is under test is
// which error the standard library actually produces and how deeply it wraps
// it, and a synthetic x509 error would assert this package's belief about that
// instead of the behaviour. Two things a synthetic error would have got wrong
// here: VerifyHostname returns HostnameError by value, not by pointer, and the
// verifier ordering described above is not what reading crypto/x509 alone
// suggests, because two of the three CI platforms do not use it.
func handshakeError(t *testing.T, fx certFixture) error {
	t.Helper()

	notAfter := certValidUntil
	if fx.expired {
		notAfter = certExpiredAt
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating a key: %v", err)
	}

	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: fx.certName},
		NotBefore:             certValidFrom,
		NotAfter:              notAfter,
		DNSNames:              []string{fx.certName},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA:                  true,
		BasicConstraintsValid: true,
	}

	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("creating the certificate: %v", err)
	}

	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parsing the certificate: %v", err)
	}

	srv := httptest.NewUnstartedServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	srv.TLS = &tls.Config{
		Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}},
		MinVersion:   tls.VersionTLS12,
	}
	srv.StartTLS()

	t.Cleanup(srv.Close)

	// A nil pool leaves the machine's own trust store in charge — and so leaves
	// the platform verifier in charge, which is what the live test meets and
	// therefore what is worth exercising. A pool holding the certificate makes
	// the chain acceptable so that a later objection can be reached.
	var roots *x509.CertPool

	if fx.trusted {
		roots = x509.NewCertPool()
		roots.AddCert(leaf)
	}

	// The dialer ignores the address it is handed and goes to the listener, so
	// askFor never reaches a resolver and the names above need not exist. The
	// client still verifies against askFor, which is the whole point.
	client := &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, srv.Listener.Addr().String())
		},
		TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12},
	}}
	defer client.CloseIdleConnections()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://"+fx.askFor, nil)
	if err != nil {
		t.Fatalf("building the request: %v", err)
	}

	resp, err := client.Do(req)
	if err == nil {
		_ = resp.Body.Close()

		t.Fatal("the handshake succeeded; this fixture exists to make it fail")
	}

	// Returned exactly as the standard library produced it. Wrapping it would
	// add this file's own words to the thing being classified, and the wrapping
	// is part of what these tests are checking the classifier can see through.
	return err //nolint:wrapcheck // the unwrapped error is the subject of the test
}

// TestAnExpiredCertificateIsTheServicesProblem is #360: api.open-elevation.com
// let its certificate lapse on 2026-09-20 and TestNewSiteEarthAddress_Live
// began failing on every run — past the TCP pre-check, which the handshake
// happens after, and through every network predicate, none of which describes
// a certificate.
func TestAnExpiredCertificateIsTheServicesProblem(t *testing.T) {
	t.Parallel()

	// Untrusted as well as expired, deliberately: the platform verifiers
	// disagree about which to report, and both answers are ones this classifier
	// must skip on, so pinning either would pin the platform rather than the
	// contract.
	err := handshakeError(t, certFixture{
		certName: "expired.example",
		askFor:   "expired.example",
		expired:  true,
	})

	reason, ok := certificateFailure(err)
	if !ok {
		t.Fatalf("certificateFailure(%v) = false; a lapsed renewal is the operator's, "+
			"and nothing about astrogo's request can be corrected to satisfy it", err)
	}

	if _, up := upstreamFailure(err); !up {
		t.Errorf("upstreamFailure = false, want a skip citing %q — this is the "+
			"classifier TestNewSiteEarthAddress_Live actually reaches, through "+
			"SkipOnUpstreamFailure", reason)
	}

	if !Unreachable(err) {
		t.Error("Unreachable = false; the handshake failed, so the request was never carried " +
			"and the two classifiers should agree")
	}
}

// TestAnUntrustedAuthorityIsTheServicesProblem covers the other counted case: a
// certificate current on its dates that chains to nothing the machine trusts.
func TestAnUntrustedAuthorityIsTheServicesProblem(t *testing.T) {
	t.Parallel()

	err := handshakeError(t, certFixture{
		certName: "untrusted.example",
		askFor:   "untrusted.example",
	})

	if _, ok := errors.AsType[x509.UnknownAuthorityError](err); !ok {
		t.Fatalf("the fixture produced %v, not an unknown-authority error; "+
			"it can no longer isolate the case this test is about", err)
	}

	if _, ok := certificateFailure(err); !ok {
		t.Errorf("certificateFailure(%v) = false for an untrusted chain", err)
	}

	if !Unreachable(err) {
		t.Error("Unreachable = false for an untrusted chain")
	}
}

// TestACertificateForAnotherNameStaysFatal is the line this classifier draws,
// and the reason it is not simply "any TLS error".
//
// A certificate valid for a different host means the service answered with
// proof of who it is and astrogo asked for somebody else — an endpoint registry
// pointing at the wrong host, or a URL that moved. Skipping past that would
// hide exactly the defect these tests exist to catch, the same way
// SkipOnUpstreamFailure keeps a 401 fatal while a 403 skips.
//
// It passes whichever way certificateFailure's hostname guard is spelled,
// which is worth knowing rather than hiding: that guard matched
// *x509.HostnameError at first and so never fired, and the answer came out
// right only because the arms below it decline a HostnameError too. What this
// test pins is the outcome the classifier owes its callers. The guard being
// the thing that produces it is certificateFailure's own business, and is
// argued there.
func TestACertificateForAnotherNameStaysFatal(t *testing.T) {
	t.Parallel()

	// Current dates and a chain the client accepts, so the name is the only
	// thing left to object to. See certFixture for why the trust matters.
	err := handshakeError(t, certFixture{
		certName: "issued-for.example",
		askFor:   "asked-for.example",
		trusted:  true,
	})

	if _, ok := errors.AsType[x509.HostnameError](err); !ok {
		t.Fatalf("the fixture produced %v, not a hostname error; it can no longer "+
			"isolate the case this test is about", err)
	}

	if _, ok := certificateFailure(err); ok {
		t.Error("certificateFailure = true for a name mismatch; astrogo asking for " +
			"the wrong host is a defect to see, not an outage to skip")
	}

	if _, ok := upstreamFailure(err); ok {
		t.Error("upstreamFailure = true for a name mismatch; the test that would have " +
			"reported the wrong endpoint now skips instead")
	}
}

// TestCertificateFailureIgnoresEverythingElse guards the negative: this
// predicate must not widen into the errors its neighbours already classify, or
// into errors nobody classifies at all.
func TestCertificateFailureIgnoresEverythingElse(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		err  error
	}{
		{"nil", nil},
		{"a plain error", errSomethingWentWrong},
		{"a cancelled context", context.Canceled},
		{"a deadline", context.DeadlineExceeded},
		{"a DNS failure", &net.DNSError{Err: "no such host", IsNotFound: true}},
		{"a refused dial", &net.OpError{Op: "dial", Err: errConnectionRefused}},
	} {
		if reason, ok := certificateFailure(tc.err); ok {
			t.Errorf("%s: certificateFailure = %q, true; want false", tc.name, reason)
		}
	}
}
