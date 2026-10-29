package kepler

import (
	"math/rand/v2"
	"runtime"
	"sync/atomic"
	"testing"
)

// The memo is read from every core at once: a ranking, a scheduler or a batch
// transform ask for the same few hundred instants from many goroutines. A hit
// writes nothing, so what these measure is whether readers wait for one another.

// benchMemoKeys is 300 distinct dates, all of which the memo holds. A
// set-associative table can lose a few to conflicts, so only the dates it kept
// are returned.
func benchMemoKeys(tb testing.TB, m *epv00Memo) [][2]float64 {
	tb.Helper()

	r := rand.New(rand.NewPCG(41, 43))
	held := make([][2]float64, 0, 300)

	for range 300 {
		d1, d2 := epv00Key(r)
		m.put(epv00At{set: true, d1: d1, d2: d2})
	}

	r = rand.New(rand.NewPCG(41, 43))

	for range 300 {
		d1, d2 := epv00Key(r)
		if _, ok := m.get(d1, d2); ok {
			held = append(held, [2]float64{d1, d2})
		}
	}

	if len(held) < 250 {
		tb.Fatalf("the memo kept only %d of 300 dates", len(held))
	}

	return held
}

// BenchmarkEpv00MemoHit is one hit, from one goroutine.
func BenchmarkEpv00MemoHit(b *testing.B) {
	m := &epv00Memo{}
	keys := benchMemoKeys(b, m)
	i := 0

	b.ReportAllocs()

	for b.Loop() {
		k := keys[i%len(keys)]
		if _, ok := m.get(k[0], k[1]); !ok {
			b.Fatal("a held date missed")
		}

		i++
	}
}

// BenchmarkEpv00MemoHitParallel is the same hits from every core at once. Its
// ns/op is wall time per hit across all of them, so it falls toward the serial
// time divided by the cores when readers do not wait for one another.
//
// "shared" is one memo read by every goroutine. "private" is the control: the
// same lookup on a memo of each goroutine's own, so no lock is shared and nothing
// can contend. How far "shared" falls behind "private" is the cost of sharing.
func BenchmarkEpv00MemoHitParallel(b *testing.B) {
	b.Run("shared", func(b *testing.B) {
		m := &epv00Memo{}
		keys := benchMemoKeys(b, m)

		var next atomic.Int64

		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			i := int(next.Add(1)) * 101

			for pb.Next() {
				k := keys[i%len(keys)]
				if _, ok := m.get(k[0], k[1]); !ok {
					b.Error("a held date missed")

					return
				}

				i++
			}
		})
	})

	b.Run("private", func(b *testing.B) {
		memos := make([]*epv00Memo, runtime.GOMAXPROCS(0))
		keys := make([][][2]float64, len(memos))

		for g := range memos {
			memos[g] = &epv00Memo{}
			keys[g] = benchMemoKeys(b, memos[g])
		}

		var next atomic.Int64

		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			g := int(next.Add(1)-1) % len(memos)
			m, ks := memos[g], keys[g]
			i := g * 101

			for pb.Next() {
				k := ks[i%len(ks)]
				if _, ok := m.get(k[0], k[1]); !ok {
					b.Error("a held date missed")

					return
				}

				i++
			}
		})
	})
}
