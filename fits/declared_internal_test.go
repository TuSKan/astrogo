package fits

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

// seekFault is the part of a source's account of itself that is wrong.
type seekFault int

const (
	claimsEmpty   seekFault = iota // its end is where it already is
	claimsMore                     // its end is a megabyte past what it holds
	cannotTell                     // it cannot report its position
	cannotFindEnd                  // it cannot seek to its end
	cannotReturn                   // it cannot be moved back
)

var errSeek = errors.New("seek refused")

// misbehavingSeeker stands for a source that can seek in name only.
type misbehavingSeeker struct {
	*bytes.Reader

	fault seekFault
}

func (s misbehavingSeeker) Seek(offset int64, whence int) (int64, error) {
	switch {
	case s.fault == cannotTell && whence == io.SeekCurrent,
		s.fault == cannotFindEnd && whence == io.SeekEnd,
		s.fault == cannotReturn && whence == io.SeekStart:
		return 0, errSeek
	case s.fault == claimsEmpty && whence == io.SeekEnd:
		return s.Reader.Seek(0, io.SeekCurrent) //nolint:wrapcheck // a test double
	case s.fault == claimsMore && whence == io.SeekEnd:
		cur, err := s.Reader.Seek(0, io.SeekCurrent)

		return cur + int64(s.Len()) + 1<<20, err //nolint:wrapcheck // a test double
	}

	return s.Reader.Seek(offset, whence) //nolint:wrapcheck // a test double
}

// brokenReader delivers a few bytes and then fails with something other than
// EOF, as a disk or a network can.
type brokenReader struct{ sent bool }

var errBroken = errors.New("read failed")

func (b *brokenReader) Read(p []byte) (int, error) {
	if b.sent {
		return 0, errBroken
	}

	b.sent = true

	return copy(p, "abc"), nil
}

// TestReadDeclared covers each way readDeclared can go: exact from a source
// that shows its data is there, grown from one that cannot say, short or
// failing from either, and every way a seeker's account of itself can be
// wrong. Only the last is an error of readDeclared's own making, and only
// where the read position could not be put back.
func TestReadDeclared(t *testing.T) {
	t.Parallel()

	data := make([]byte, 3*growChunk+17) // three doublings from one growChunk
	for i := range data {
		data[i] = byte(i * 7)
	}

	// The source is positioned past a prefix, as Read's is past a header:
	// what remains is counted from there, not from the start.
	prefixed := func() *bytes.Reader {
		r := bytes.NewReader(append([]byte("HEADER"), data...))
		_, _ = r.Seek(6, io.SeekStart)

		return r
	}

	for _, c := range []struct {
		name string
		r    io.Reader
	}{
		{"seekable", prefixed()},
		{"through a BlockReader", NewBlockReader(prefixed())},
		{"stream", struct{ io.Reader }{prefixed()}},
		{"a seeker that claims to be empty", misbehavingSeeker{prefixed(), claimsEmpty}},
		{"a seeker that cannot report its position", misbehavingSeeker{prefixed(), cannotTell}},
		{"a seeker that cannot find its end", misbehavingSeeker{prefixed(), cannotFindEnd}},
	} {
		got, err := readDeclared(c.r, int64(len(data)))
		if err != nil || !bytes.Equal(got, data) {
			t.Errorf("%s: %d bytes, err %v; want the %d bytes declared", c.name, len(got), err, len(data))
		}
	}

	for _, c := range []struct {
		name string
		r    io.Reader
	}{
		{"seekable", bytes.NewReader(data)},
		{"stream", struct{ io.Reader }{bytes.NewReader(data)}},
		{"a seeker that claims more than it holds", misbehavingSeeker{bytes.NewReader(data), claimsMore}},
	} {
		if _, err := readDeclared(c.r, int64(len(data))+1); !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Errorf("%s, one byte short: err %v, want io.ErrUnexpectedEOF", c.name, err)
		}
	}

	if _, err := readDeclared(misbehavingSeeker{bytes.NewReader(data), cannotReturn}, 4); !errors.Is(err, errSeek) {
		t.Errorf("a seeker that cannot be moved back: err %v, want errSeek", err)
	}

	if _, err := readDeclared(&brokenReader{}, 10); !errors.Is(err, errBroken) {
		t.Errorf("a source failing partway: err %v, want errBroken", err)
	}

	if got, err := readDeclared(struct{ io.Reader }{bytes.NewReader(data)}, 0); err != nil || len(got) != 0 {
		t.Errorf("nothing declared: %d bytes, err %v; want none", len(got), err)
	}
}
