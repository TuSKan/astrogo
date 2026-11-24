package remote

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// mirrorClient is a client whose SIMBAD endpoint is primary, with mirrors,
// and a timeout short enough to wait out in a test. It sets them in the
// client's own registry, since SetURL clears an endpoint's mirrors.
func mirrorClient(t *testing.T, primary string, mirrors ...string) *Client {
	t.Helper()

	c := NewClient()

	c.mu.Lock()
	ep := c.endpoints[SIMBAD]
	ep.URL, ep.Mirrors = primary, mirrors
	c.endpoints[SIMBAD] = ep
	c.mu.Unlock()

	c.SetAPIOptions(WithTimeout(300 * time.Millisecond))

	return c
}

// answeringHost is a server that answers every request with status and body,
// counting them.
func answeringHost(t *testing.T, status int, body string) (base string, asked *atomic.Int32) {
	t.Helper()

	asked = new(atomic.Int32)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		asked.Add(1)
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)

	return srv.URL + "/sync", asked
}

// TestASilentPrimaryIsAnsweredByItsMirror: SIMBAD's primary gave no answer
// for days after the CDS incident of 2026-11-13, and every caller had to know
// to point SetURL at a mirror (#1214). Now each request method, finding the
// primary silent, asks the endpoint's mirrors in turn and returns the first
// answer, passing over a mirror that is silent too. The next request does not
// ask the primary again: it is remembered as unreachable.
func TestASilentPrimaryIsAnsweredByItsMirror(t *testing.T) {
	t.Parallel()

	form := url.Values{"QUERY": {"SELECT 1"}}

	for _, m := range []struct {
		name string
		call func(c *Client) (string, error)
	}{
		{"Get", func(c *Client) (string, error) { return read(c.Get(t.Context(), SIMBAD, "", nil)) }},
		{"PostForm", func(c *Client) (string, error) { return read(c.PostForm(t.Context(), SIMBAD, "", form)) }},
		{"PostJSON", func(c *Client) (string, error) {
			return read(c.PostJSON(t.Context(), SIMBAD, "", map[string]int{"a": 1}))
		}},
		{"GetJSON", func(c *Client) (string, error) {
			var out struct{ Answer string }

			err := c.GetJSON(t.Context(), SIMBAD, "", nil, &out)

			return out.Answer, err
		}},
	} {
		t.Run(m.name, func(t *testing.T) {
			t.Parallel()

			primary, primaryAccepted := silentHost(t)
			silentMirror, _ := silentHost(t)
			mirror, mirrorAsked := answeringHost(t, http.StatusOK, `{"Answer":"mirror"}`)

			c := mirrorClient(t, primary, silentMirror, mirror)

			for i := range 2 {
				got, err := m.call(c)
				if err != nil || !strings.Contains(got, "mirror") {
					t.Fatalf("request %d: %q, %v; want the mirror's answer", i+1, got, err)
				}
			}

			if n := primaryAccepted.Load(); n != 1 {
				t.Errorf("the silent primary was connected to %d times over two requests, want once", n)
			}

			if n := mirrorAsked.Load(); n != 2 {
				t.Errorf("the answering mirror was asked %d times, want 2", n)
			}
		})
	}
}

// read is a body's text, closing it, or the request's error.
func read(body io.ReadCloser, err error) (string, error) {
	if err != nil {
		return "", err
	}

	defer func() { _ = body.Close() }()

	b, err := io.ReadAll(body)

	return string(b), err
}

// TestAPrimaryThatRefusesTheQueryIsTheAnswer: a primary that answers 400, a
// query it refused as malformed, is the answer, and no mirror is asked to
// settle a request it would refuse too.
func TestAPrimaryThatRefusesTheQueryIsTheAnswer(t *testing.T) {
	t.Parallel()

	primary, _ := answeringHost(t, http.StatusBadRequest, "bad ADQL")
	mirror, mirrorAsked := answeringHost(t, http.StatusOK, "mirror")

	c := mirrorClient(t, primary, mirror)

	var httpErr *HTTPError
	if _, err := read(c.Get(t.Context(), SIMBAD, "", nil)); !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusBadRequest {
		t.Errorf("Get from a primary answering 400 = %v, want its HTTPError", err)
	}

	if n := mirrorAsked.Load(); n != 0 {
		t.Errorf("the mirror was asked %d times, want none", n)
	}
}

