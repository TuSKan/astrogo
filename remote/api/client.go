package api

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// DefaultTimeout applies to an endpoint whose registered Timeout is zero.
const DefaultTimeout = 30 * time.Second

// defaultUserAgent identifies astrogo to the services it calls, several of
// which (Nominatim in particular) require a real one. It names the project
// so an operator seeing unwelcome traffic has somewhere to look and someone
// to contact.
const defaultUserAgent = "AstroGo/1.0 (+https://github.com/TuSKan/astrogo)"

// defaultRetries bounds attempts for a retriable failure.
const defaultRetries = 3

// config carries NewClient's options.
type config struct {
	timeout     time.Duration
	retries     int
	userAgent   string
	minInterval time.Duration
	authScheme  string
	authToken   string
	retryPolicy RetryPolicy

	idempotentPost bool
}

// Option customizes a NewClient call.
type Option func(*config)

// WithTimeout overrides the endpoint's registered Timeout.
func WithTimeout(d time.Duration) Option {
	return func(c *config) { c.timeout = d }
}

// WithRetries sets how many times a retriable failure is re-attempted.
// Zero disables retrying.
func WithRetries(n int) Option {
	return func(c *config) { c.retries = n }
}

// WithUserAgent overrides the User-Agent header sent with every request.
func WithUserAgent(ua string) Option {
	return func(c *config) { c.userAgent = ua }
}

// WithAuthToken authenticates every request with an Authorization header,
// "<scheme> <token>". An empty scheme is "Bearer".
//
// The scheme is a parameter because services disagree, silently: Gaia@AIP
// accepts "Token" and ignores "Bearer", serving the request anonymously, so
// the wrong scheme shows as anonymous limits rather than as an error.
//
// The token goes in a header, never in a URL, which proxies and servers log,
// and this package never writes it to disk. Where it comes from is
// [github.com/TuSKan/astrogo/remote]'s decision.
func WithAuthToken(scheme, token string) Option {
	return func(c *config) { c.authScheme, c.authToken = scheme, token }
}

// WithMinInterval spaces this client's requests at least d apart, and lets
// one be in flight at a time: a rate limit and a concurrency limit at once,
// for a caller issuing hundreds of queries to a shared service that defends
// itself by no longer answering.
func WithMinInterval(d time.Duration) Option {
	return func(c *config) { c.minInterval = d }
}

// WithIdempotentPost declares that a POST through this client is a query:
// sent again, it gets the same answer and changes nothing on the server, as
// a TAP sync query or a JSON search does. Such a POST is retried under the
// client's [RetryPolicy], as a GET is. Without it a POST is sent once,
// whatever the policy says, since a POST that creates something, a UWS job,
// would create a second.
func WithIdempotentPost() Option {
	return func(c *config) { c.idempotentPost = true }
}

// Client issues paced, retried requests against a base URL given per call.
//
// It holds no endpoint and no URL: which service a request goes to is the
// caller's choice, one call at a time. [github.com/TuSKan/astrogo/remote]
// owns endpoints, offline mode and overrides.
//
// Safe for concurrent use.
type Client struct {
	hc        *http.Client
	transport *http.Transport

	userAgent string
	// auth is the Authorization header's value, empty for none.
	auth string

	retries        int
	idempotentPost bool
	// retryPolicy decides whether a failed attempt is retried, and whether a
	// final failure is reported as one the policy would have retried.
	retryPolicy RetryPolicy

	// timeout bounds a whole call: every attempt, the waits between them,
	// and the reading of the body.
	timeout time.Duration

	// mu serializes paced requests and guards last. Both are unused when no
	// minimum interval was asked for.
	minInterval time.Duration
	mu          sync.Mutex
	last        time.Time
}

