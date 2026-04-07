package provider

import (
	"strings"
	"sync"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
)

// Cache provides a thread-safe caching system for query results.
type Cache interface {
	Get(key string) (SeqIterator[Target], bool)
	Set(key string, items []Target) error
	Close() error
}

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

// ArrowCache implements an in-memory cache using Apache Arrow records to minimize GC overhead.
type ArrowCache struct {
	mu      sync.RWMutex
	records map[string]arrow.RecordBatch
	mem     memory.Allocator
}

func NewArrowCache() *ArrowCache {
	return &ArrowCache{
		records: make(map[string]arrow.RecordBatch),
		mem:     memory.NewGoAllocator(),
	}
}

func (c *ArrowCache) Get(key string) (SeqIterator[Target], bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	rec, ok := c.records[key]
	if !ok {
		return nil, false
	}

	// Retain the record so it doesn't get released while iterating
	rec.Retain()

	return func(yield func(Target, error) bool) {
		defer rec.Release()

		idArr := rec.Column(0).(*array.String)
		nameArr := rec.Column(1).(*array.String)
		desigArr := rec.Column(2).(*array.String)
		spkArr := rec.Column(3).(*array.String)
		kindArr := rec.Column(4).(*array.String)
		catArr := rec.Column(5).(*array.String)
		raArr := rec.Column(6).(*array.Float64)
		decArr := rec.Column(7).(*array.Float64)
		aliasesArr := rec.Column(8).(*array.String)

		for i := 0; i < int(rec.NumRows()); i++ {
			var aliases []string
			if !aliasesArr.IsNull(i) {
				s := aliasesArr.Value(i)
				if s != "" {
					aliases = strings.Split(s, ";")
				}
			}

			var icrs *coord.ICRS
			if !raArr.IsNull(i) && !decArr.IsNull(i) {
				icrs = coord.NewICRS(angle.Rad(raArr.Value(i)), angle.Rad(decArr.Value(i)))
			}

			t := Target{
				ID:          idArr.Value(i),
				Name:        nameArr.Value(i),
				Designation: desigArr.Value(i),
				SPKID:       spkArr.Value(i),
				Kind:        Kind(kindArr.Value(i)),
				Catalog:     catArr.Value(i),
				Coord:       icrs,
				Aliases:     aliases,
			}

			if !yield(t, nil) {
				return
			}
		}
	}, true
}

func (c *ArrowCache) Set(key string, items []Target) error {
	b := array.NewRecordBuilder(c.mem, TargetSchema)
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

		if t.Coord != nil {
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

	rec := b.NewRecordBatch()

	c.mu.Lock()
	defer c.mu.Unlock()

	// Release old record if it exists
	if old, exists := c.records[key]; exists {
		old.Release()
	}

	c.records[key] = rec
	return nil
}

func (c *ArrowCache) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, r := range c.records {
		r.Release()
	}
	c.records = make(map[string]arrow.RecordBatch)
	return nil
}
