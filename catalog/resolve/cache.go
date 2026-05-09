package resolve

import (
	"strings"
	"sync"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
)

// Cache provides a thread-safe caching system for query results.
type Cache interface {
	Get(key string) (SeqIterator[Target], bool)
	Set(key string, items []Target) error
	Close() error
}

// TargetSchema defines the Apache Arrow schema used for serializing Target records.
// Retained for API compatibility with downstream consumers that may inspect it.
var TargetSchema = arrow.NewSchema(
	[]arrow.Field{
		{Name: "ID", Type: arrow.BinaryTypes.String},
		{Name: "Name", Type: arrow.BinaryTypes.String},
		{Name: "Designation", Type: arrow.BinaryTypes.String},
		{Name: "SPKID", Type: arrow.BinaryTypes.String},
		{Name: "Kind", Type: arrow.BinaryTypes.String},
		{Name: "Catalog", Type: arrow.BinaryTypes.String},
		{Name: "RA", Type: arrow.PrimitiveTypes.Float64, Nullable: true},
		{Name: "Dec", Type: arrow.PrimitiveTypes.Float64, Nullable: true},
		{Name: "Aliases", Type: arrow.BinaryTypes.String, Nullable: true}, // ';' separated to keep schema simple
	},
	nil,
)

// MapCache implements an in-memory cache that stores Target slices directly.
//
// The previous implementation serialized targets to/from Arrow RecordBatches on
// every Set/Get, adding marshal/unmarshal overhead and per-row allocations
// (ICRS, aliases) without exposing a columnar query path. This version stores
// the slices directly, retaining the MapCache name and Cache interface for
// API compatibility. The ToRecordBatch helper is available for callers that
// need columnar representation for downstream Arrow-native operations.
type MapCache struct {
	items map[string][]Target
	mu    sync.RWMutex
}

// NewMapCache returns a ready-to-use in-memory MapCache.
func NewMapCache() *MapCache {
	return &MapCache{
		items: make(map[string][]Target),
	}
}

// Get retrieves cached targets for the given query key, returning a streaming
// iterator and true if the key was found.
func (c *MapCache) Get(key string) (SeqIterator[Target], bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	items, ok := c.items[key]
	if !ok {
		return nil, false
	}

	// Return a snapshot copy to prevent mutation of cached data.
	snapshot := make([]Target, len(items))
	copy(snapshot, items)

	return func(yield func(Target, error) bool) {
		for _, t := range snapshot {
			if !yield(t, nil) {
				return
			}
		}
	}, true
}

// Set stores a slice of targets under the given query key.
func (c *MapCache) Set(key string, items []Target) error {
	// Store a defensive copy to prevent caller mutation.
	stored := make([]Target, len(items))
	copy(stored, items)

	c.mu.Lock()
	defer c.mu.Unlock()

	c.items[key] = stored
	return nil
}

// Close clears all cached entries.
func (c *MapCache) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string][]Target)
	return nil
}

// ToRecordBatch converts a slice of Targets to an Arrow RecordBatch for callers
// that need columnar representation for downstream Arrow-native operations.
// The caller is responsible for calling Release() on the returned record.
func ToRecordBatch(items []Target) arrow.RecordBatch {
	mem := memory.NewGoAllocator()
	b := array.NewRecordBuilder(mem, TargetSchema)
	defer b.Release()

	idB := b.Field(0).(*array.StringBuilder)
	nameB := b.Field(1).(*array.StringBuilder)
	desigB := b.Field(2).(*array.StringBuilder)
	spkB := b.Field(3).(*array.StringBuilder)
	kindB := b.Field(4).(*array.StringBuilder)
	catB := b.Field(5).(*array.StringBuilder)
	raB := b.Field(6).(*array.Float64Builder)
	decB := b.Field(7).(*array.Float64Builder)
	aliasesB := b.Field(8).(*array.StringBuilder)

	for _, t := range items {
		idB.Append(t.ID)
		nameB.Append(t.Name)
		desigB.Append(t.Designation)
		spkB.Append(t.SPKID)
		kindB.Append(string(t.Kind))
		catB.Append(t.Catalog)

		if t.HasCoord {
			raB.Append(t.Coord.RA().Radians())
			decB.Append(t.Coord.Dec().Radians())
		} else {
			raB.AppendNull()
			decB.AppendNull()
		}

		if len(t.Aliases) > 0 {
			aliasesB.Append(strings.Join(t.Aliases, ";"))
		} else {
			aliasesB.AppendNull()
		}
	}

	return b.NewRecordBatch()
}

// FromRecordBatch converts an Arrow RecordBatch back to a slice of Targets.
// This is the inverse of ToRecordBatch for callers that receive Arrow data
// from external sources.
func FromRecordBatch(rec arrow.RecordBatch) []Target {
	idArr := rec.Column(0).(*array.String)
	nameArr := rec.Column(1).(*array.String)
	desigArr := rec.Column(2).(*array.String)
	spkArr := rec.Column(3).(*array.String)
	kindArr := rec.Column(4).(*array.String)
	catArr := rec.Column(5).(*array.String)
	raArr := rec.Column(6).(*array.Float64)
	decArr := rec.Column(7).(*array.Float64)
	aliasesArr := rec.Column(8).(*array.String)

	targets := make([]Target, int(rec.NumRows()))
	for i := range targets {
		var aliases []string
		if !aliasesArr.IsNull(i) {
			s := aliasesArr.Value(i)
			if s != "" {
				aliases = strings.Split(s, ";")
			}
		}

		var icrs coord.ICRS
		hasCoord := false
		if !raArr.IsNull(i) && !decArr.IsNull(i) {
			icrs = coord.NewICRS(angle.Rad(raArr.Value(i)), angle.Rad(decArr.Value(i)))
			hasCoord = true
		}

		targets[i] = Target{
			ID:          idArr.Value(i),
			Name:        nameArr.Value(i),
			Designation: desigArr.Value(i),
			SPKID:       spkArr.Value(i),
			Kind:        Kind(kindArr.Value(i)),
			Catalog:     catArr.Value(i),
			Coord:       icrs,
			HasCoord:    hasCoord,
			Aliases:     aliases,
		}
	}

	return targets
}
