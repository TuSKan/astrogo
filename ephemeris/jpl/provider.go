package jpl

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/jpl/lsk"
	"github.com/TuSKan/astrogo/ephemeris/jpl/spk"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// Sentinel errors for JPL provider.
var (
	ErrNotImplemented        = errors.New("jpl: source not implemented")
	ErrUnknownSource         = errors.New("jpl: unknown source")
	ErrRecursionDepth        = errors.New("jpl: recursion depth exceeded")
	ErrNilKernel             = errors.New("jpl: kernel is nil")
	ErrKernelIndexOutOfRange = errors.New("jpl: kernel index out of range")

	// ErrUnsupportedFrame refuses a kernel holding a segment in a reference
	// frame other than NAIF 1 (J2000), whose vectors would otherwise be summed
	// into the ICRF chain unrotated (#813).
	ErrUnsupportedFrame = errors.New("jpl: segment in a reference frame other than J2000")

	// ErrNoSmallBodyKernel indicates Horizons matched no small body for the
	// requested designation, so the provider would carry only its planetary
	// base kernel.
	//
	// It used to succeed. spk.CacheAPI returns an empty slice rather than an
	// error when nothing matches, the loop over it did nothing, and the
	// caller got a provider back with a nil error — for "Ceres", for
	// "101955", and for "totally-not-a-body-xyz" alike. SupportedBodies then
	// listed the eleven planetary bodies from the base kernel, which is
	// exactly what a working provider looks like from the outside.
	//
	// Horizons wants its own designation syntax for small bodies: "433" and
	// "433;" resolve to Eros while the name does not, and "DES=433;"
	// resolves to a different object entirely. Getting that wrong is easy;
	// getting it wrong silently is what this refuses.
	ErrNoSmallBodyKernel = errors.New("jpl: no small-body kernel for designation")

	// ErrWrongSmallBody is returned when Horizons answers with a body that
	// is not the one asked for.
	//
	// The load succeeds in every mechanical sense — a kernel arrives, a
	// segment parses, a new body appears — and the object is simply somebody
	// else. Asking for "1" returns comet 1000036 rather than 1 Ceres,
	// because a bare number resolves against the major-body and comet
	// indices first; "DES=433;" returns 248370 rather than Eros. Both are
	// quieter than a failure and worse.
	ErrWrongSmallBody = errors.New("jpl: horizons returned a different small body")
)

// kmPerAU is the number of kilometers in one Astronomical Unit.
//
// Unexported. It was KMPerAU, which let a caller outside this module pin a
// copy of the value that would not move if constants ever did — the precise
// hazard routing everything through constants exists to remove. Nothing
// outside this file ever referenced it.
//
// var, not const: constants.IAU.AstronomicalUnit is a struct, and Go does not
// permit selecting a struct field inside a constant expression.
var kmPerAU = constants.IAU.AstronomicalUnit.Value / 1e3

// naifJ2000 is NAIF's integer code for the "J2000" inertial frame, the only
// frame a segment may be in to join this provider's chains (see
// addKernelLocked).
const naifJ2000 = 1

// ErrNoSegment is returned when the loaded kernels hold the target but no
// segment of it covers the requested epoch.
var ErrNoSegment = errors.New("jpl: no coverage for target at requested epoch")

// ErrUnknownTarget is returned when no loaded kernel holds the target at all:
// the Pluto centre (999) from a planetary kernel, which has only the Pluto
// barycenter (9). It used to be reported as ErrNoSegment, as if the kernel
// covered the body at other epochs (#809).
var ErrUnknownTarget = errors.New("jpl: no loaded kernel holds the target")

// Kernel represents a single SPK file and its metadata.
type Kernel struct {
	Reader   *spk.Reader
	Segments []spk.Segment
	// Path is the kernel file's location on disk — set by NewProvider,
	// AddKernelFile, and Open. Empty only for a kernel added via the bare
	// AddKernel(r *spk.Reader), since that call has no path to record
	// (per-object small-body kernels obtained via NewProvider's
	// spk.CacheAPI recursion currently fall into this case too).
	Path string
}

