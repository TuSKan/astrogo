package file

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"path"
	"sort"
	"strings"
	"sync"

	"github.com/TuSKan/astrogo/time"
)

// Registered from init — see the note on localFS's registration.
//
//nolint:gochecknoinits // the registry pattern this package is built on
func init() { Register("mem", openMem) }

// memFS is an in-memory filesystem, for tests and for a caller who wants a
// cache that does not touch a disk.
//
// # Why not fstest.MapFS
//
// MapFS is the right answer for a fixture that is read and never written, and
// it is what a test wanting three files should still reach for. This exists for
// the other case: the write path. astrogo's cache is written as well as read,
// and testing GetFile's download-stage-promote sequence needs a filesystem that
// implements CreateFS — which MapFS, being a map of static entries, does not.
//
// Keyed by URL host, so two "mem://cache" opens share one store and
// "mem://other" is a different one. That is how a test gives two components the
// same cache, or deliberately different ones, without a package-level global.
type memFS struct {
	store *memStore
	// ctx is the context WithContext bound, or nil.
	ctx context.Context //nolint:containedctx // the io/fs contract has nowhere else to put it
}

// memStore is the shared state behind one mem:// host.
type memStore struct {
	mu      sync.RWMutex
	objects map[string]memObject
}

type memObject struct {
	data    []byte
	modTime time.GoTime
}

var (
	memHostsMu sync.Mutex
	memHosts   = map[string]*memStore{}
)

func openMem(u *url.URL) (fs.FS, error) {
	memHostsMu.Lock()
	defer memHostsMu.Unlock()

	host := u.Host

	store, ok := memHosts[host]
	if !ok {
		store = &memStore{objects: map[string]memObject{}}
		memHosts[host] = store
	}

	return &memFS{store: store}, nil
}

// WithContext implements [ContextFS]. Nothing here blocks, so what is honoured
// is the context's state when each operation begins — see [localFS.WithContext]
// for why a backend with nothing to cancel implements this rather than leaving
// RequireContext with an exception.
func (m *memFS) WithContext(ctx context.Context) fs.FS {
	clone := *m
	clone.ctx = ctx

	return &clone
}

// Open implements [fs.FS].
func (m *memFS) Open(name string) (fs.File, error) {
	if err := m.check("open", name); err != nil {
		return nil, err
	}

	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}

	m.store.mu.RLock()
	defer m.store.mu.RUnlock()

	if obj, ok := m.store.objects[name]; ok {
		return &memFile{
			Reader: bytes.NewReader(obj.data),
			info:   memInfo{name: path.Base(name), size: int64(len(obj.data)), modTime: obj.modTime},
		}, nil
	}

	// A directory exists if anything is under it. Nothing stores directories:
	// a key prefix is all a directory ever is here, which is also true of every
	// object store this package talks to.
	if name == "." || m.hasPrefixLocked(name+"/") {
		return &memDir{fsys: m, name: name}, nil
	}

	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}

// Stat implements [fs.StatFS].
func (m *memFS) Stat(name string) (fs.FileInfo, error) {
	if err := m.check("stat", name); err != nil {
		return nil, err
	}

	f, err := m.Open(name)
	if err != nil {
		return nil, err
	}

	defer func() { _ = f.Close() }()

	return f.Stat() //nolint:wrapcheck // memFile and memDir never fail here
}

// ReadDir implements [fs.ReadDirFS].
func (m *memFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if err := m.check("readdir", name); err != nil {
		return nil, err
	}

	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrInvalid}
	}

	prefix := ""
	if name != "." {
		prefix = name + "/"
	}

	m.store.mu.RLock()
	defer m.store.mu.RUnlock()

	seen := map[string]fs.DirEntry{}

	for key, obj := range m.store.objects {
		if !strings.HasPrefix(key, prefix) {
			continue
		}

		rest := key[len(prefix):]
		if rest == "" {
			continue
		}

		if child, _, nested := strings.Cut(rest, "/"); nested {
			seen[child] = memInfo{name: child, dir: true}

			continue
		}

		seen[rest] = memInfo{name: rest, size: int64(len(obj.data)), modTime: obj.modTime}
	}

	if len(seen) == 0 && name != "." {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
	}

	out := make([]fs.DirEntry, 0, len(seen))
	for _, e := range seen {
		out = append(out, e)
	}

	// fs.ReadDir promises directory order, and fstest.TestFS checks it.
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })

	return out, nil
}

// Create implements [CreateFS]. The object appears only on Close, matching the
// local backend, so a reader never sees half a write.
func (m *memFS) Create(name string) (io.WriteCloser, error) {
	if err := m.check("create", name); err != nil {
		return nil, err
	}

	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "create", Path: name, Err: fs.ErrInvalid}
	}

	return &memWrite{fsys: m, name: name}, nil
}

// CreateExcl implements [CreateExclFS].
//
// The object is reserved empty under the store's lock and the write appends to
// it, rather than appearing on Close the way [memFS.Create] does. That is the
// same choice the local backend makes and for the same reason: a lock that
// became visible only when the holder finished writing would not exclude
// anybody. See [localFS.CreateExcl].
func (m *memFS) CreateExcl(name string) (io.WriteCloser, error) {
	if err := m.check("createexcl", name); err != nil {
		return nil, err
	}

	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "createexcl", Path: name, Err: fs.ErrInvalid}
	}

	m.store.mu.Lock()
	defer m.store.mu.Unlock()

	if _, ok := m.store.objects[name]; ok {
		return nil, &fs.PathError{Op: "createexcl", Path: name, Err: fs.ErrExist}
	}

	m.store.objects[name] = memObject{modTime: time.Now()}

	return &memAppend{fsys: m, name: name}, nil
}