// NewClient builds a client with timeout as the deadline of each call, its
// retries and its body included; zero means [DefaultTimeout]. A retry is made
// only inside it, so a call never takes longer than the timeout, however many
// attempts the policy allows.
func NewClient(timeout time.Duration, opts ...Option) *Client {
	cfg := config{
		timeout:     timeout,
		retries:     defaultRetries,
		userAgent:   defaultUserAgent,
		retryPolicy: DefaultRetryPolicy,
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	if cfg.timeout == 0 {
		cfg.timeout = DefaultTimeout
	}

	var auth string

	if cfg.authToken != "" {
		scheme := cfg.authScheme
		if scheme == "" {
			scheme = "Bearer"
		}

		auth = scheme + " " + cfg.authToken
	}

	transport := newTransport()

	return &Client{
		// No http.Client timeout: each call's budget context bounds it.
		hc:             &http.Client{Transport: transport},
		transport:      transport,
		userAgent:      cfg.userAgent,
		auth:           auth,
		retries:        cfg.retries,
		idempotentPost: cfg.idempotentPost,
		retryPolicy:    cfg.retryPolicy,
		timeout:        cfg.timeout,
		minInterval:    cfg.minInterval,
	}
}

// newTransport is a transport of this client's own, configured as
// net/http's default one, so Close can release its connections without
// touching anyone else's.
func newTransport() *http.Transport {
	if def, ok := http.DefaultTransport.(*http.Transport); ok {
		return def.Clone()
	}

	return &http.Transport{Proxy: http.ProxyFromEnvironment, ForceAttemptHTTP2: true}
}

// Timeout reports the per-call timeout this client was built with.
func (c *Client) Timeout() time.Duration { return c.timeout }

// Close releases the client's idle connections. A Client is usually held
// for the life of a provider, so this is optional.
func (c *Client) Close() error {
	c.transport.CloseIdleConnections()

	return nil
}

// Get issues a GET against baseURL/path with query added to whatever query
// baseURL carries, and returns the response body for the caller to close. A
// non-2xx response is an *HTTPError instead of a body, so a caller never
// parses an error page as data.
func (c *Client) Get(ctx context.Context, baseURL string, path string, query url.Values) (io.ReadCloser, error) {
	full, err := requestURL(baseURL, path)
	if err != nil {
		return nil, err
	}

	full, err = withQuery(full, query)
	if err != nil {
		return nil, err
	}

	release, err := c.pace(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	return c.do(ctx, http.MethodGet, full, "", nil)
}

// GetJSON issues a GET and decodes the JSON response into out, closing the
// body. For endpoints that always answer JSON; a provider that must sniff
// the format from raw bytes uses Get instead.
func (c *Client) GetJSON(ctx context.Context, baseURL string, path string, query url.Values, out any) error {
	r, err := c.Get(ctx, baseURL, path, query)
	if err != nil {
		return err
	}

	defer func() { _ = r.Close() }()

	if err := json.UnmarshalRead(r, out, decodeOptions...); err != nil {
		return fmt.Errorf("remote/api: decode JSON from %q: %w", baseURL, err)
	}

	return nil
}

// PostForm issues a POST with an application/x-www-form-urlencoded body
// and returns the raw response body: the shape TAP-ADQL services and any
// endpoint whose response format must be sniffed need.
func (c *Client) PostForm(ctx context.Context, baseURL string, path string, form url.Values) (io.ReadCloser, error) {
	full, err := requestURL(baseURL, path)
	if err != nil {
		return nil, err
	}

	release, err := c.pace(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	return c.do(ctx, http.MethodPost, full, "application/x-www-form-urlencoded", []byte(form.Encode()))
}

// PostJSON marshals payload as the JSON request body and returns the raw
// response body.
func (c *Client) PostJSON(ctx context.Context, baseURL string, path string, payload any) (io.ReadCloser, error) {
	full, err := requestURL(baseURL, path)
	if err != nil {
		return nil, err
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("remote/api: marshal JSON body: %w", err)
	}

	release, err := c.pace(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	return c.do(ctx, http.MethodPost, full, "application/json", encoded)
}

// do sends one call: its attempts, the waits between them, and the
// response, all inside the call's budget. A success hands the budget to the
// returned body, which releases it on Close; anything else releases it here.
//
// A POST is retried only from a client built [WithIdempotentPost]. A failed
// attempt is retried while the policy says so, attempts remain, and the wait
// before the next one ends inside the budget.
func (c *Client) do(ctx context.Context, method, full, contentType string, body []byte) (io.ReadCloser, error) {
	ctx, cancel := c.budget(ctx)

	retries := c.retries
	if method == http.MethodPost && !c.idempotentPost {
		retries = 0
	}

	for attempt := 1; ; attempt++ {
		resp, err := c.send(ctx, method, full, contentType, body)

		a := Attempt{Number: attempt, Err: err}
		if resp != nil {
			a.StatusCode = resp.StatusCode
		}

		if err == nil && resp.StatusCode < http.StatusBadRequest {
			return &budgetedBody{ReadCloser: resp.Body, cancel: cancel}, nil
		}

		if attempt <= retries && ctx.Err() == nil && c.retryPolicy(a) {
			wait := retryWait(resp, attempt)

			if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > wait {
				discard(resp)

				if sleep(ctx, wait) == nil {
					continue
				}
			}
		}

		return c.failure(resp, err, a, cancel)
	}
}

// send makes one attempt. The body is read from a reader of its own, so an
// earlier attempt's write that net/http has not finished, as when a server
// answers before reading the whole body, never shares a cursor with it.
func (c *Client) send(ctx context.Context, method, full, contentType string, body []byte) (*http.Response, error) {
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, full, r)
	if err != nil {
		return nil, fmt.Errorf("remote/api: build request: %w", err)
	}

	req.Header.Set("User-Agent", c.userAgent)

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	if c.auth != "" {
		req.Header.Set("Authorization", c.auth)
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err //nolint:wrapcheck // failure wraps it with ErrNoResponse
	}

	return resp, nil
}

// failure turns a final failed attempt into the error every method here
// returns, and releases the call's budget.
//
// A transport failure, no answer at all, wraps [ErrNoResponse]. A status is
// an *HTTPError carrying the body, since these services describe their
// failures in it, and wraps [ErrRetriable] when the policy would have
// retried that status: the attempts ran out, rather than the request being
// wrong.
func (c *Client) failure(resp *http.Response, err error, a Attempt, cancel context.CancelFunc) (io.ReadCloser, error) {
	defer cancel()

	if err != nil {
		discard(resp)

		return nil, fmt.Errorf("remote/api: request failed: %w: %w", ErrNoResponse, err)
	}

	defer func() { _ = resp.Body.Close() }()

	detail, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
	httpErr := &HTTPError{StatusCode: resp.StatusCode, Body: string(detail)}

	if c.retryPolicy(a) {
		return nil, fmt.Errorf("%w: %w", ErrRetriable, httpErr)
	}

	return nil, httpErr
}

// discard drains a little of a response being abandoned and closes it, so
// its connection can be reused.
func discard(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}

	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxErrorBody))
	_ = resp.Body.Close()
}

