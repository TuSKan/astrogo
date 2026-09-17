package remote

import (
	"bytes"
	"errors"
)

// ErrNotServingData reports that an endpoint answered, with a success status,
// and what it sent was not data.
//
// # Why this is not an HTTPError
//
// [HTTPError] covers a service declining with a status code, and #208 settled
// that a 403 is the service declining rather than a data error. This is the
// same statement made less honestly: archives serve their own failures — a
// maintenance notice, a load shedder, a login wall — as an HTML page with a
// 200, so nothing before the parser can tell. There is no status code to
// branch on, which is precisely why there has to be something.
//
// # Why a caller needs it
//
// Because the two possibilities want opposite handling, and until this existed
// they arrived as the same opaque error string:
//
//	the archive served a web page   back off and retry later; this is downtime
//	the payload is malformed        stop; retrying will not help
//
// Without the distinction the honest options are to retry everything or to
// retry nothing, and both are wrong some of the time. The only alternative was
// matching on message text, which is the practice the sentinels in #190 exist
// to end.
//
// # Why it lives in remote rather than beside a parser
//
// The condition is discovered by a parser — only something that knows what the
// payload should look like can notice that it is a web page — but it is not a
// statement about the payload. It is a statement about what the endpoint did,
// which is what this package already speaks for with [ErrOffline],
// [ErrDownloadDenied] and [HTTPError].
//
// It also has to be reachable from both sides of the architecture. `catalog`
// providers hit it, and so does `skybrightness/dataset/starlight` fetching a
// Gaia map; `skybrightness` cannot import `catalog` without inverting the
// layering, so a sentinel on `catalog/resolve` would have served one of them
// and forced the other to invent a second name for the same condition. Both
// already import this package.
//
// Detection stays with the parsers. This package sees bytes and does not know
// what a VOTable is; it owns the classification, not the recognition.
var ErrNotServingData = errors.New("remote: the endpoint answered with a web page, not data")

// LooksLikeHTML reports whether head begins an HTML document.
//
// Pass the first few hundred bytes of a response body — enough to clear a byte
// order mark and any leading blank lines, which some services emit before the
// doctype. Reading more decides nothing: a document that has not declared
// itself HTML in its opening element is not going to.
//
// It is a heuristic and is meant to be one. The question it answers is "did
// this archive send its error page instead of the payload", where the archive
// controls both and the answer is obvious from the first tag. It is not a
// content-type sniffer and must not be used as one — in particular it says
// nothing about XML, since a VOTable is XML too and the whole point is to
// separate the two.
//
// Case is ignored, because `<HTML>` is still a web page.
func LooksLikeHTML(head []byte) bool {
	// A UTF-8 byte order mark ahead of the document, which some services
	// emit and which would otherwise hide the first tag.
	head = bytes.TrimPrefix(head, []byte{0xEF, 0xBB, 0xBF})

	head = bytes.TrimLeft(head, " \t\r\n")

	// Lowercased once over a bounded prefix rather than per comparison. The
	// caller is expected to pass a few hundred bytes, but a caller who passes
	// a whole multi-megabyte document should not pay for all of it.
	const enough = 64

	if len(head) > enough {
		head = head[:enough]
	}

	head = bytes.ToLower(head)

	return bytes.HasPrefix(head, []byte("<!doctype html")) ||
		bytes.HasPrefix(head, []byte("<html"))
}