// memAppend writes into an object that already exists, in place.
type memAppend struct {
	fsys *memFS
	name string
}

func (w *memAppend) Write(p []byte) (int, error) {
	w.fsys.store.mu.Lock()
	defer w.fsys.store.mu.Unlock()

	obj := w.fsys.store.objects[w.name]
	obj.data = append(obj.data, p...)
	obj.modTime = time.Now()
	w.fsys.store.objects[w.name] = obj

	return len(p), nil
}

func (w *memAppend) Close() error { return nil }

// Abort implements [AbortWriter]: remove the object the exclusive create
// reserved, so an abandoned lock attempt leaves nothing behind.
func (w *memAppend) Abort() error {
	w.fsys.store.mu.Lock()
	defer w.fsys.store.mu.Unlock()

	delete(w.fsys.store.objects, w.name)

	return nil
}

// Remove implements [RemoveFS].
func (m *memFS) Remove(name string) error {
	if err := m.check("remove", name); err != nil {
		return err
	}

	if !fs.ValidPath(name) {
		return &fs.PathError{Op: "remove", Path: name, Err: fs.ErrInvalid}
	}

	m.store.mu.Lock()
	defer m.store.mu.Unlock()

	if _, ok := m.store.objects[name]; !ok {
		return &fs.PathError{Op: "remove", Path: name, Err: fs.ErrNotExist}
	}

	delete(m.store.objects, name)

	return nil
}

// check reports the bound context's error, if it has one.
func (m *memFS) check(op, name string) error {
	if m.ctx == nil {
		return nil
	}

	if err := m.ctx.Err(); err != nil {
		return &fs.PathError{Op: op, Path: name, Err: err}
	}

	return nil
}

func (m *memFS) hasPrefixLocked(prefix string) bool {
	for k := range m.store.objects {
		if strings.HasPrefix(k, prefix) {
			return true
		}
	}

	return false
}

// memWrite buffers until Close.
type memWrite struct {
	fsys *memFS
	name string
	buf  bytes.Buffer
}

func (w *memWrite) Write(p []byte) (int, error) {
	n, err := w.buf.Write(p)
	if err != nil {
		return n, fmt.Errorf("remote/file: buffer %s: %w", w.name, err)
	}

	return n, nil
}

// Abort implements [AbortWriter]: drop the buffer, publish nothing.
func (w *memWrite) Abort() error {
	w.buf.Reset()

	return nil
}

func (w *memWrite) Close() error {
	w.fsys.store.mu.Lock()
	defer w.fsys.store.mu.Unlock()

	w.fsys.store.objects[w.name] = memObject{
		data:    bytes.Clone(w.buf.Bytes()),
		modTime: time.Now(),
	}

	return nil
}

// memFile is an open object. *bytes.Reader supplies Read, ReadAt and Seek, so
// this satisfies [File] without writing any of them.
type memFile struct {
	*bytes.Reader

	info memInfo
}

func (f *memFile) Stat() (fs.FileInfo, error) { return f.info, nil }
func (f *memFile) Close() error               { return nil }

// memDir is an open directory, with a cursor.
//
// The cursor is the part worth having: fs.ReadDirFile's n > 0 form is a
// paginating read, so it must return the NEXT n entries and remember how far it
// got. An implementation that returns the first n every time makes a caller
// looping until io.EOF loop for ever, and fstest.TestFS catches it by comparing
// a single ReadDir(-1) against a ReadDir(1), ReadDir(2), ... walk.
type memDir struct {
	fsys    *memFS
	name    string
	entries []fs.DirEntry
	pos     int
	loaded  bool
}

func (d *memDir) Stat() (fs.FileInfo, error) {
	return memInfo{name: path.Base(d.name), dir: true}, nil
}

func (d *memDir) Read([]byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: d.name, Err: fs.ErrInvalid}
}

func (d *memDir) Close() error { return nil }

// ReadDir implements [fs.ReadDirFile].
//
// The two exhaustion behaviours differ and fstest checks both: with n > 0 an
// exhausted directory reports io.EOF, and with n <= 0 it reports an empty slice
// and a nil error.
func (d *memDir) ReadDir(n int) ([]fs.DirEntry, error) {
	if !d.loaded {
		entries, err := d.fsys.ReadDir(d.name)
		if err != nil {
			return nil, err
		}

		d.entries = entries
		d.loaded = true
	}

	remaining := d.entries[d.pos:]

	if n <= 0 {
		d.pos = len(d.entries)

		return remaining, nil
	}

	if len(remaining) == 0 {
		return nil, io.EOF
	}

	if n > len(remaining) {
		n = len(remaining)
	}

	d.pos += n

	return remaining[:n], nil
}

// memInfo is both a [fs.FileInfo] and a [fs.DirEntry], which saves a wrapper
// type and is what fstest compares against itself.
type memInfo struct {
	name    string
	size    int64
	modTime time.GoTime
	dir     bool
}

func (i memInfo) Name() string { return i.name }
func (i memInfo) Size() int64  { return i.size }

func (i memInfo) Mode() fs.FileMode {
	if i.dir {
		return fs.ModeDir | 0o555
	}

	return 0o444
}

func (i memInfo) ModTime() time.GoTime       { return i.modTime }
func (i memInfo) IsDir() bool                { return i.dir }
func (i memInfo) Sys() any                   { return nil }
func (i memInfo) Type() fs.FileMode          { return i.Mode().Type() }
func (i memInfo) Info() (fs.FileInfo, error) { return i, nil }