// SegmentRef indexes a segment within a specific kernel.
type SegmentRef struct {
	KernelIndex  int
	SegmentIndex int
}

// TargetCoverage stores summary coverage info per plan.
type TargetCoverage struct {
	StartET float64
	EndET   float64
	Count   int
}

// Provider implements core.Provider using JPL SPK/LSK kernels.
//
// mu guards Kernels, Index, ByTarget, and ByTargetCoverage: AddKernel (and
// Close) mutate them, and State/FindSegment/SupportedBodies read them, so a
// Provider that has AddKernel called after construction — while other
// goroutines are concurrently querying it — needs this to avoid a data race.
type Provider struct {
	mu               sync.RWMutex
	startTime        time.Time
	endTime          time.Time
	LSK              *lsk.Reader
	ByTarget         map[int32][]SegmentRef
	ByTargetCoverage map[int32]TargetCoverage
	source           core.Source
	kernel           string
	Kernels          []*Kernel
	Index            []SegmentRef
}

// Option configures a Provider.
type Option func(*Provider)

// WithTimeInterval sets the time interval for which the provider is valid.
func WithTimeInterval(start, end time.Time) Option {
	return func(p *Provider) {
		p.startTime = start
		p.endTime = end
	}
}

// NewProvider creates a new JPL ephemeris provider.
//
// The source selects the kind of JPL data (Planets, SmallBody, Asteroids,
// Comets). The kernel identifies the specific dataset (e.g. "de442", "433").
func NewProvider(ctx context.Context, source core.Source, kernel string, opts ...Option) (*Provider, error) {
	p := &Provider{
		ByTarget:         make(map[int32][]SegmentRef),
		ByTargetCoverage: make(map[int32]TargetCoverage),
		source:           source,
		kernel:           kernel,
	}
	for _, opt := range opts {
		opt(p)
	}

	var err error

	switch p.source {
	case core.Planets:
		if p.kernel == "" {
			p.kernel = "de440"
		}

		spkKey := "planets/" + p.kernel + ".bsp"

		k, err := spk.CacheDownload(ctx, spkKey)
		if err != nil {
			return nil, fmt.Errorf("jpl: failed to load planetary kernel: %w", err)
		}

		if err := p.addKernelPath(k, spkKey); err != nil {
			return nil, fmt.Errorf("jpl: failed to load planetary kernel: %w", err)
		}
	case core.Asteroids, core.Comets, core.SmallBody:
		// Always load a minimal planetary kernel for recursion (center resolution)
		const baseKey = "planets/de440s.bsp"

		pk, err := spk.CacheDownload(ctx, baseKey)
		if err != nil {
			return nil, fmt.Errorf("jpl: failed to load planetary base kernel: %w", err)
		}

		if err := p.addKernelPath(pk, baseKey); err != nil {
			return nil, fmt.Errorf("jpl: failed to add planetary base kernel: %w", err)
		}

		// Horizons-generated kernels land in the same cache as downloaded
		// ones, under a per-source prefix — wherever remote's data
		// directory points, local or not.
		cacheBucket, prefix, err := remote.CacheDir(ctx, remote.NAIFSPK)
		if err != nil {
			return nil, fmt.Errorf("jpl: resolve kernel cache: %w", err)
		}

		spkReaders, err := spk.CacheAPI(ctx, cacheBucket, prefix+string(p.source)+"/", p.kernel, p.startTime, p.endTime)
		if err != nil {
			return nil, fmt.Errorf("jpl: failed to get SPK files: %w", err)
		}

		if err := p.loadSmallBodyKernels(spkReaders); err != nil {
			return nil, err
		}
	case core.Moons:
		// Unlike Asteroids/Comets/SmallBody, a planetary satellite kernel
		// needs no separate planetary base kernel for center resolution —
		// NAIF's own satellite SPKs (e.g. jup365.bsp, sat441.bsp) already
		// include the Sun, Earth, and the relevant planet barycenter
		// directly relative to the Solar System Barycenter, confirmed
		// against NAIF's own published segment summaries
		// (naif.jpl.nasa.gov/pub/naif/generic_kernels/spk/satellites/aa_summaries.txt).
		if p.kernel == "" {
			return nil, fmt.Errorf("%w: a satellite kernel name is required (e.g. \"sat441\")", ErrUnknownSource)
		}

		spkKey := "satellites/" + p.kernel + ".bsp"

		k, err := spk.CacheDownload(ctx, spkKey)
		if err != nil {
			return nil, fmt.Errorf("jpl: failed to load satellite kernel: %w", err)
		}

		if err := p.addKernelPath(k, spkKey); err != nil {
			return nil, fmt.Errorf("jpl: failed to load satellite kernel: %w", err)
		}
	case core.Satellites:
		return nil, fmt.Errorf("%w: satellites", ErrNotImplemented)
	case core.Stations:
		return nil, fmt.Errorf("%w: stations", ErrNotImplemented)
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnknownSource, p.source)
	}

	p.LSK, err = lsk.Cache(ctx, "lsk/naif0012.tls")
	if err != nil {
		return nil, fmt.Errorf("jpl: failed to locate/cache LSK: %w", err)
	}

	// The kernel now drives this provider's epochs, so its DELTA_AT table is
	// the record in force — publish it to time so UTC↔TAI↔TT agree with the
	// ET the SPK is evaluated at, rather than following a leap-second table
	// pinned by go.mod.
	//
	// Fatal, for the same reason the LSK load above is: a kernel whose table
	// contradicts the built-in record is a broken kernel, and continuing means
	// computing positions from a clock that is a whole second wrong somewhere
	// without saying where.
	if err := lsk.RegisterLeapSeconds(p.LSK); err != nil {
		return nil, fmt.Errorf("jpl: %w", err)
	}

	return p, nil
}

