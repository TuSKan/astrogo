package starlight

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/remote/api"
)

// ErrAsyncJob reports that an asynchronous TAP job did not produce a result.
var ErrAsyncJob = errors.New("starlight: asynchronous query")

// asyncPoll is how often a running job is asked whether it has finished.
//
// Two seconds. A whole-sky aggregation runs for minutes, so polling faster
// buys nothing and only adds requests to a service that is already doing the
// expensive part; polling much slower adds latency to the short jobs.
const asyncPoll = 2 * time.Second

// asyncQueue is the service queue a job is submitted to; see the note where
// it is set.
const asyncQueue = "2h"

// asyncPhaseCap bounds how long a job is waited on.
//
// Separate from the per-request timeout, which governs a single exchange with
// the service and not the job's own runtime. The whole point of the
// asynchronous endpoint is that the job outlives any one request.
const asyncPhaseCap = 45 * time.Minute

// runAsync submits an ADQL query as a UWS job, waits for it, and returns the
// result stream.
//
// The IVOA asynchronous protocol in the four steps a caller actually needs:
// POST the query, which creates a job in PENDING; POST phase RUN to start it;
// poll the phase until it settles; then fetch the result. Each step is an
// ordinary form POST or GET against the job's own URL under the async
// endpoint.
//
// It exists because the synchronous endpoint gives up: a GROUP BY over the
// whole Gaia catalogue is a minutes-long query and the service aborts it after
// about one, which is what forced a whole-sky build into 787 range-restricted
// chunks. Asynchronously it is one query.
//
// The returned stream is the caller's to close.
func runAsync(ctx context.Context, client *api.Client, id remote.EndpointID, adql string) (io.ReadCloser, error) {
	v := url.Values{}
	v.Set("REQUEST", "doQuery")
	v.Set("LANG", "ADQL")
	v.Set("FORMAT", "csv")
	v.Set("PHASE", "RUN") // start on creation, so no separate run step is needed
	v.Set("QUERY", adql)

	// The long queue, and the reason a whole-sky aggregation completes at all.
	//
	// The service runs jobs in queues that differ by the statement timeout
	// they set: the default one gives a query well under a minute, and the "2h"
	// queue issues SET SESSION statement_timeout TO 7200000 before running it.
	// A GROUP BY over the whole Gaia catalogue takes about fourteen minutes,
	// so on the default queue it is cancelled mid-flight with "canceling
	// statement due to statement timeout" — which reads like a service problem
	// and is a missing parameter.
	//
	// Asking for it unconditionally is right for this endpoint: nothing sent
	// asynchronously here is short enough to prefer the fast queue, and a job
	// that finishes early is not charged for the queue it did not use.
	v.Set("QUEUE", asyncQueue)

	created, err := client.PostForm(ctx, id, "", v)
	if err != nil {
		return nil, fmt.Errorf("%w: submit: %w", ErrAsyncJob, err)
	}

	job, err := io.ReadAll(created)
	_ = created.Close()

	if err != nil {
		return nil, fmt.Errorf("%w: submit: %w", ErrAsyncJob, err)
	}

	jobID, err := jobIDOf(job)
	if err != nil {
		return nil, err
	}

	if err := awaitPhase(ctx, client, id, jobID); err != nil {
		return nil, err
	}

	body, err := client.Get(ctx, id, jobID+"/results/result", nil)
	if err != nil {
		return nil, fmt.Errorf("%w %s: result: %w", ErrAsyncJob, jobID, err)
	}

	return body, nil
}

// jobIDOf pulls the job identifier out of the document the service returns
// when a job is created.
//
// Some services answer with the full UWS job description and some redirect to
// it; either way the client has followed to a document carrying the id. The
// element is namespaced, so it is matched on local name rather than on a
// prefix the service is free to choose.
func jobIDOf(doc []byte) (string, error) {
	dec := xml.NewDecoder(strings.NewReader(string(doc)))

	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return "", fmt.Errorf("%w: reading the job description: %w", ErrAsyncJob, err)
		}

		el, ok := tok.(xml.StartElement)
		if !ok || el.Name.Local != "jobId" {
			continue
		}

		var id string
		if err := dec.DecodeElement(&id, &el); err != nil {
			return "", fmt.Errorf("%w: reading the job id: %w", ErrAsyncJob, err)
		}

		if id = strings.TrimSpace(id); id != "" {
			return id, nil
		}
	}

	// Worth quoting: a service that refuses a query answers here, and the
	// message is the only thing that says why.
	summary := strings.TrimSpace(string(doc))
	if len(summary) > 400 {
		summary = summary[:400] + "..."
	}

	return "", fmt.Errorf("%w: the service returned no job id: %s", ErrAsyncJob, summary)
}

// awaitPhase polls a job until it settles, and reports what it settled as.
func awaitPhase(ctx context.Context, client *api.Client, id remote.EndpointID, jobID string) error {
	deadline := time.Now().Add(asyncPhaseCap)

	for {
		phase, err := phaseOf(ctx, client, id, jobID)
		if err != nil {
			return err
		}

		switch phase {
		case "COMPLETED":
			return nil

		case "ERROR", "ABORTED":
			return fmt.Errorf("%w %s: the service reported %s: %s",
				ErrAsyncJob, jobID, phase, errorSummary(ctx, client, id, jobID))

		case "PENDING", "QUEUED", "EXECUTING", "UNKNOWN", "HELD", "SUSPENDED":
			// Still going.

		default:
			return fmt.Errorf("%w %s: unrecognised phase %q", ErrAsyncJob, jobID, phase)
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("%w %s: still %s after %v", ErrAsyncJob, jobID, phase, asyncPhaseCap)
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("%w %s: %w", ErrAsyncJob, jobID, ctx.Err())
		case <-time.After(asyncPoll):
		}
	}
}

// phaseOf reads a job's current phase.
func phaseOf(ctx context.Context, client *api.Client, id remote.EndpointID, jobID string) (string, error) {
	body, err := client.Get(ctx, id, jobID+"/phase", nil)
	if err != nil {
		return "", fmt.Errorf("%w %s: phase: %w", ErrAsyncJob, jobID, err)
	}

	defer func() { _ = body.Close() }()

	raw, err := io.ReadAll(body)
	if err != nil {
		return "", fmt.Errorf("%w %s: phase: %w", ErrAsyncJob, jobID, err)
	}

	return strings.ToUpper(strings.TrimSpace(string(raw))), nil
}

// errorSummary fetches a failed job's own explanation.
//
// Best effort: a job that failed and whose error document cannot be read is
// still a failed job, and the phase already says so. This only turns "ERROR"
// into something a reader can act on.
func errorSummary(ctx context.Context, client *api.Client, id remote.EndpointID, jobID string) string {
	body, err := client.Get(ctx, id, jobID+"/error", nil)
	if err != nil {
		return "no error document"
	}

	defer func() { _ = body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(body, 2000))
	if err != nil {
		return "no error document"
	}

	return strings.Join(strings.Fields(string(raw)), " ")
}
