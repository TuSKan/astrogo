// Package jpl provides an ephemeris provider using JPL SPK/LSK kernels.
package jpl

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"

	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/jpl/lsk"
	"github.com/TuSKan/astrogo/ephemeris/jpl/spk"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

const JPL_KERNEL_URI = "https://naif.jpl.nasa.gov/pub/naif/generic_kernels/"
const KM_PER_AU = 149597870.7

var BodyIDToNAIF = map[core.ID]int{
	core.Sun: 10, core.Moon: 301, core.Mercury: 199, core.Venus: 299,
	core.Earth: 399, core.Mars: 4, core.Jupiter: 5, core.Saturn: 6,
	core.Uranus: 7, core.Neptune: 8,
}

var ErrNoSegment = fmt.Errorf("jpl: no coverage for target at requested epoch")

// Kernel represents a single SPK file and its metadata.
type Kernel struct {
	Reader   *spk.Reader
	Segments []spk.Segment
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
type Provider struct {
	LSK              *lsk.Reader
	Kernels          []*Kernel
	Index            []SegmentRef             // Global flat index
	ByTarget         map[int32][]SegmentRef   // Grouped by target
	ByTargetCoverage map[int32]TargetCoverage // Precomputed metadata
	DataDir          string

	// Configuration fields for initialization
	source    core.Source
	kernel    string
	startTime time.Time
	endTime   time.Time
}

type Option func(*Provider)

func WithDataDir(dataDir string) Option {
	return func(p *Provider) { p.DataDir = dataDir }
}

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
func NewProvider(source core.Source, kernel string, opts ...Option) (*Provider, error) {
	_, filename, _, _ := runtime.Caller(0)
	p := &Provider{
		DataDir:          filepath.Join(filepath.Dir(filename), "data"),
		ByTarget:         make(map[int32][]SegmentRef),
		ByTargetCoverage: make(map[int32]TargetCoverage),
		source:           source,
		kernel:           kernel,
	}
	for _, opt := range opts {
		opt(p)
	}

	if err := os.MkdirAll(p.DataDir, 0755); err != nil {
		return nil, fmt.Errorf("jpl: failed to create directory: %w", err)
	}

	switch p.source {
	case core.Planets:
		if p.kernel == "" {
			p.kernel = "de440"
		}
		k, err := spk.CacheDownload("planets/"+p.kernel+".bsp", p.DataDir)
		if err != nil {
			return nil, fmt.Errorf("jpl: failed to load planetary kernel: %w", err)
		}
		if err := p.AddKernel(k); err != nil {
			return nil, fmt.Errorf("jpl: failed to load planetary kernel: %w", err)
		}
	case core.Asteroids, core.Comets, core.SmallBody:
		// Always load a minimal planetary kernel for recursion (center resolution)
		pk, err := spk.CacheDownload("planets/de440s.bsp", p.DataDir)
		if err != nil {
			return nil, fmt.Errorf("jpl: failed to load planetary base kernel: %w", err)
		}
		if err := p.AddKernel(pk); err != nil {
			return nil, fmt.Errorf("jpl: failed to add planetary base kernel: %w", err)
		}

		spkReaders, err := spk.CacheAPI(p.kernel, p.startTime, p.endTime, filepath.Join(p.DataDir, string(p.source)))
		if err != nil {
			return nil, fmt.Errorf("jpl: failed to get SPK files: %w", err)
		}
		for _, reader := range spkReaders {
			if err := p.AddKernel(reader); err != nil {
				return nil, fmt.Errorf("jpl: failed to load small-body kernel: %w", err)
			}
		}
	case core.Satellites:
		return nil, fmt.Errorf("jpl: satellites source not implemented")
	case core.Stations:
		return nil, fmt.Errorf("jpl: stations source not implemented")
	default:
		return nil, fmt.Errorf("jpl: unknown source %q", p.source)
	}

	var err error
	p.LSK, err = lsk.Cache("lsk/naif0012.tls", p.DataDir)
	if err != nil {
		return nil, fmt.Errorf("jpl: failed to locate/cache LSK: %w", err)
	}

	return p, nil
}

// Close releases all kernel resources.
func (p *Provider) Close() error {
	var lastErr error
	for _, k := range p.Kernels {
		if err := k.Reader.Close(); err != nil {
			lastErr = err
		}
	}
	if p.LSK != nil {
		if err := p.LSK.Close(); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// State returns the geocentric state (position and velocity) of the given
// body at time t.
func (p *Provider) State(id core.ID, t time.Time) (core.State, error) {
	naif, ok := BodyIDToNAIF[id]
	if !ok {
		naif = int(id)
	}

	tdb := lsk.UTCToTDB(t, p.LSK)
	et := lsk.TDBToET(tdb)

	// Get target relative to SSB (0)
	targetSSB, err := p.evaluateRecursive(int32(naif), et, 0)
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
			X: relPos.X / KM_PER_AU,
			Y: relPos.Y / KM_PER_AU,
			Z: relPos.Z / KM_PER_AU,
		},
		Vel: vector.Vec3{
			X: relVel.X * 86400 / KM_PER_AU,
			Y: relVel.Y * 86400 / KM_PER_AU,
			Z: relVel.Z * 86400 / KM_PER_AU,
		},
	}, nil
}

func (p *Provider) evaluateRecursive(targetID int32, et float64, baseID int32) (core.State, error) {
	currentID := targetID
	var totalPos, totalVel vector.Vec3

	// Limit depth to prevent infinite loops (though SPK trees should be shallow)
	for depth := 0; depth < 10; depth++ {
		if currentID == baseID {
			return core.State{Pos: totalPos, Vel: totalVel}, nil
		}

		ref, err := p.FindSegment(currentID, et)
		if err != nil {
			return core.State{}, err
		}

		k := p.Kernels[ref.KernelIndex]
		s := &k.Segments[ref.SegmentIndex]
		pos, vel, err := spk.EvaluateSegment(s, k.Reader, et)
		if err != nil {
			return core.State{}, err
		}

		totalPos = totalPos.Add(pos)
		totalVel = totalVel.Add(vel)
		currentID = s.Center
	}

	return core.State{}, fmt.Errorf("jpl: recursion depth exceeded for target %d", targetID)
}

// AddKernel opens an SPK file and adds its segments to the provider index.
func (p *Provider) AddKernel(k *spk.Reader) error {
	if k == nil {
		return fmt.Errorf("jpl: kernel is nil")
	}

	summaries, err := k.ReadSummaries()
	if err != nil {
		return fmt.Errorf("jpl: failed to read summaries: %w", err)
	}

	kIdx := len(p.Kernels)
	var segments []spk.Segment
	for i, s := range summaries {
		seg := spk.Segment{
			StartET:   s.Doubles[0],
			EndET:     s.Doubles[1],
			Target:    s.Integers[0],
			Center:    s.Integers[1],
			Frame:     s.Integers[2],
			Type:      s.Integers[3],
			StartAddr: s.Integers[4],
			EndAddr:   s.Integers[5],
		}
		segments = append(segments, seg)
		ref := SegmentRef{
			KernelIndex:  kIdx,
			SegmentIndex: i,
		}
		p.Index = append(p.Index, ref)
		p.ByTarget[seg.Target] = append(p.ByTarget[seg.Target], ref)

		// Update coverage metadata
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

	p.Kernels = append(p.Kernels, &Kernel{
		Reader:   k,
		Segments: segments,
	})

	return nil
}

func (p *Provider) FindSegment(target int32, et float64) (*SegmentRef, error) {
	// Fast failure path 1: target not loaded
	refs, ok := p.ByTarget[target]
	if !ok {
		// Try asteroid mapping (20,000,000 + ID)
		if target > 0 && target < 1000000 {
			target += 20000000
			refs, ok = p.ByTarget[target]
		}
		if !ok {
			return nil, ErrNoSegment
		}
	}

	// Fast failure path 2: ET outside known target coverage
	cov := p.ByTargetCoverage[target]
	if et < cov.StartET || et > cov.EndET {
		return nil, ErrNoSegment
	}

	// Scan target-local segments in reverse (last match wins = precedence)
	for i := len(refs) - 1; i >= 0; i-- {
		ref := &refs[i]
		seg := p.segment(*ref)
		if et >= seg.StartET && et <= seg.EndET {
			return ref, nil
		}
	}
	return nil, ErrNoSegment
}

// segment dereferences a SegmentRef to the actual spk.Segment.
func (p *Provider) segment(ref SegmentRef) *spk.Segment {
	return &p.Kernels[ref.KernelIndex].Segments[ref.SegmentIndex]
}

// SupportedBodies returns a list of body IDs available in the loaded kernels.
func (p *Provider) SupportedBodies() []core.ID {
	seen := make(map[core.ID]bool)
	var res []core.ID
	for targetID := range p.ByTarget {
		bid := core.ID(targetID)
		// Map back from asteroid ID if needed
		if targetID > 20000000 && targetID < 21000000 {
			bid = core.ID(targetID - 20000000)
		}

		// Check if it's a known body
		for b, naif := range BodyIDToNAIF {
			if int32(naif) == targetID {
				bid = b
				break
			}
		}

		if !seen[bid] {
			res = append(res, bid)
			seen[bid] = true
		}
	}
	return res
}