// Close releases all kernel resources.
func (p *Provider) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	var lastErr error

	for _, k := range p.Kernels {
		err := k.Reader.Close()
		if err != nil {
			lastErr = err
		}
	}

	if p.LSK != nil {
		err := p.LSK.Close()
		if err != nil {
			lastErr = err
		}
	}

	return lastErr
}

// State returns the geocentric state (position and velocity) of the given
// body at time t.
func (p *Provider) State(id core.ID, t time.Time) (core.State, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Straight to ET seconds: going via a summed Julian Date quantises the
	// result to about 40 microseconds, which is 4 cm of lunar motion (#150).
	et := lsk.UTCToET(t, p.LSK)

	// Get target relative to SSB (0)
	// An ID is a NAIF code (#809): the target is looked up as itself, with no
	// table between, so a code no loaded kernel holds is refused rather than
	// read as some other body.
	targetSSB, err := p.evaluateRecursive(int32(id), et, 0)
	if err != nil {
		return core.State{}, err
	}

	// Get Earth relative to SSB (0)
	earthSSB, err := p.evaluateRecursive(399, et, 0)
	if err != nil {
		return core.State{}, fmt.Errorf("jpl: failed to get Earth state: %w", err)
	}

	// Geocentric = Target(SSB) - Earth(SSB)
	relPos := targetSSB.Pos.Sub(earthSSB.Pos)
	relVel := targetSSB.Vel.Sub(earthSSB.Vel)

	// Convert to AU and AU/day
	return core.State{
		Pos: vector.Vec3{
			X: relPos.X / kmPerAU,
			Y: relPos.Y / kmPerAU,
			Z: relPos.Z / kmPerAU,
		},
		Vel: vector.Vec3{
			X: relVel.X * 86400 / kmPerAU,
			Y: relVel.Y * 86400 / kmPerAU,
			Z: relVel.Z * 86400 / kmPerAU,
		},
		Frame:  core.FrameICRS,
		Center: core.CenterGeocenter,
	}, nil
}

