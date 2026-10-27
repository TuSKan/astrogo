package docsguard_test

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// liveTag is a build constraint naming one of the tiers CLAUDE.md puts live
// services behind.
var liveTag = regexp.MustCompile(`^//go:build .*\b(network|integration|validation)\b`)

// unboundedRequest is a live request that can wait for ever: one built on a
// context with no deadline, or one sent by net/http's package-level helpers,
// which take no context and use http.DefaultClient, whose timeout is none.
var unboundedRequest = regexp.MustCompile(
	`http\.NewRequestWithContext\(\s*(context\.Background\(\)|context\.TODO\(\)|(t|b|tb)\.Context\(\))` +
		`|\bhttp\.(DefaultClient\.)?(Get|Post|Head|PostForm)\(`)

// TestLiveRequestsHaveADeadline refuses a live test's request that nothing
// bounds (#849).
//
// On 2026-10-22 FINK stopped answering mid-request. magnitude's finkPost had
// built its request on context.Background() and sent it with
// http.DefaultClient, so it waited as long as the server did: go test's
// ten-minute limit panicked the package, and every result in it was lost. The
// skip logic around the call never ran, because no error ever came back to
// skip on. Ten live-service tests had the same shape. t.Context() is among the
// refused contexts because it is canceled when the test ends, not at any
// deadline.
//
// The fix gives each request its endpoint's own timeout, so a hung service
// fails the request, which skips through SkipOnUpstreamFailure. This guard
// keeps the next live test from going back to the shape. It reads only the
// line that builds or sends the request, so a context variable derived from
// Background elsewhere is beyond it: it catches the form these tests had, which
// is the form a new one copies.
func TestLiveRequestsHaveADeadline(t *testing.T) {
	t.Parallel()

	files := readTestFiles(t, filepath.Join("..", "..", ""))

	var live int

	for path, lines := range files {
		if !hasLiveTag(lines) {
			continue
		}

		live++

		for i, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue
			}

			if unboundedRequest.MatchString(line) {
				t.Errorf("%s:%d: a live request nothing bounds.\n"+
					"  %s\n"+
					"  Build it on a context with the endpoint's timeout, as remote.Lookup(id).Timeout gives it, "+
					"or remote.DefaultAPITimeout, so a hung service fails the request and the test skips (#849).",
					path, i+1, strings.TrimSpace(line))
			}
		}
	}

	// The walk or the tag pattern being wrong would otherwise pass vacuously.
	if live == 0 {
		t.Fatal("no network-, integration- or validation-tagged test files found: the walk or liveTag is wrong")
	}
}

// hasLiveTag reports whether a file's build constraint, above its package
// clause, names a live tier.
func hasLiveTag(lines []string) bool {
	for _, line := range lines {
		if strings.HasPrefix(line, "package ") {
			return false
		}

		if liveTag.MatchString(line) {
			return true
		}
	}

	return false
}

// TestUnboundedRequestPattern pins what the guard reads as unbounded, and what
// it lets through: a request built on a deadline-bearing context, and a client
// call that takes a request rather than a URL.
func TestUnboundedRequestPattern(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		line string
		want bool
	}{
		{`req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)`, true},
		{`req, err := http.NewRequestWithContext(context.TODO(), http.MethodPost, u, body)`, true},
		{`req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)`, true},
		{`resp, err := http.Get(reqURL)`, true},
		{`resp, err := http.Post(u, "application/json", body)`, true},
		{`resp, err := http.DefaultClient.Get(u)`, true},
		{`resp, err := http.PostForm(u, values)`, true},
		{`req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)`, false},
		{`resp, err := http.DefaultClient.Do(req)`, false},
		{`resp, err := client.Get(u)`, false},
		{`server := httptest.NewServer(handler)`, false},
	} {
		if got := unboundedRequest.MatchString(tc.line); got != tc.want {
			t.Errorf("unboundedRequest.MatchString(%q) = %v, want %v", tc.line, got, tc.want)
		}
	}
}
