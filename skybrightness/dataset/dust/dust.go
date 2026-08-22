// Package dust supplies the 100 micron thermal emission of interstellar dust
// that the diffuse-galactic-light component correlates against.
//
// Dust scatters starlight, and the scattered light is what
// [github.com/TuSKan/astrogo/skybrightness.DiffuseGalacticLight] predicts. The
// correlation it evaluates — Kawara et al. (2017) — is fitted against 100
// micron emission, so that emission is the input the component needs, in
// MJy/sr, by galactic coordinates.
//
// The data comes from NASA/IPAC's Galactic Dust Extinction Service, which
// reprocesses IRAS, COBE/DIRBE and Planck. It is queried per direction rather
// than downloaded whole, so this package fetches the directions a caller asks
// about and caches them — the same shape as
// [github.com/TuSKan/astrogo/skybrightness/dataset/starlight.Fetch], and for
// the same reason: nobody observes the whole sky at once, and the alternative
// is moving a large map to use a little of it.
//
// Evaluation performs no I/O, so fetching happens here and the result is handed
// to the component through the scene.
package dust

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/remote/api"
	"github.com/TuSKan/astrogo/remote/file"
)

// Sentinel errors for the dust provider.
var (
	// ErrNoCoverage is returned for a direction the map was never asked to
	// fetch. It is distinct from a genuinely dark sightline, which has a
	// value.
	ErrNoCoverage = errors.New("dust: no 100 micron value fetched for that direction")

	// ErrResponse is returned when the service answers with something this
	// package cannot read.
	ErrResponse = errors.New("dust: unreadable response from the dust service")
)

// queryPace is the minimum gap between requests to the service.
//
// IRSA is a shared, free, anonymous service, and this package issues one
// request per direction rather than one per session. Two seconds is the same
// pacing the Gaia aggregation uses, adopted for the same reason: this project
// has already been throttled once for asking a research archive too much too
// quickly.
const queryPace = 2 * time.Second

// Map holds fetched 100 micron intensities and satisfies
// [github.com/TuSKan/astrogo/skybrightness.DustMap].
//
// It is sparse by construction: it answers for the directions it was asked to
// fetch and reports [ErrNoCoverage] elsewhere, which the component treats as
// missing data rather than as a dust-free sightline.
//
// Safe for concurrent use.
type Map struct {
	mu     sync.RWMutex
	values map[cell]float64
}

// cell is a direction rounded to the resolution the service is sampled at.
//
// Rounding is what makes a cache useful: two targets a few arcminutes apart sit
// in the same dust cell, and asking twice would spend a request to learn the
// same number. The resolution matches the region the service averages over.
type cell struct{ l, b int }

// cellSizeDeg is the angular size of a cache cell.
//
// SFD's own resolution is about 6 arcminutes and the diffuse galactic light
// varies smoothly on larger scales than that, so a tenth of a degree keeps the
// structure the correlation can actually use while collapsing repeat queries.
const cellSizeDeg = 0.1

func cellOf(l, b angle.Angle) cell {
	return cell{
		l: int(math.Round(l.Degrees() / cellSizeDeg)),
		b: int(math.Round(b.Degrees() / cellSizeDeg)),
	}
}

// NewMap returns an empty map, ready to be filled by [Fetch].
func NewMap() *Map {
	return &Map{values: make(map[cell]float64)}
}

// IntensityAt implements
// [github.com/TuSKan/astrogo/skybrightness.DustMap].
func (m *Map) IntensityAt(l, b angle.Angle) (float64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	v, ok := m.values[cellOf(l, b)]
	if !ok {
		return 0, fmt.Errorf("%w: l=%.3f b=%.3f", ErrNoCoverage, l.Degrees(), b.Degrees())
	}

	return v, nil
}

// Len reports how many directions the map holds.
func (m *Map) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return len(m.values)
}

// set records a value, rounded into its cell.
func (m *Map) set(l, b angle.Angle, v float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.values[cellOf(l, b)] = v
}