// AddKernel opens an SPK file and adds its segments to the provider index.
//
// A kernel that cannot be added — its summaries do not parse, or one of its
// segments is in a frame other than J2000 (ErrUnsupportedFrame) — is not
// added, and stays the caller's to close.
func (p *Provider) AddKernel(k *spk.Reader) error {
	if k == nil {
		return ErrNilKernel
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	return p.addKernelLocked(k, "")
}

// AddKernelFrom opens the SPK object at bucket/key and adds it to the
// provider index, recording key for LoadedKernels/RemoveKernel. No network
// access and no download consent: the object must already be there.
func (p *Provider) AddKernelFrom(ctx context.Context, fsys remote.FS, key string) error {
	ra, err := remote.Open(ctx, fsys, key)
	if err != nil {
		return fmt.Errorf("jpl: open kernel %s: %w", key, err)
	}

	r, err := spk.NewReader(ra)
	if err != nil {
		return errors.Join(fmt.Errorf("jpl: read kernel %s: %w", key, err), ra.Close())
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// A kernel this call opened and then could not add is closed here: the
	// caller never held it.
	if err := p.addKernelLocked(r, key); err != nil {
		return errors.Join(err, r.Close())
	}

	return nil
}

// RemoveKernel closes and unloads the kernel at index i (as positioned in
// p.Kernels — see LoadedKernels) and rebuilds the segment index. Removing
// shifts the indices of every kernel after i down by one; re-fetch indices
// via LoadedKernels before removing more than one kernel.
func (p *Provider) RemoveKernel(i int) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if i < 0 || i >= len(p.Kernels) {
		return fmt.Errorf("%w: index %d, have %d kernels", ErrKernelIndexOutOfRange, i, len(p.Kernels))
	}

	if err := p.Kernels[i].Reader.Close(); err != nil {
		return fmt.Errorf("jpl: close kernel: %w", err)
	}

	p.Kernels = append(p.Kernels[:i], p.Kernels[i+1:]...)
	p.rebuildIndexLocked()

	return nil
}

// UnloadAll closes every loaded kernel reader and clears the segment
// index, leaving the Provider empty but reusable via
// AddKernel/AddKernelFrom. The LSK reader (if set) is left untouched —
// pair with Close to also release it.
func (p *Provider) UnloadAll() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	var errs []error

	for _, k := range p.Kernels {
		if err := k.Reader.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	p.Kernels = nil
	p.rebuildIndexLocked()

	return errors.Join(errs...)
}

// KernelInfo summarizes one loaded kernel for inspection — e.g. a setup
// UI or diagnostic log listing what a Provider currently has loaded.
type KernelInfo struct {
	// Key is the fsys key the kernel was loaded from.
	Key      string
	Segments int
	StartET  float64
	EndET    float64
}

// LoadedKernels reports what is currently loaded, in p.Kernels order —
// indices here match RemoveKernel's i parameter.
func (p *Provider) LoadedKernels() []KernelInfo {
	p.mu.RLock()
	defer p.mu.RUnlock()

	infos := make([]KernelInfo, len(p.Kernels))

	for i, k := range p.Kernels {
		info := KernelInfo{Key: k.Path, Segments: len(k.Segments)}

		for j, seg := range k.Segments {
			if j == 0 || seg.StartET < info.StartET {
				info.StartET = seg.StartET
			}

			if j == 0 || seg.EndET > info.EndET {
				info.EndET = seg.EndET
			}
		}

		infos[i] = info
	}

	return infos
}

// Open constructs a Provider from kernels already present in fsys — no
// network access, no download consent. This is the offline path: pre-seed
// the objects yourself (files a prior consented run cached, or copied into
// a deployment image) and open them by key. fsys may be backed by
// anything remote/file can open, not just local disk.
func Open(ctx context.Context, fsys remote.FS, lskKey string, spkKeys ...string) (*Provider, error) {
	p := &Provider{
		ByTarget:         make(map[int32][]SegmentRef),
		ByTargetCoverage: make(map[int32]TargetCoverage),
	}

	for _, key := range spkKeys {
		if err := p.AddKernelFrom(ctx, fsys, key); err != nil {
			// The kernels already added are closed with it, so a failed Open
			// leaves nothing open behind the error.
			return nil, errors.Join(fmt.Errorf("jpl: open: %w", err), p.Close())
		}
	}

	f, err := fsys.Open(lskKey)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("jpl: open LSK %s: %w", lskKey, err), p.Close())
	}

	// lsk.NewReader takes ownership of f and stores it, so Provider.Close
	// is what closes it — matching lsk.Cache's own contract. Closing it
	// here as well would double-close the reader.
	l, err := lsk.NewReader(f)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("jpl: read LSK %s: %w", lskKey, err), f.Close(), p.Close())
	}

	p.LSK = l

	return p, nil
}

