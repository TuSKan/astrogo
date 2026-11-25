package iers

import (
	"errors"
	"math"
	"strings"
	"sync"
	"testing"
	"time"
)

// Every Earth orientation lookup in a process reads the model, from every core
// at once. These tests hold the read path to what #953 made it: no lock on the
// model or the loader, one read of the model's state, and an answer only where
// nothing need be loaded.

// answeringModel answers every epoch and is not a table: a model the lazy
// loader never registers, authoritative only when a caller chose it.
type answeringModel struct{}

func (answeringModel) EOP(float64) (EOP, error) { return EOP{DUT1: 0.25}, nil }

func sampleTable(t *testing.T) *Table {
	t.Helper()

	table, err := ParseFinals2000A(strings.NewReader(sampleFinals2000A))
	if err != nil {
		t.Fatal(err)
	}

	return table
}

func sameEOPBits(a, b EOP) bool {
	return math.Float64bits(a.DUT1) == math.Float64bits(b.DUT1) &&
		math.Float64bits(a.XP) == math.Float64bits(b.XP) &&
		math.Float64bits(a.YP) == math.Float64bits(b.YP) &&
		math.Float64bits(a.LOD) == math.Float64bits(b.LOD)
}

// TestLookupAnswersOnlyWhatNeedsNothingLoaded: Lookup is a lookup's first
// answer, given without EnsureLoaded, so it must answer exactly where the full
// path would return the model's own EOP with nothing to load and nothing
// defaulted. There it gives that EOP to the bit, and EnsureLoaded returns at
// once without asking the loader. Everywhere else it declines, and the full
// path runs as before: an uncovered epoch, a model the lazy loader would try
// to replace, and a table recorded as no source, which the full path reports
// as defaulted.
func TestLookupAnswersOnlyWhatNeedsNothingLoaded(t *testing.T) {
	table := sampleTable(t)

	const covered, uncovered = 41684.5, 50000.0

	for _, tc := range []struct {
		name     string
		register func()
		mjd      float64
		want     bool
	}{
		{"pristine", func() {}, covered, false},
		{"explicit zero", func() { RegisterModel(ZeroModel{}) }, covered, true},
		{"explicit table, covered", func() { RegisterModel(table) }, covered, true},
		{"explicit table, uncovered", func() { RegisterModel(table) }, uncovered, false},
		{"explicit non-table", func() { RegisterModel(answeringModel{}) }, covered, true},
		{"cached table, covered", func() { registerModelInternal(table, SourceCache) }, covered, true},
		{"fetched table, uncovered", func() { registerModelInternal(table, SourceNetwork) }, uncovered, false},
		{"loaded non-table", func() { registerModelInternal(answeringModel{}, SourceCache) }, covered, false},
		{"table with no source", func() { registerModelInternal(table, SourceZero) }, covered, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l := noCache(Data{}, errUpstream)
			installLoader(t, l)
			tc.register()

			got, ok := Lookup(tc.mjd)
			if ok != tc.want {
				t.Fatalf("Lookup(%v) ok = %v, want %v", tc.mjd, ok, tc.want)
			}

			if !ok {
				return
			}

			want, err := GetModel().EOP(tc.mjd)
			if err != nil {
				t.Fatalf("the model Lookup answered for does not answer %v: %v", tc.mjd, err)
			}

			if !sameEOPBits(got, want) {
				t.Errorf("Lookup(%v) = %+v, want the model's own %+v", tc.mjd, got, want)
			}

			if err := EnsureLoaded(tc.mjd); err != nil {
				t.Errorf("EnsureLoaded(%v) = %v where Lookup answered, want nil", tc.mjd, err)
			}

			if cached, fetched := l.counts(); cached != 0 || fetched != 0 {
				t.Errorf("the loader was asked (%d cached, %d fetched) where Lookup answered", cached, fetched)
			}
		})
	}
}

// TestModelReadsTakeNoLock: modelMu orders the writers only. It is held here
// as a writer holds it, and every read of the model must still return. They
// used to take it for reading, each an atomic add on one shared counter, so
// sixteen goroutines looked up fewer epochs a second than one (#953).
func TestModelReadsTakeNoLock(t *testing.T) {
	t.Cleanup(resetForTest)
	RegisterModel(sampleTable(t))

	modelMu.Lock()

	done := make(chan struct{})

	go func() {
		defer close(done)

		_ = GetModel()
		_ = EOPSource()
		_ = modelIsExplicit()
		_, _ = Lookup(41684.5)
	}()

	blocked := false

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		blocked = true
	}

	modelMu.Unlock()
	<-done

	if blocked {
		t.Fatal("a read of the model waited on modelMu")
	}
}

// TestEnsureLoadedAnswersNoLoaderWithoutFetchMu: with no loader registered,
// the state of every program that does not import remote/eop, the answer can
// change only when RegisterLoader runs. It is given here while fetchMu is held,
// which every lookup of such a program used to queue on to hear it (#953).
func TestEnsureLoadedAnswersNoLoaderWithoutFetchMu(t *testing.T) {
	installLoader(t, nil)

	fetchMu.Lock()

	result := make(chan error, 1)

	go func() { result <- EnsureLoaded(41684) }()

	var err error

	blocked := false

	select {
	case err = <-result:
	case <-time.After(5 * time.Second):
		blocked = true
	}

	fetchMu.Unlock()

	if blocked {
		t.Fatalf("EnsureLoaded with no loader waited on fetchMu, then returned %v", <-result)
	}

	if !errors.Is(err, ErrNoLoader) {
		t.Errorf("EnsureLoaded with no loader = %v, want ErrNoLoader", err)
	}
}

// TestModelStateIsOneRegistration: a reader takes the model, its source and
// whether a caller chose it as one value, so it never pairs one registration's
// model with another's source however registrations interleave. Under -race
// it is also the check that the state is read and written without a race.
func TestModelStateIsOneRegistration(t *testing.T) {
	t.Cleanup(resetForTest)

	table := sampleTable(t)
	stop := make(chan struct{})

	var wg sync.WaitGroup

	for range 4 {
		wg.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
				}

				s := state()
				_, isTable := s.model.(*Table)

				var want string

				switch {
				case s.explicit && isTable:
					want = SourceExplicit
				case !s.explicit && isTable:
					want = SourceCache
				case !s.explicit:
					want = SourceZero
				default:
					t.Errorf("an explicit %T, which this test never registers", s.model)

					return
				}

				if s.source != want {
					t.Errorf("explicit %v, %T, source %q: want %q", s.explicit, s.model, s.source, want)

					return
				}
			}
		})
	}

	for range 2000 {
		RegisterModel(table)
		Reset()
		registerModelInternal(table, SourceCache)
		Reset()
	}

	close(stop)
	wg.Wait()
}