// Direction is a galactic coordinate to fetch.
type Direction struct{ L, B angle.Angle }

// Fetch retrieves the 100 micron intensity for each direction and returns a map
// holding them.
//
// Directions falling in the same cell are queried once. Passing an existing map
// as into adds to it rather than starting over, so a caller can accumulate the
// sky it uses across sessions.
//
// One request per new cell, paced. A long target list is therefore slow rather
// than heavy: the service is asked politely and repeatedly, never in a burst.
func Fetch(ctx context.Context, into *Map, directions ...Direction) (*Map, error) {
	if into == nil {
		into = NewMap()
	}

	if len(directions) == 0 {
		return into, nil
	}

	// What previous sessions already learned. A sightline's 100 micron
	// intensity does not change, so a value fetched once is a value fetched
	// for good; the alternative is asking a shared facility the same question
	// every run, which is how this package spent twenty-five minutes of IRSA's
	// time answering a question it had already answered.
	bucket, prefix, cacheErr := remote.CacheDir(ctx, remote.IRSADust)

	key := ""
	held := map[cell]float64{}

	if cacheErr == nil {
		key = path.Join(prefix, cacheFile)
		// A cache that cannot be read is a cold cache, not a failure.
		held = readCache(ctx, bucket, key)
	}

	var (
		client  *api.Client
		fetched bool
	)

	defer func() {
		if client != nil {
			_ = client.Close()
		}
	}()

	seen := make(map[cell]struct{}, len(directions))

	for _, d := range directions {
		c := cellOf(d.L, d.B)
		if _, dup := seen[c]; dup {
			continue
		}

		seen[c] = struct{}{}

		if _, err := into.IntensityAt(d.L, d.B); err == nil {
			continue // already held in memory
		}

		if v, ok := held[c]; ok {
			into.set(d.L, d.B, v)

			continue // already held on disk
		}

		// The client is built on the first sightline that actually needs
		// asking, so a fully cached call makes no connection at all.
		if client == nil {
			c, err := api.NewClient(remote.IRSADust,
				api.WithMinInterval(queryPace),
				api.WithTimeout(90*time.Second))
			if err != nil {
				return nil, fmt.Errorf("dust: client: %w", err)
			}

			client = c
		}

		v, err := query(ctx, client, d)
		if err != nil {
			// Keep whatever this call did learn: being cut off part way
			// through a long list should cost the remaining sightlines, not
			// the ones already paid for.
			if fetched && key != "" {
				_ = writeCache(ctx, bucket, key, held)
			}

			return nil, err
		}

		into.set(d.L, d.B, v)

		held[c] = v
		fetched = true
	}

	if fetched && key != "" {
		// A cache that cannot be written costs the next run its time, not
		// this one its answer.
		_ = writeCache(ctx, bucket, key, held)
	}

	return into, nil
}

// cacheFile is where fetched intensities accumulate under the endpoint's
// cache directory.
const cacheFile = "i100.txt"

// readCache returns the intensities a previous session stored, keyed by cell.
//
// Any failure is a cold cache: the file may not exist yet, and a truncated or
// malformed line costs the sightline on it rather than the whole file, since
// the worst case is asking IRSA again for that one direction.
func readCache(ctx context.Context, bucket *file.Bucket, key string) map[cell]float64 {
	out := map[cell]float64{}

	r, err := bucket.NewReader(ctx, key, nil)
	if err != nil {
		return out
	}
	defer func() { _ = r.Close() }()

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 3 {
			continue
		}

		l, errL := strconv.Atoi(fields[0])
		b, errB := strconv.Atoi(fields[1])

		v, errV := strconv.ParseFloat(fields[2], 64)
		if errL != nil || errB != nil || errV != nil {
			continue
		}

		if v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}

		out[cell{l: l, b: b}] = v
	}

	return out
}