// FindSegment finds the appropriate segment for a target at a given time.
func (p *Provider) FindSegment(target int32, et float64) (*SegmentRef, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	ref, err := p.findSegmentLocked(target, et)
	if err != nil {
		return nil, err
	}

	return &ref, nil
}

// SupportedBodies returns every target the loaded kernels hold, as the NAIF
// codes they hold them under, sorted.
func (p *Provider) SupportedBodies() []core.ID {
	p.mu.RLock()
	defer p.mu.RUnlock()

	res := make([]core.ID, 0, len(p.ByTarget))

	for targetID := range p.ByTarget {
		res = append(res, core.ID(targetID))
	}

	slices.Sort(res)

	return res
}

// loadSmallBodyKernels adds the fetched kernels and reports
// [ErrNoSmallBodyKernel] when none of them contributed a body.
//
// What the caller asked for is a body, so that is what is checked for — not
// whether a file arrived. Counting readers is not enough: Horizons answers
// some designations with a kernel carrying no small-body segment at all
// ("1;" and "4;" both do), so a non-empty slice can still leave the provider
// with nothing but its planetary base. Comparing the body set before and
// after states the actual requirement.
func (p *Provider) loadSmallBodyKernels(readers []*spk.Reader) error {
	before := len(p.SupportedBodies())

	for _, reader := range readers {
		if err := p.AddKernel(reader); err != nil {
			return fmt.Errorf("jpl: failed to load small-body kernel: %w", err)
		}
	}

	after := p.SupportedBodies()

	if len(after) == before {
		return fmt.Errorf("%w: %q added no body to the %d already in the planetary "+
			"base kernel; Horizons matches small bodies by number (\"433\") or by its own "+
			"designation syntax (\"433;\"), not by name",
			ErrNoSmallBodyKernel, p.kernel, before)
	}

	return p.verifyRequestedBodyLoaded(after)
}

// numberedAsteroid reports the body a designation names, and whether it names
// a predictable one at all. "433" and "433;" both name Eros; a provisional
// designation or a comet fragment names nothing predictable.
//
// A bare integer is not enough to make it an asteroid number. Horizons'
// comet index issues SPK-IDs that are also bare integers — 1000390 for
// C/2002 J5 (LINEAR) — and they sit outside NAIF's numbered-asteroid
// allocation entirely. [core.SmallBodyID] is the authority on that bound and
// reports false for anything beyond it, so it is asked rather than having its
// constant copied here, where the two could drift apart without either side
// noticing.
func numberedAsteroid(designation string) (core.ID, bool) {
	n, err := strconv.Atoi(strings.TrimSuffix(designation, ";"))
	if err != nil {
		return 0, false
	}

	return core.SmallBodyID(n)
}

