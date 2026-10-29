package kepler

import (
	"math"
	"math/bits"
	"sync"
)

const (
	// epv00Ways is how many instants one set of the memo holds.
	epv00Ways = 4

	// epv00ShardBits is log2 of the number of shards, which are independently
	// locked so that goroutines ranking or scheduling in parallel do not queue
	// on one mutex.
	epv00ShardBits = 4
	epv00Shards    = 1 << epv00ShardBits

	// epv00ShardSets is how many sets one shard has, a power of two. The memo
	// holds epv00Shards * epv00ShardSets * epv00Ways = 2048 instants, about
	// 250 KB once every shard has a table.
	//
	// A sky search returns to the instants of an earlier pass of the same window,
	// a few hundred to a thousand of them away, and this holds those with room
	// for the conflicts a set-associative table has.
	epv00ShardSets = 32
)

// epv00Set is one set of the memo: epv00Ways instants that hash to it, and the
// way the next eviction takes.
type epv00Set struct {
	ways [epv00Ways]epv00At
	next uint8
}

// epv00Shard is one independently locked part of the memo. Its table is
// allocated by the first store, so a program that evaluates Epv00 a few times
// pays for a shard or two and not for the whole memo.
type epv00Shard struct {
	mu   sync.Mutex
	sets []epv00Set
}

// epv00Memo is a bounded memo of Epv00 evaluations, keyed by the exact bits of
// the two-part date.
//
// Epv00 is a pure function of its date and about 30 µs, nearly all of it sin and
// cos over a few hundred terms, so a memo changes how often it is evaluated and
// nothing about what it returns. The key is the date's bits rather than a
// rounded or truncated date for that reason: the same date gives the very same
// answer, and a different one is never mistaken for it. Interpolating between
// held instants would be cheaper still and is not this — it would change the
// bits.
//
// # Why one for the process
//
// A [sofaBase] is built per provider, and eph.Default() builds one per call in
// places, so a memo each held would start empty every time and cost an
// allocation to do it. Because the function is pure, a date one provider
// evaluated is the date any other would, and sharing the memo is invisible to
// every result. The zero value is ready to use.
//
// It replaced a memo of the last evaluation alone, which served the Sun and the
// Earth at one epoch and nothing else. Measured on a year of conjunctions and on
// a run of planet nights, the stream of dates a search asks for has a few
// hundred to a thousand distinct dates between a date and its next request.
type epv00Memo struct {
	shards [epv00Shards]epv00Shard
}

// sharedEpv00 is the memo every sofaBase uses unless it is given its own.
var sharedEpv00 epv00Memo

// at reports whether e holds the evaluation at d1 + d2.
func (e *epv00At) at(d1, d2 float64) bool {
	return e.set && math.Float64bits(e.d1) == math.Float64bits(d1) && math.Float64bits(e.d2) == math.Float64bits(d2)
}

// epv00Hash spreads the bits of a date over a word. The low bits of a date part
// barely vary from one request to the next (the day is a whole number, the
// fraction advances by steps of a minute), so they are mixed before an index is
// taken from them.
func epv00Hash(d1, d2 float64) uint64 {
	h := math.Float64bits(d1)*0x9e3779b97f4a7c15 ^ bits.RotateLeft64(math.Float64bits(d2)*0xc2b2ae3d27d4eb4f, 29)
	h ^= h >> 32
	h *= 0xd6e8feb86659fd93
	h ^= h >> 32

	return h
}

// shard is the shard a hash falls in: the top bits pick it, and the low bits
// pick the set within it.
func (m *epv00Memo) shard(h uint64) *epv00Shard {
	return &m.shards[h>>(64-epv00ShardBits)]
}

// get is the evaluation held for d1 + d2, if any.
func (m *epv00Memo) get(d1, d2 float64) (epv00At, bool) {
	h := epv00Hash(d1, d2)
	sh := m.shard(h)

	sh.mu.Lock()
	defer sh.mu.Unlock()

	if sh.sets == nil {
		return epv00At{}, false
	}

	set := &sh.sets[h&(epv00ShardSets-1)]
	for i := range set.ways {
		if set.ways[i].at(d1, d2) {
			return set.ways[i], true
		}
	}

	return epv00At{}, false
}

// put holds e, evicting the way of its set that has been held longest when the
// set is full.
func (m *epv00Memo) put(e epv00At) {
	h := epv00Hash(e.d1, e.d2)
	sh := m.shard(h)

	sh.mu.Lock()
	defer sh.mu.Unlock()

	if sh.sets == nil {
		sh.sets = make([]epv00Set, epv00ShardSets)
	}

	set := &sh.sets[h&(epv00ShardSets-1)]

	// Another goroutine may have stored the same date while this one was
	// evaluating it; a second copy would only take a way from another date.
	for i := range set.ways {
		if set.ways[i].at(e.d1, e.d2) {
			return
		}
	}

	for i := range set.ways {
		if !set.ways[i].set {
			set.ways[i] = e

			return
		}
	}

	set.ways[set.next%epv00Ways] = e
	set.next++
}