// writeCache stores every intensity held, replacing the file.
//
// The whole map each time rather than an append: the file is a few tens of
// bytes per sightline, and rewriting it keeps one reader implementation
// instead of one for the file and another for its tail.
func writeCache(ctx context.Context, bucket *file.Bucket, key string, held map[cell]float64) error {
	var buf strings.Builder

	for c, v := range held {
		fmt.Fprintf(&buf, "%d %d %.6e\n", c.l, c.b, v)
	}

	if err := file.Save(ctx, bucket, key, strings.NewReader(buf.String())); err != nil {
		return fmt.Errorf("dust: write cache %s: %w", key, err)
	}

	return nil
}

// hundredMicron matches the 100 micron block's reference-pixel value, in
// MJy/sr.
//
// The service answers with several results in one document — reddening, 100
// micron emission and dust temperature — each with the same element names, so
// the block has to be located by its description before its value is read.
// Taking the first refPixelValue in the document would silently return a
// reddening in magnitudes where an intensity in MJy/sr was wanted.
var hundredMicron = regexp.MustCompile(
	`(?s)100 Micron Emission.*?<refPixelValue>\s*([0-9.eE+-]+)\s*\(MJy/sr\)`)

// query asks the service for one direction.
func query(ctx context.Context, client *api.Client, d Direction) (float64, error) {
	params := url.Values{}
	params.Set("locstr", fmt.Sprintf("%.6f %.6f gal", d.L.Degrees(), d.B.Degrees()))
	params.Set("regSize", "2.0")

	body, err := client.Get(ctx, remote.IRSADust, "", params)
	if err != nil {
		return 0, fmt.Errorf("dust: l=%.3f b=%.3f: %w", d.L.Degrees(), d.B.Degrees(), err)
	}
	defer func() { _ = body.Close() }()

	raw, err := io.ReadAll(body)
	if err != nil {
		return 0, fmt.Errorf("dust: reading response: %w", err)
	}

	return parse(string(raw))
}

// parse extracts the 100 micron intensity from a service response.
func parse(doc string) (float64, error) {
	m := hundredMicron.FindStringSubmatch(doc)
	if m == nil {
		return 0, fmt.Errorf("%w: no 100 micron block with MJy/sr units", ErrResponse)
	}

	v, err := strconv.ParseFloat(strings.TrimSpace(m[1]), 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %q is not a number", ErrResponse, m[1])
	}

	if v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, fmt.Errorf("%w: intensity %v", ErrResponse, v)
	}

	return v, nil
}

// Cached is one direction the service has already been asked about, and what
// it answered.
type Cached struct {
	L, B      angle.Angle
	Intensity float64
}

// CachedDirections returns every answer already held on disk.
//
// The intensity toward a direction does not change, so an answer fetched once
// is an answer for good, and the cache accumulates whatever a caller has ever
// asked for. Exposing it is what lets the local all-sky map be validated
// against the service without asking the service anything: the comparison runs
// against questions it has already answered.
//
// A cache that cannot be read is an empty result rather than an error — a cold
// cache is a normal state, not a failure.
func CachedDirections(ctx context.Context) ([]Cached, error) {
	bucket, prefix, err := remote.CacheDir(ctx, remote.IRSADust)
	if err != nil {
		return nil, fmt.Errorf("dust: cache: %w", err)
	}

	held := readCache(ctx, bucket, path.Join(prefix, cacheFile))

	out := make([]Cached, 0, len(held))
	for c, v := range held {
		out = append(out, Cached{
			L:         angle.Deg(float64(c.l) * cellSizeDeg),
			B:         angle.Deg(float64(c.b) * cellSizeDeg),
			Intensity: v,
		})
	}

	// A stable order, so a comparison reports the same worst case twice
	// running.
	sort.Slice(out, func(i, j int) bool {
		if out[i].B != out[j].B {
			return out[i].B < out[j].B
		}

		return out[i].L < out[j].L
	})

	return out, nil
}