// verifyRequestedBodyLoaded checks that the body loaded is the body asked
// for, when the designation says which one that is.
//
// A bare number is ambiguous to Horizons: it resolves against the major-body
// and comet indices before the numbered-asteroid record, so asking for "1"
// returns comet 1000036 rather than 1 Ceres — and the load succeeds, because
// a body genuinely was added. The caller then works with a different object
// entirely, which is the same failure as "DES=433;" resolving to 248370
// rather than Eros.
//
// A numbered asteroid has a knowable identifier, so this is checkable rather
// than a matter of trusting the syntax: asteroid n must arrive as
// [core.SmallBodyID](n). A designation that does not name one — a comet
// fragment, a provisional designation, or a Horizons SPK-ID from the comet
// index — names nothing this can predict and is left alone.
//
// That last case is why the check asks [numberedAsteroid] for a body rather
// than for a number. Composing the two by hand read as
// slices.Contains(loaded, core.SmallBodyID(n)), which for a comet SPK-ID
// compares against SmallBodyID's out-of-range zero: the body genuinely
// requested is present, ID 0 is not, and the guard reported the loudest wrong
// answer it has — that Horizons substituted a different object — for a load
// that was entirely correct.
func (p *Provider) verifyRequestedBodyLoaded(loaded []core.ID) error {
	want, ok := numberedAsteroid(p.kernel)
	if !ok {
		return nil
	}

	if slices.Contains(loaded, want) {
		return nil
	}

	return fmt.Errorf("%w: asked for %q and Horizons returned %v, which does not include %s; "+
		"a bare number resolves against the major-body and comet indices before the "+
		"numbered-asteroid record, so the object loaded is not the one requested",
		ErrWrongSmallBody, p.kernel, loaded, want)
}

