package file

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"sort"
	"sync"
)

// Errors this layer raises on its own behalf. Everything else a caller sees
// comes from the backend, and the one error worth matching on across all of
// them — "it is not there" — is [fs.ErrNotExist], which every backend here
// returns and which errors.Is already understands.
var (
	// ErrNoScheme indicates a URL whose scheme no backend has registered.
	//
	// Almost always a missing blank import: s3:// needs remote/file/s3, which
	// exists to be imported for its side effect and exports nothing at all.
	ErrNoScheme = errors.New("remote/file: no backend registered for this URL scheme")

	// ErrNoContext indicates a filesystem that cannot be cancelled being used
	// where cancellation is required. See [ContextFS].
	ErrNoContext = errors.New("remote/file: backend does not support cancellation")

	// ErrReadOnly indicates a write to a filesystem that does not implement
	// [CreateFS].
	ErrReadOnly = errors.New("remote/file: backend is read-only")

	// ErrNoPath indicates a file:// URL with nothing to open.
	ErrNoPath = errors.New("remote/file: file URL has no path")

	// ErrDiscarded indicates a staged write thrown away because an earlier
	// Write failed. The object it would have replaced is untouched.
	ErrDiscarded = errors.New("remote/file: staged write discarded after a failed write")
)

// Opener builds a filesystem from a parsed URL.
//
// The whole URL is handed over, query string included, because that is where
// scheme-specific connection detail lives — region, endpoint, path-style
// addressing — and keeping it in the URL is what lets astrogo reach a non-AWS
// S3 service with no astrogo configuration API and no AWS type in any
// signature.
type Opener func(u *url.URL) (fs.FS, error)

var (
	registryMu sync.RWMutex
	openers    = map[string]Opener{}
	opened     = map[string]fs.FS{}
)

// Register associates a URL scheme with an opener.
//
// Called from a backend package's init. Registering a scheme twice panics,
// because two backends answering for one scheme is a build-time mistake and the
// alternative is a program whose storage depends on import order.
//
// This is the whole of scheme dispatch. There is no switch on scheme anywhere
// in this package and no Backend interface beyond [fs.FS]: adding SFTP means
// adding a package that calls this, and editing nothing here.
func Register(scheme string, open Opener) {
	registryMu.Lock()
	defer registryMu.Unlock()

	if _, dup := openers[scheme]; dup {
		panic("remote/file: scheme " + scheme + " registered twice")
	}

	openers[scheme] = open
}

// Schemes returns every registered scheme, sorted.
//
// For diagnostics: an unregistered scheme is nearly always a missing blank
// import, and the fix is far more obvious when the error can list what IS
// registered.
func Schemes() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()

	out := make([]string, 0, len(openers))
	for s := range openers {
		out = append(out, s)
	}

	sort.Strings(out)

	return out
}

// OpenFS resolves a URL to a filesystem.
//
// One filesystem is built per distinct URL and reused for the life of the
// process, which is what the cache and source handles want: an fs.FS here is a
// configuration value rather than a connection, safe to share, and rebuilding
// one per fetch would re-resolve credentials on every call.
//
// Per-call context does not go here — see [ContextFS]. The shared value is
// context-free by construction, and a caller binds its own with [WithContext]
// or [RequireContext], so one registered filesystem serves any number of
// concurrent callers with different deadlines.
//
// # Two portable query parameters
//
// Handled here rather than in each backend, so every scheme gets them and no
// backend has to remember:
//
//   - ?prefix=sub/dir/ scopes the filesystem to that subtree, via fs.Sub.
//   - ?key=exact/object.dat serves one object under whatever name the caller
//     asks for. It is the supported way to point an endpoint at a single file,
//     since a bare single-object URL leaves no room for a name.
func OpenFS(rawURL string) (fs.FS, error) {
	registryMu.RLock()

	cached, hit := opened[rawURL]

	registryMu.RUnlock()

	if hit {
		return cached, nil
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("remote/file: parse %q: %w", rawURL, err)
	}

	registryMu.RLock()

	open, known := openers[u.Scheme]

	registryMu.RUnlock()

	if !known {
		return nil, fmt.Errorf("remote/file: %q: %w (registered: %v)",
			u.Scheme, ErrNoScheme, Schemes())
	}

	fsys, err := open(u)
	if err != nil {
		return nil, fmt.Errorf("remote/file: open %s: %w", redact(u), err)
	}

	if fsys, err = applyQueryWrappers(u, fsys); err != nil {
		return nil, err
	}

	registryMu.Lock()

	// Another goroutine may have won the race; keep whichever is already
	// there so every caller of one URL gets one value.
	if existing, dup := opened[rawURL]; dup {
		fsys = existing
	} else {
		opened[rawURL] = fsys
	}

	registryMu.Unlock()

	return fsys, nil
}

// applyQueryWrappers implements ?prefix= and ?key=.
func applyQueryWrappers(u *url.URL, fsys fs.FS) (fs.FS, error) {
	q := u.Query()

	if prefix := q.Get("prefix"); prefix != "" {
		// fs.Sub wants a valid path: no leading or trailing slash, no dot
		// elements. Trimming the trailing slash is the one accommodation,
		// because writing a prefix with one is the natural thing to do.
		sub, err := fs.Sub(fsys, trimSlashes(prefix))
		if err != nil {
			return nil, fmt.Errorf("remote/file: prefix %q: %w", prefix, err)
		}

		fsys = sub
	}

	if key := q.Get("key"); key != "" {
		fsys = singleObject{under: fsys, key: trimSlashes(key)}
	}

	return fsys, nil
}

// singleObject serves one underlying object under any name the caller asks for.
//
// The alternative — pointing a bucket URL at one object directly — cannot work,
// because the caller still supplies a name and there is nothing for it to mean.
type singleObject struct {
	under fs.FS
	key   string
}

func (s singleObject) Open(string) (fs.File, error) {
	f, err := s.under.Open(s.key)
	if err != nil {
		return nil, fmt.Errorf("remote/file: single object %s: %w", s.key, err)
	}

	return f, nil
}

func trimSlashes(s string) string {
	for len(s) > 0 && s[0] == '/' {
		s = s[1:]
	}

	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}

	return s
}

// redact removes userinfo from a URL before it reaches an error message.
//
// sftp:// URLs carry credentials, and an error string ends up in logs, issue
// reports and CI output. url.URL.Redacted replaces only the password; the
// username is worth keeping since it is diagnostic, which is why this is
// Redacted rather than something stricter.
func redact(u *url.URL) string { return u.Redacted() }