// sleep waits d, or until ctx ends, which it reports.
func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return fmt.Errorf("remote/api: waiting to retry: %w", ctx.Err())
	case <-timer.C:
		return nil
	}
}

// budget bounds one call by the client's timeout: every attempt and every
// wait between them comes out of it. Pacing waits before it starts, since
// that wait is spent being polite to the service, not waiting for it.
func (c *Client) budget(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, c.timeout)
}

// pace blocks until this client is allowed to make its next request.
//
// It returns a release function so the caller holds the slot for the duration
// of the request rather than only its start: one request is in flight at a
// time, not merely departures evenly spaced.
func (c *Client) pace(ctx context.Context) (release func(), err error) {
	if c.minInterval <= 0 {
		return func() {}, nil
	}

	c.mu.Lock()

	wait := time.Until(c.last.Add(c.minInterval))
	if wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()

		select {
		case <-ctx.Done():
			c.mu.Unlock()

			return nil, fmt.Errorf("api: waiting to pace a request: %w", ctx.Err())
		case <-timer.C:
		}
	}

	return func() {
		c.last = time.Now()
		c.mu.Unlock()
	}, nil
}

// requestURL appends path to baseURL through net/url, so a base URL that
// carries a query, a mirror with a token in it, keeps it after the path.
func requestURL(baseURL string, path string) (string, error) {
	if path == "" {
		return baseURL, nil
	}

	u, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("remote/api: unparseable base URL %q: %w", baseURL, err)
	}

	return u.JoinPath(path).String(), nil
}

// withQuery adds query to the query full already carries.
func withQuery(full string, query url.Values) (string, error) {
	if len(query) == 0 {
		return full, nil
	}

	u, err := url.Parse(full)
	if err != nil {
		return "", fmt.Errorf("remote/api: unparseable URL %q: %w", full, err)
	}

	q := u.Query()

	for k, vs := range query {
		for _, v := range vs {
			q.Add(k, v)
		}
	}

	u.RawQuery = q.Encode()

	return u.String(), nil
}

// budgetedBody is a response body that releases its call's budget when it is
// closed. The budget has to outlive the call that returned the body, since the
// caller reads the body afterwards.
type budgetedBody struct {
	io.ReadCloser

	cancel context.CancelFunc
}

// Close closes the body and releases the call's budget.
func (b *budgetedBody) Close() error {
	err := b.ReadCloser.Close()
	b.cancel()

	if err != nil {
		return fmt.Errorf("remote/api: closing a response body: %w", err)
	}

	return nil
}

// maxErrorBody bounds how much of a failed response is captured in
// [HTTPError.Body]: enough to reach the sentence an HTML error page puts
// after its styles. It bounds memory, not the message: [HTTPError.Error]
// prints an excerpt.
const maxErrorBody = 64 << 10

// maxExcerpt is the most characters of the body [HTTPError.Error] prints,
// keeping the message to one readable line.
const maxExcerpt = 300

// HTTPError is a non-2xx response from an API endpoint. The body is
// captured because these services put the actual reason in it — a bad ADQL
// query, an unknown designation, a rate-limit notice.
type HTTPError struct {
	// Body is the response as it arrived, up to 64 KiB.
	Body       string
	StatusCode int
}

