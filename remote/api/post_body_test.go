package api_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/remote/api"
)

// TestPostBodiesSurviveAnEarlyAnswer: a server that answers a POST before
// reading its body leaves net/http writing the body after RoundTrip has
// returned. resty used to hand that body's pooled buffer back to its pool at
// exactly that point (#1107). The write still in progress would then read a
// buffer another request had taken, and could send that request's bytes or
// empty them.
//
// Three kinds of request go at once:
//   - early ones, answered 400 by a server that never reads them, so their only
//     attempt returns while its write is still blocked;
//   - retried ones, answered 503 that way on the first attempt and read in
//     full on the retry, so a retry starts while the first attempt's write is
//     still blocked;
//   - whole ones, read in full and checked.
//
// Under -race, as CI's race shard runs it, pooling the body is a reported data
// race, and so is a retry that shares a reader with the earlier attempt's
// write. Without -race the test still holds that every body that is read
// arrives as its own, whole.
func TestPostBodiesSurviveAnEarlyAnswer(t *testing.T) {
	// The early bodies must not fit in the loopback connection's buffers, so
	// that their write blocks; the read ones only need to be checkable.
	const (
		perKind   = 4
		blocking  = 16 << 20
		checkable = 64 << 10
	)

	var seen sync.Map // path → its first attempt has been answered

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		kind, _, _ := strings.Cut(name, "-")
		_, retried := seen.LoadOrStore(name, true)

		size := checkable
		if kind != "whole" {
			size = blocking
		}

		switch {
		case kind == "early":
			answerWithoutReading(t, w, http.StatusBadRequest)
		case kind == "retry" && !retried:
			answerWithoutReading(t, w, http.StatusServiceUnavailable)
		default:
			got, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)

				return
			}

			if want := (url.Values{"q": {letterFor(name, size)}}).Encode(); string(got) != want {
				http.Error(w, fmt.Sprintf("%s received %d bytes that are not its own %d", name, len(got), len(want)),
					http.StatusBadRequest)

				return
			}

			_, _ = io.WriteString(w, "ok")
		}
	}))
	t.Cleanup(srv.Close)

	c := newClient(t, api.WithIdempotentPost())

	var wg sync.WaitGroup

	for i := range perKind {
		for _, kind := range []string{"early", "retry", "whole"} {
			path := fmt.Sprintf("%s-%d", kind, i)

			size := checkable
			if kind != "whole" {
				size = blocking
			}

			wg.Go(func() {
				body, err := c.PostForm(context.Background(), srv.URL, path, url.Values{"q": {letterFor(path, size)}})

				if kind == "early" {
					var he *api.HTTPError
					if !errors.As(err, &he) || he.StatusCode != http.StatusBadRequest {
						t.Errorf("%s: err = %v, want the server's 400", path, err)
					}

					return
				}

				if err != nil {
					t.Errorf("%s: %v", path, err)

					return
				}

				defer func() { _ = body.Close() }()

				if got, _ := io.ReadAll(body); string(got) != "ok" {
					t.Errorf("%s: the server answered %q", path, got)
				}
			})
		}
	}

	wg.Wait()
}

// answerWithoutReading sends status on a hijacked connection and holds the
// connection open without reading the request body, so the client's write of
// a body larger than the connection's buffers blocks after the answer has
// arrived. Then it closes the connection.
func answerWithoutReading(t *testing.T, w http.ResponseWriter, status int) {
	t.Helper()

	hj, ok := w.(http.Hijacker)
	if !ok {
		t.Error("the test server's ResponseWriter cannot be hijacked")

		return
	}

	conn, rw, err := hj.Hijack()
	if err != nil {
		t.Errorf("hijack: %v", err)

		return
	}

	defer func() { _ = conn.Close() }()

	_, _ = fmt.Fprintf(rw, "HTTP/1.1 %d %s\r\nContent-Length: 0\r\nConnection: close\r\n\r\n", status, http.StatusText(status))
	_ = rw.Flush()

	time.Sleep(300 * time.Millisecond)
}

// letterFor is the value the request for path sends: one letter, repeated,
// chosen from the path, so a body that arrives at the wrong request, or mixed
// with another, shows.
func letterFor(path string, size int) string {
	sum := 0
	for _, r := range path {
		sum += int(r)
	}

	return strings.Repeat(string(rune('a'+sum%26)), size)
}
