package remote

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/TuSKan/astrogo/remote/api"
	"github.com/TuSKan/astrogo/time"
)

// ErrUnreachable marks a request to a host that gave no answer: it could not
// be reached, or nothing came back within the endpoint's timeout.
//
// A Client remembers such a host for a cooldown, five minutes unless
// [Client.SetUnreachableCooldown] says otherwise. A request to it inside the
// cooldown fails at once, without asking, with an error that wraps this and
// the failure that started the cooldown. One outage then costs a program one
// timeout rather than one per call: example 05's search and resolve each
// waited out SIMBAD in turn during the CDS outage of 2026-11-13, 73 s and then
// 61 s.
//
// A status the service sent is an [HTTPError], never this: a host that
// answers 404 or 503 answered. Neither is a request the caller's own context
// ended, which says nothing about the host.
var ErrUnreachable = errors.New("remote: no answer")

// defaultUnreachableCooldown is how long a Client remembers a host that gave
// no answer. It is the interval the Earth-orientation lazy load waits between
// attempts to fetch the IERS bulletin, astrogo's other "try again later".
const defaultUnreachableCooldown = 5 * time.Minute

// noAnswer is what a Client remembers about a host that gave no answer: when,
// and the failure.
type noAnswer struct {
	at  time.GoTime
	err error
}

// SetUnreachableCooldown sets how long c remembers a host that gave no
// answer: every request to it fails at once with [ErrUnreachable] until the
// cooldown has passed since the failure. The default is five minutes. Zero or
// less forgets at once, so every request asks again.
func (c *Client) SetUnreachableCooldown(d time.Duration) {
	c.noAnswerMu.Lock()
	defer c.noAnswerMu.Unlock()

	c.cooldown = d
}

// SetUnreachableCooldown sets how long the default client remembers a host
// that gave no answer. See [Client.SetUnreachableCooldown].
func SetUnreachableCooldown(d time.Duration) { Default().SetUnreachableCooldown(d) }

// hostOf is the host a request to base goes to, scheme and port included: a
// mirror set with [SetURL] is a different host from the one it replaces, and
// is asked even while that one is remembered.
func hostOf(base string) string {
	u, err := url.Parse(base)
	if err != nil || u.Host == "" {
		return base
	}

	return u.Scheme + "://" + u.Host
}

// answering returns nil when a request to host may be sent, and otherwise an
// error wrapping [ErrUnreachable] and the failure c remembers.
func (c *Client) answering(host string) error {
	c.noAnswerMu.Lock()
	defer c.noAnswerMu.Unlock()

	last, ok := c.noAnswer[host]
	if !ok {
		return nil
	}

	since := time.Since(last.at)
	if since >= c.cooldown {
		delete(c.noAnswer, host)

		return nil
	}

	return fmt.Errorf("%w: %s gave no answer %s ago, and is not asked again for %s more: %w",
		ErrUnreachable, host, since.Round(time.Second), (c.cooldown - since).Round(time.Second), last.err)
}

// heard records how a request to host ended, and returns its error, wrapped
// in [ErrUnreachable] when the host gave no answer. Any answer, an
// [HTTPError] included, clears what c remembers about the host. A request the
// caller's own context ended records nothing.
func (c *Client) heard(ctx context.Context, host string, err error) error {
	switch {
	case err == nil || !errors.Is(err, api.ErrNoResponse):
		c.noAnswerMu.Lock()
		delete(c.noAnswer, host)
		c.noAnswerMu.Unlock()

		return err
	case ctx.Err() != nil:
		return err
	}

	c.noAnswerMu.Lock()
	if c.cooldown > 0 {
		c.noAnswer[host] = noAnswer{at: time.Now(), err: err}
	}
	c.noAnswerMu.Unlock()

	return fmt.Errorf("%w: %s: %w", ErrUnreachable, host, err)
}