// TestAPrimaryNotServingIsAnsweredByItsMirror: a primary that answers 404, as
// SIMBAD's maintenance page did after the CDS incident, or 503 once its
// retries are spent, is not serving the API, and a mirror answers instead.
func TestAPrimaryNotServingIsAnsweredByItsMirror(t *testing.T) {
	t.Parallel()

	for _, status := range []int{http.StatusNotFound, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()

			primary, primaryAsked := answeringHost(t, status, "<html>page unavailable</html>")
			mirror, _ := answeringHost(t, http.StatusOK, "mirror")

			got, err := read(mirrorClient(t, primary, mirror).Get(t.Context(), SIMBAD, "", nil))
			if err != nil || got != "mirror" {
				t.Errorf("Get from a primary answering %d = %q, %v; want the mirror's answer", status, got, err)
			}

			if primaryAsked.Load() == 0 {
				t.Errorf("the primary was never asked")
			}
		})
	}
}

// TestWhenNoHostAnswersEachIsNamed: with the primary and every mirror silent,
// the error is ErrUnreachable and names each host. A request the caller's own
// context ended asks no mirror after the host it was waiting on.
func TestWhenNoHostAnswersEachIsNamed(t *testing.T) {
	t.Parallel()

	primary, _ := silentHost(t)
	mirror, mirrorAccepted := silentHost(t)

	_, err := read(mirrorClient(t, primary, mirror).Get(t.Context(), SIMBAD, "", nil))
	if !errors.Is(err, ErrUnreachable) {
		t.Fatalf("Get with every host silent = %v, want ErrUnreachable", err)
	}

	for _, host := range []string{hostOf(primary), hostOf(mirror)} {
		if !strings.Contains(err.Error(), host) {
			t.Errorf("the error does not name %s: %v", host, err)
		}
	}

	if n := mirrorAccepted.Load(); n != 1 {
		t.Errorf("the silent mirror was connected to %d times, want once", n)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()

	primary2, _ := silentHost(t)
	mirror2, mirror2Accepted := silentHost(t)

	if _, err := read(mirrorClient(t, primary2, mirror2).Get(ctx, SIMBAD, "", nil)); err == nil {
		t.Fatal("Get under a context that ended succeeded")
	}

	if n := mirror2Accepted.Load(); n != 0 {
		t.Errorf("after the caller's own context ended, a mirror was connected to %d times, want none", n)
	}
}

// TestSetURLClearsTheMirrors: a URL the caller chose with SetURL is the one
// asked, so it clears the endpoint's mirrors, and Reset brings them back.
func TestSetURLClearsTheMirrors(t *testing.T) {
	t.Parallel()

	c := NewClient()

	before, _ := c.Lookup(SIMBAD)
	if len(before.Mirrors) == 0 {
		t.Fatal("SIMBAD has no mirrors registered")
	}

	if err := c.SetURL(SIMBAD, "https://proxy.invalid/sync"); err != nil {
		t.Fatal(err)
	}

	if after, _ := c.Lookup(SIMBAD); len(after.Mirrors) != 0 {
		t.Errorf("after SetURL, SIMBAD's mirrors are %v, want none", after.Mirrors)
	}

	c.Reset()

	if reset, _ := c.Lookup(SIMBAD); len(reset.Mirrors) != len(before.Mirrors) {
		t.Errorf("after Reset, SIMBAD's mirrors are %v, want %v", reset.Mirrors, before.Mirrors)
	}
}

// TestMirrorsAreOtherHostsOfAnAPI: every registered mirror belongs to an API
// endpoint that carries no token, is an https URL, and is on a host other
// than the endpoint's own and the other mirrors', since a request is passed
// over by host.
func TestMirrorsAreOtherHostsOfAnAPI(t *testing.T) {
	t.Parallel()

	for _, ep := range Endpoints() {
		if len(ep.Mirrors) == 0 {
			continue
		}

		if ep.Kind != KindAPI {
			t.Errorf("%s: mirrors on a KindFile endpoint, which no request method serves", ep.ID)
		}

		// A mirror is asked through the endpoint's transport, token included.
		if ep.TokenEnv != "" {
			t.Errorf("%s: mirrors on an endpoint that carries a token (%s), which would reach the mirrors' hosts",
				ep.ID, ep.TokenEnv)
		}

		hosts := map[string]bool{hostOf(ep.URL): true}

		for _, m := range ep.Mirrors {
			u, err := url.Parse(m)
			if err != nil || u.Scheme != "https" || u.Host == "" {
				t.Errorf("%s: mirror %q is not an https URL", ep.ID, m)
			}

			if hosts[hostOf(m)] {
				t.Errorf("%s: mirror %q shares a host with the endpoint or another mirror", ep.ID, m)
			}

			hosts[hostOf(m)] = true
		}
	}
}
