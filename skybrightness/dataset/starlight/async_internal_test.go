package starlight

import (
	"errors"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/remote"
)

// The job id is found wherever the service puts it.
//
// UWS namespaces the element, and the prefix is the service's to choose, so
// matching on a prefix rather than the local name works against one service
// and fails against the next.
func TestJobIDOfFindsANamespacedElement(t *testing.T) {
	t.Parallel()

	for _, c := range []struct{ name, doc, want string }{
		{
			"uws prefix",
			`<uws:job xmlns:uws="http://www.ivoa.net/xml/UWS/v1.0">` +
				`<uws:jobId>abc-123</uws:jobId><uws:phase>PENDING</uws:phase></uws:job>`,
			"abc-123",
		},
		{
			"no prefix",
			`<job xmlns="http://www.ivoa.net/xml/UWS/v1.0">` +
				`<jobId>plain-9</jobId></job>`,
			"plain-9",
		},
		{
			"another prefix entirely",
			`<u:job xmlns:u="http://www.ivoa.net/xml/UWS/v1.0"><u:jobId>x7</u:jobId></u:job>`,
			"x7",
		},
		{
			"surrounded by other elements",
			`<uws:job xmlns:uws="http://www.ivoa.net/xml/UWS/v1.0">` +
				`<uws:ownerId>someone</uws:ownerId><uws:jobId> spaced </uws:jobId>` +
				`<uws:quote/></uws:job>`,
			"spaced",
		},
	} {
		got, err := jobIDOf([]byte(c.doc))
		if err != nil {
			t.Errorf("%s: %v", c.name, err)

			continue
		}

		if got != c.want {
			t.Errorf("%s: job id %q, want %q", c.name, got, c.want)
		}
	}
}

// A document with no job in it is an error carrying what the service said.
//
// This is the case that matters in practice: a refused query answers here, and
// the service's own message is the only thing that says why. An error that
// dropped it would turn "your ADQL is invalid because X" into "no job id".
func TestJobIDOfQuotesARefusal(t *testing.T) {
	t.Parallel()

	const refusal = `<VOTABLE><RESOURCE type="results">` +
		`<INFO name="QUERY_STATUS" value="ERROR">Field bp_rp: no such column</INFO>` +
		`</RESOURCE></VOTABLE>`

	_, err := jobIDOf([]byte(refusal))
	if !errors.Is(err, ErrAsyncJob) {
		t.Fatalf("err = %v, want ErrAsyncJob", err)
	}

	if !strings.Contains(err.Error(), "no such column") {
		t.Errorf("the error dropped the service's explanation: %v", err)
	}
}

// An empty or unparseable body does not yield a job id.
func TestJobIDOfRejectsRubbish(t *testing.T) {
	t.Parallel()

	for _, c := range []struct{ name, doc string }{
		{"empty", ""},
		{"not xml", "Service Unavailable"},
		{"empty job id", `<uws:job xmlns:uws="u"><uws:jobId></uws:jobId></uws:job>`},
		{"truncated", `<uws:job xmlns:uws="u"><uws:jobId>abc`},
	} {
		if got, err := jobIDOf([]byte(c.doc)); err == nil {
			t.Errorf("%s: returned job id %q", c.name, got)
		}
	}
}

// The long queue is requested, and that is not cosmetic.
//
// A whole-sky aggregation runs about fourteen minutes. On the service's
// default queue it is cancelled with "canceling statement due to statement
// timeout", which reads like a service fault and is a missing parameter; the
// "2h" queue sets the session statement timeout to 7200000 ms. This pins the
// value so that dropping it fails here rather than a quarter of an hour into a
// build.
func TestAsyncQueueIsTheLongOne(t *testing.T) {
	t.Parallel()

	if asyncQueue != "2h" {
		t.Errorf("asyncQueue = %q, want the long queue %q", asyncQueue, "2h")
	}
}

// Only the asynchronous endpoint takes the job protocol.
func TestIsAsyncNamesOnlyTheAsyncEndpoint(t *testing.T) {
	t.Parallel()

	if !isAsync(remote.GaiaAIPAsync) {
		t.Error("the asynchronous endpoint is not recognised as one")
	}

	for _, id := range []remote.EndpointID{remote.GaiaTAP, remote.GaiaAIP, remote.VizieR} {
		if isAsync(id) {
			t.Errorf("%s is treated as asynchronous", id)
		}
	}
}