// Error is one line: the status and an excerpt of the body. An HTML page's
// excerpt is its visible text, without the markup, scripts or styles around
// it; any body has its whitespace collapsed and is cut at 300 characters.
func (e *HTTPError) Error() string {
	return fmt.Sprintf("remote/api: http %d - %s", e.StatusCode, excerpt(e.Body))
}

// excerpt is the part of a response body an error message shows: its
// visible text if it is an HTML page, with whitespace collapsed, cut at
// maxExcerpt characters.
func excerpt(body string) string {
	text := body
	if looksLikeHTML(body) {
		text = htmlText(body)
	}

	text = strings.Join(strings.Fields(text), " ")

	if r := []rune(text); len(r) > maxExcerpt {
		text = string(r[:maxExcerpt]) + "…"
	}

	return text
}

// looksLikeHTML reports whether body is an HTML document rather than the
// plain text, JSON or VOTable these services answer with otherwise.
func looksLikeHTML(body string) bool {
	head := strings.ToLower(strings.TrimSpace(body))

	return strings.HasPrefix(head, "<!doctype html") || strings.HasPrefix(head, "<html")
}

// htmlText is an HTML document's visible text: the content of script and
// style elements removed, then every tag, then entities decoded. An element
// left open, as a body cut short leaves one, is removed to the end.
func htmlText(doc string) string {
	for _, element := range []string{"script", "style"} {
		doc = removeElements(doc, element)
	}

	var b strings.Builder

	for {
		open := strings.IndexByte(doc, '<')
		if open < 0 {
			b.WriteString(doc)

			break
		}

		b.WriteString(doc[:open])
		b.WriteByte(' ')

		closing := strings.IndexByte(doc[open:], '>')
		if closing < 0 {
			break
		}

		doc = doc[open+closing+1:]
	}

	return html.UnescapeString(b.String())
}

// removeElements removes every element named name, with its content, from
// doc, matching the tag case-insensitively.
func removeElements(doc, name string) string {
	lower := strings.ToLower(doc)

	var b strings.Builder

	for {
		start := strings.Index(lower, "<"+name)
		if start < 0 {
			b.WriteString(doc)

			return b.String()
		}

		b.WriteString(doc[:start])

		end := strings.Index(lower[start:], "</"+name+">")
		if end < 0 {
			return b.String()
		}

		cut := start + end + len("</"+name+">")
		doc, lower = doc[cut:], lower[cut:]
	}
}

// HTTPStatus returns the status code of the response that produced this error.
//
// It exists so a caller can branch on the status — backing off on a 429,
// treating a 5xx as the service's fault rather than its own — through a small
// interface rather than a dependency on this package's concrete type.
func (e *HTTPError) HTTPStatus() int { return e.StatusCode }

// decodeOptions holds the two places where encoding/json/v2's defaults are
// deliberately turned back to v1's behavior, and by omission the one place
// they are not.
//
// # What v2 is here for
//
// Measured on an SBDB-shaped 5.4 KB response: 24.5 us against v1's 37.4 us,
// 12.1 KB against 28.8 KB, 9 allocations against 20. Every JSON response any
// astrogo provider receives comes through this one function, so that is the
// whole of the library's API decoding.
//
// # Duplicate object names stay rejected, which is v2's default
//
// v1 silently took the last of a repeated key. That is the failure this
// library keeps finding in other guises: a wrong answer delivered with
// confidence, indistinguishable from a right one. A response that repeats a
// name is malformed, and saying so is worth more than guessing which
// occurrence was meant.
//
// # Case-insensitive matching is restored, which v2 drops
//
// v2 matches JSON names to struct fields exactly, so a service that renamed
// "ra" to "RA" would stop binding — silently, leaving the field zero. For a
// coordinate that is not a missing value: RA 0, Dec 0 is a real point in
// Pisces, and a target built on it rises, transits and sets without
// complaint. plan.ErrNoCoordinates exists because that exact substitution
// reached production once already.
//
// So exact matching trades a silently wrong value for a silently zero one,
// which is not an improvement, and v1's leniency is kept. Every response
// astrogo receives today matches exactly — the whole network suite passes
// with this option removed — so this costs nothing now and covers a rename
// later.
//
// # Invalid UTF-8 is tolerated, which v2 rejects
//
// A bad byte in an object's *name* should not cost the caller its position.
// v1 replaced it with U+FFFD and that is the right trade here: the field that
// might be mangled is a label, and the fields that must be right are numbers.
var decodeOptions = []json.Options{
	json.MatchCaseInsensitiveNames(true),
	jsontext.AllowInvalidUTF8(true),
}
