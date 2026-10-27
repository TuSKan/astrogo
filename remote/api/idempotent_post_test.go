package api_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/TuSKan/astrogo/remote/api"
)

// TestPostIsRetriedOnlyWhenDeclaredIdempotent: a POST through a client built
// WithIdempotentPost is retried under the policy, as a GET is; without it, a
// POST is sent once (#845). Before the option existed every POST was the
// second case, so no TAP query was ever retried while DefaultRetryPolicy
// documented that it would be.
func TestPostIsRetriedOnlyWhenDeclaredIdempotent(t *testing.T) {
	t.Parallel()

	post := map[string]func(*api.Client, string) (io.ReadCloser, error){
		"PostForm": func(c *api.Client, base string) (io.ReadCloser, error) {
			return c.PostForm(context.Background(), base, "", url.Values{"QUERY": {"SELECT 1"}})
		},
		"PostJSON": func(c *api.Client, base string) (io.ReadCloser, error) {
			return c.PostJSON(context.Background(), base, "", map[string]int{"n": 1})
		},
	}

	for name, send := range post {
		for _, tc := range []struct {
			opts     []api.Option
			attempts int32
		}{
			{[]api.Option{api.WithIdempotentPost()}, 2},
			{nil, 1},
		} {
			// One 503, then an answer.
			var attempts atomic.Int32

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("%s sent a %s", name, r.Method)
				}

				if attempts.Add(1) == 1 {
					w.WriteHeader(http.StatusServiceUnavailable)

					return
				}

				_, _ = io.WriteString(w, "answer")
			}))

			body, err := send(newClient(t, tc.opts...), srv.URL)
			if body != nil {
				_ = body.Close()
			}

			srv.Close()

			declared := len(tc.opts) > 0
			if got := attempts.Load(); got != tc.attempts {
				t.Errorf("%s, declared idempotent %v: %d requests, want %d", name, declared, got, tc.attempts)
			}

			if declared != (err == nil) {
				t.Errorf("%s, declared idempotent %v: err = %v", name, declared, err)
			}
		}
	}
}