// addKernelPath is AddKernel with a known file path, used internally by
// NewProvider so its kernels show up correctly in LoadedKernels.
func (p *Provider) addKernelPath(k *spk.Reader, path string) error {
	if k == nil {
		return ErrNilKernel
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// addKernelPath adds kernels NewProvider opened, so one it cannot add is
	// closed here rather than left open behind the returned error.
	if err := p.addKernelLocked(k, path); err != nil {
		return errors.Join(err, k.Close())
	}

	return nil
}

// addKernelLocked is AddKernel/AddKernelFile/addKernelPath's shared body.
// Callers must already hold p.mu.
func (p *Provider) addKernelLocked(k *spk.Reader, path string) error {
	summaries, err := k.ReadSummaries()
	if err != nil {
		return fmt.Errorf("jpl: failed to read summaries: %w", err)
	}

	segments := make([]spk.Segment, 0, len(summaries))
	for _, s := range summaries {
		// Every state this provider returns is labeled ICRF and every chain is
		// a plain sum of segment vectors, so a segment is usable only in the
		// frame that sum assumes: NAIF frame 1, "J2000", which for the DE
		// kernels and every kernel astrogo fetches is the ICRF (NAIF Frames
		// Required Reading). The frame used to be parsed and never read, so a
		// caller's ECLIPJ2000 kernel (NAIF 17) was chained 23.4° off and still
		// labeled ICRS (#813). A kernel holding any other frame is refused
		// whole: half a kernel loaded is a provider that answers some targets
		// and silently not others.
		if frame := s.Integers[2]; frame != naifJ2000 {
			return fmt.Errorf("%w: the segment for target %d is in NAIF frame %d", ErrUnsupportedFrame, s.Integers[0], frame)
		}

		segments = append(segments, spk.Segment{
			StartET:   s.Doubles[0],
			EndET:     s.Doubles[1],
			Target:    s.Integers[0],
			Center:    s.Integers[1],
			Frame:     s.Integers[2],
			Type:      s.Integers[3],
			StartAddr: s.Integers[4],
			EndAddr:   s.Integers[5],
		})
	}

	p.Kernels = append(p.Kernels, &Kernel{Reader: k, Segments: segments, Path: path})
	p.rebuildIndexLocked()

	return nil
}

// rebuildIndexLocked recomputes Index, ByTarget, and ByTargetCoverage from
// p.Kernels' already-parsed Segments (no re-reading from disk). Callers
// must already hold p.mu.
func (p *Provider) rebuildIndexLocked() {
	p.Index = nil
	p.ByTarget = make(map[int32][]SegmentRef)
	p.ByTargetCoverage = make(map[int32]TargetCoverage)

	for kIdx, k := range p.Kernels {
		for i, seg := range k.Segments {
			ref := SegmentRef{KernelIndex: kIdx, SegmentIndex: i}
			p.Index = append(p.Index, ref)
			p.ByTarget[seg.Target] = append(p.ByTarget[seg.Target], ref)

			cov := p.ByTargetCoverage[seg.Target]
			if cov.Count == 0 {
				cov.StartET = seg.StartET
				cov.EndET = seg.EndET
			} else {
				cov.StartET = math.Min(cov.StartET, seg.StartET)
				cov.EndET = math.Max(cov.EndET, seg.EndET)
			}

			cov.Count++
			p.ByTargetCoverage[seg.Target] = cov
		}
	}
}

// evaluateRecursive evaluates the state of a target body at a given time.
// It recursively evaluates the state of the target body at a given time.
//
// Only called from State, which already holds p.mu for the duration — it
// uses the *Locked helpers rather than FindSegment/segment to avoid
// recursively re-acquiring the RWMutex from the same goroutine.
func (p *Provider) evaluateRecursive(targetID int32, et float64, baseID int32) (core.State, error) {
	currentID := targetID

	var totalPos, totalVel vector.Vec3

	// Limit depth to prevent infinite loops (though SPK trees should be shallow)
	for range 10 {
		if currentID == baseID {
			return core.State{Pos: totalPos, Vel: totalVel, Frame: core.FrameICRS, Center: core.CenterGeocenter}, nil
		}

		ref, err := p.findSegmentLocked(currentID, et)
		if err != nil {
			return core.State{}, err
		}

		k := p.Kernels[ref.KernelIndex]
		s := &k.Segments[ref.SegmentIndex]

		pos, vel, err := spk.EvaluateSegment(s, k.Reader, et)
		if err != nil {
			return core.State{}, fmt.Errorf("jpl: evaluate segment: %w", err)
		}

		totalPos = totalPos.Add(pos)
		totalVel = totalVel.Add(vel)
		currentID = s.Center
	}

	return core.State{}, fmt.Errorf("%w: target %d", ErrRecursionDepth, targetID)
}

// findSegmentLocked is FindSegment's body, callable by other methods that
// already hold p.mu (evaluateRecursive) without recursively re-locking it —
// sync.RWMutex's RLock is not safe to call recursively from one goroutine
// (a writer queued in between would deadlock it).
//
// It returns the reference by value. It used to return a pointer to its loop
// variable, which escaped: one allocation for every segment of every chain,
// four for a geocentric Moon, and with records cached (#802) the only
// allocations a State made.
func (p *Provider) findSegmentLocked(target int32, et float64) (SegmentRef, error) {
	// Fast failure path 1: target not loaded. A bare number is not retried as
	// a small body's: 4 is the Mars barycenter, and 4 Vesta is 20000004
	// (#809).
	refs, ok := p.ByTarget[target]
	if !ok {
		return SegmentRef{}, fmt.Errorf("%w: %d", ErrUnknownTarget, target)
	}

	// Fast failure path 2: ET outside known target coverage
	cov := p.ByTargetCoverage[target]
	if et < cov.StartET || et > cov.EndET {
		return SegmentRef{}, ErrNoSegment
	}

	// Scan target-local segments in reverse (last match wins = precedence)
	for _, ref := range slices.Backward(refs) {
		seg := p.segmentLocked(ref)
		if et >= seg.StartET && et <= seg.EndET {
			return ref, nil
		}
	}

	return SegmentRef{}, ErrNoSegment
}

// segmentLocked dereferences a SegmentRef to the actual spk.Segment. Callers
// must already hold p.mu (see findSegmentLocked's comment).
func (p *Provider) segmentLocked(ref SegmentRef) *spk.Segment {
	return &p.Kernels[ref.KernelIndex].Segments[ref.SegmentIndex]
}
