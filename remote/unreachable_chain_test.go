package remote

import (
	"context"
	"net"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
)

// TestAnUnreachableSourceStaysRecognisableThroughGetFile is the reproduction
// #348 asked for, and it exists because a guard that was in place did not fire.
//
// # What happened
//
// ephemeris/jpl's kernel tests already skip when JPL is unreachable, using
// testutil.Unreachable on the error NewProvider returns. On the run recorded in
// #348 NAIF timed out, the guard did not fire, and the build went red on
// somebody else's outage anyway.
//
// Reconstructing the error chain by hand — a net.OpError inside a url.Error
// inside the fmt.Errorf wraps that remote/file, remote and jpl each add — the
// predicate returns true at every level. So the predicate is not the problem,
// and neither is the wrapping as written down. The difference is somewhere in
// what the real stack produces, which no reconstruction can settle.
//
// # Why this is in remote and not in the test that failed
//
// Because the property is remote's, not jpl's: every sentinel this package
// exports is matched with errors.Is through exactly this chain, so if something
// in the fetch path flattens an error, ErrOffline, ErrDownloadDenied and
// ErrNotServingData are all unreachable too and the failing test is only the
// first symptom.
//
// # Why a refused connection rather than a timeout
//
// A timeout is what NAIF produced, and waiting for one costs the timeout. A
// refused connection is instant, deterministic, needs no external service, and
// travels the identical path — dial fails, net/http wraps it, remote/file wraps
// that, remote wraps that. If a wrap along the way breaks the chain it breaks
// it for both.
func TestAnUnreachableSourceStaysRecognisableThroughGetFile(t *testing.T) {
	cleanRemoteState(t)

	// A port nothing is listening on: bind one, learn its number, release it.
	// Anything dialling it afterwards is refused immediately.
	var lc net.ListenConfig

	l, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		// Fatal, and the line releasing this listener below already was: a
		// machine that cannot bind a loopback port cannot run this test at all,
		// and the two halves of the same operation should not disagree about
		// whether failing at it is news.
		t.Fatalf("reserve a local port: %v", err)
	}

	addr := l.Addr().String()

	if err := l.Close(); err != nil {
		t.Fatalf("release the port: %v", err)
	}

	if err := SetURL(NAIFSPK, "http://"+addr+"/"); err != nil {
		t.Fatal(err)
	}

	EnableDownloads(0, NAIFSPK)

	_, _, err = GetFile(context.Background(), NAIFSPK, "planets/de440s.bsp")
	if err == nil {
		t.Fatal("GetFile succeeded against a port with nothing on it")
	}

	if !testutil.Unreachable(err) {
		t.Errorf("testutil.Unreachable = false for a refused connection through GetFile.\n"+
			"error: %v\n"+
			"This is #348: something between the dial and here is flattening the error, "+
			"so every errors.Is against a remote sentinel fails through this path too.", err)
	}
}
