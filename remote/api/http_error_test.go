package api

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// cdsOutagePage is the shape of the page CDS served for every SIMBAD request
// during its outage of 2026-11-13: a doctype, a style block longer than the
// 4 KB the error used to capture, and only then the reason.
var cdsOutagePage = "<!doctype html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n" +
	"<title>CDS - Page unavailable</title>\n<style>\n" +
	strings.Repeat("body { margin: 0; padding: 0; font-family: sans-serif; }\n", 90) +
	"</style>\n<script>window.dataLayer = [];</script>\n</head>\n<body>\n" +
	"<h1>Service unavailable</h1>\n<p>This page is currently unavailable.</p>\n" +
	"<p>We apologise &amp; are working on it.</p>\n</body>\n</html>\n"

// TestHTTPErrorIsOneReadableLine: an HTML error page used to print as its
// raw first 4 KB, the doctype and the whole style block over about 150
// lines, cut before the sentence that said why (#1135). Error is now one
// line, the page's visible text, without markup, styles or scripts.
func TestHTTPErrorIsOneReadableLine(t *testing.T) {
	t.Parallel()

	if len(cdsOutagePage) <= 4096 {
		t.Fatalf("the fixture is %d bytes; it must run past the old 4 KB capture to test it", len(cdsOutagePage))
	}

	msg := (&HTTPError{StatusCode: 404, Body: cdsOutagePage}).Error()

	if strings.Contains(msg, "\n") {
		t.Errorf("Error() spans several lines:\n%s", msg)
	}

	for _, want := range []string{"remote/api: http 404 - ", "CDS - Page unavailable", "This page is currently unavailable.", "We apologise & are working on it."} {
		if !strings.Contains(msg, want) {
			t.Errorf("Error() lacks %q: %s", want, msg)
		}
	}

	for _, markup := range []string{"<", "font-family", "dataLayer"} {
		if strings.Contains(msg, markup) {
			t.Errorf("Error() carries markup %q: %s", markup, msg)
		}
	}
}

// TestHTTPErrorKeepsAPlainAnswerAndBoundsItsLength: a TAP or plain-text
// answer is the reason itself, so it is kept, with its whitespace collapsed
// onto one line; any body is cut at maxExcerpt characters. Body keeps the
// bytes as they arrived.
func TestHTTPErrorKeepsAPlainAnswerAndBoundsItsLength(t *testing.T) {
	t.Parallel()

	tap := "Incorrect ADQL query:\n  Encountered \".\". Was expecting one of: <EOF> \",\""
	if got := (&HTTPError{StatusCode: 400, Body: tap}).Error(); got != `remote/api: http 400 - Incorrect ADQL query: Encountered ".". Was expecting one of: <EOF> ","` {
		t.Errorf("a plain answer reads %q", got)
	}

	long := strings.Repeat("é", 2*maxExcerpt)
	e := &HTTPError{StatusCode: 500, Body: long}

	excerpt := strings.TrimPrefix(e.Error(), "remote/api: http 500 - ")
	if n := utf8.RuneCountInString(excerpt); n != maxExcerpt+1 || !strings.HasSuffix(excerpt, "…") {
		t.Errorf("a long body's excerpt is %d characters, want %d and an ellipsis", n, maxExcerpt+1)
	}

	if e.Body != long {
		t.Error("Body no longer holds the response as it arrived")
	}
}

// TestHTMLTextSurvivesACutBody: a body cut short inside an element still
// gives the text before it, rather than markup or nothing.
func TestHTMLTextSurvivesACutBody(t *testing.T) {
	t.Parallel()

	for _, c := range []struct{ doc, want string }{
		{"<html><title>Gone</title><style>p { color: red", "Gone"},
		{"<html><p>Down for maintenance</p><scr", "Down for maintenance"},
		{"<HTML><STYLE>x</STYLE><P>Upper case &lt;tags&gt;</P></HTML>", "Upper case <tags>"},
	} {
		if got := strings.Join(strings.Fields(htmlText(c.doc)), " "); got != c.want {
			t.Errorf("htmlText(%q) = %q, want %q", c.doc, got, c.want)
		}
	}
}
