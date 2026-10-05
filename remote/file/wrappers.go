package file

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"
)

// The wrappers behind ?prefix= and ?key=, and why they are written out.
//
// Both used to be what io/fs offers: fs.Sub for the prefix, a struct with one
// Open method for the key. Each hid everything astrogo discovers on a
// filesystem by type assertion — CreateFS, CreateExclFS, RemoveFS and
// ContextFS — so a data dir scoped with ?prefix= could not take the download
// lock, and a source reached through either could not be bound to a context,
// which a download requires. Every wrapped configuration failed to download
// (#487), and the tests, which only read through the wrappers, passed.
//
// So each wrapper carries its backend's capabilities, and the rule that
// shapes the code below is that it may never claim one the backend lacks: a
// wrapper reporting ContextFS over a backend that cannot cancel would make
// RequireContext lie about the one thing it exists to check. The concrete
// type is therefore picked from what the backend has, and applyQueryWrappers
// refuses a backend whose capabilities no wrapper reproduces rather than
// handing back a quietly weaker filesystem.

// capability is one of the optional filesystem interfaces, as a bit.
type capability uint8

const (
	capCreate capability = 1 << iota
	capCreateExcl
	capRemove
	capContext

	capWrite = capCreate | capCreateExcl | capRemove
)

// capabilities reports which optional interfaces fsys implements.
func capabilities(fsys fs.FS) capability {
	var c capability

	if _, ok := fsys.(CreateFS); ok {
		c |= capCreate
	}

	if _, ok := fsys.(CreateExclFS); ok {
		c |= capCreateExcl
	}

	if _, ok := fsys.(RemoveFS); ok {
		c |= capRemove
	}

	if _, ok := fsys.(ContextFS); ok {
		c |= capContext
	}

	return c
}

func (c capability) String() string {
	var names []string

	for _, n := range []struct {
		bit  capability
		name string
	}{{capCreate, "CreateFS"}, {capCreateExcl, "CreateExclFS"}, {capRemove, "RemoveFS"}, {capContext, "ContextFS"}} {
		if c&n.bit != 0 {
			names = append(names, n.name)
		}
	}

	if len(names) == 0 {
		return "none"
	}

	return strings.Join(names, ", ")
}

// errCapabilitiesLost is returned by OpenFS when a ?prefix= or ?key= wrapper
// cannot carry what its backend implements.
var errCapabilitiesLost = errors.New("remote/file: the wrapper cannot carry this backend's capabilities")

// prefixed is ?prefix='s filesystem. Reads go through fs.Sub, which already
// scopes them and rewrites error paths; writes join the prefix themselves.
type prefixed struct {
	fs.FS // fs.Sub(under, dir)

	under fs.FS
	dir   string
}

// prefixedCtx is prefixed over a backend that can be bound to a context.
type prefixedCtx struct{ prefixed }

// prefixedRW is prefixed over a backend that can write.
type prefixedRW struct{ prefixed }

// prefixedRWCtx is prefixed over a backend that can write and be bound.
type prefixedRWCtx struct{ prefixedRW }

// scopePrefix returns under scoped to dir, keeping every capability under has
// that a prefixed type exists for. It never claims one under lacks.
func scopePrefix(under fs.FS, dir string) (fs.FS, error) {
	sub, err := fs.Sub(under, dir)
	if err != nil {
		return nil, fmt.Errorf("remote/file: prefix %q: %w", dir, err)
	}

	p := prefixed{FS: sub, under: under, dir: dir}
	c := capabilities(under)

	switch {
	case c&capWrite == capWrite && c&capContext != 0:
		return prefixedRWCtx{prefixedRW{p}}, nil
	case c&capWrite == capWrite:
		return prefixedRW{p}, nil
	case c&capContext != 0:
		return prefixedCtx{p}, nil
	default:
		return p, nil
	}
}

// full joins the prefix onto name, refusing a name fs.Sub would refuse.
func (p prefixed) full(op, name string) (string, error) {
	if !fs.ValidPath(name) {
		return "", &fs.PathError{Op: op, Path: name, Err: fs.ErrInvalid}
	}

	return path.Join(p.dir, name), nil
}

// shorten reports an error from the backend against the caller's name, as
// fs.Sub does for reads.
func (p prefixed) shorten(err error) error {
	if pe, ok := errors.AsType[*fs.PathError](err); ok {
		if rel, ok := strings.CutPrefix(pe.Path, p.dir+"/"); ok {
			pe.Path = rel
		}
	}

	return err
}

func (p prefixedRW) Create(name string) (io.WriteCloser, error) {
	full, err := p.full("create", name)
	if err != nil {
		return nil, err
	}

	w, err := p.under.(CreateFS).Create(full) //nolint:forcetypeassert // scopePrefix picks prefixedRW only over a CreateFS
	if err != nil {
		return nil, p.shorten(err)
	}

	return w, nil
}

func (p prefixedRW) CreateExcl(name string) (io.WriteCloser, error) {
	full, err := p.full("createexcl", name)
	if err != nil {
		return nil, err
	}

	w, err := p.under.(CreateExclFS).CreateExcl(full) //nolint:forcetypeassert // scopePrefix picks prefixedRW only over a CreateExclFS
	if err != nil {
		return nil, p.shorten(err)
	}

	return w, nil
}

func (p prefixedRW) Remove(name string) error {
	full, err := p.full("remove", name)
	if err != nil {
		return err
	}

	return p.shorten(p.under.(RemoveFS).Remove(full)) //nolint:forcetypeassert // scopePrefix picks prefixedRW only over a RemoveFS
}

// withContext scopes the bound backend the same way. A bound filesystem is,
// by ContextFS's contract, the receiver in every respect but the context, so
// it has the same capabilities and fs.Sub cannot refuse a dir it accepted.
func (p prefixed) withContext(ctx context.Context) fs.FS {
	scoped, err := scopePrefix(p.under.(ContextFS).WithContext(ctx), p.dir) //nolint:forcetypeassert // only the Ctx types call this, and scopePrefix picks those only over a ContextFS
	if err != nil {
		return p
	}

	return scoped
}

func (p prefixedCtx) WithContext(ctx context.Context) fs.FS { return p.withContext(ctx) }

func (p prefixedRWCtx) WithContext(ctx context.Context) fs.FS { return p.withContext(ctx) }

// singleObject serves one underlying object under any name the caller asks for.
//
// The alternative — pointing a bucket URL at one object directly — cannot work,
// because the caller still supplies a name and there is nothing for it to mean.
//
// It is read-only whatever the backend can do: every name is the one object,
// so a write through it would replace that object under a name that means
// nothing. It keeps ContextFS, because a source must have it to be downloaded
// from.
type singleObject struct {
	under fs.FS
	key   string
}

// singleObjectCtx is singleObject over a backend that can be bound to a context.
type singleObjectCtx struct{ singleObject }

func newSingleObject(under fs.FS, key string) fs.FS {
	s := singleObject{under: under, key: key}
	if _, ok := under.(ContextFS); ok {
		return singleObjectCtx{s}
	}

	return s
}

func (s singleObject) Open(string) (fs.File, error) {
	f, err := s.under.Open(s.key)
	if err != nil {
		return nil, fmt.Errorf("remote/file: single object %s: %w", s.key, err)
	}

	return f, nil
}

func (s singleObjectCtx) WithContext(ctx context.Context) fs.FS {
	return newSingleObject(s.under.(ContextFS).WithContext(ctx), s.key) //nolint:forcetypeassert // newSingleObject picks singleObjectCtx only over a ContextFS
}
