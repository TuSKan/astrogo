package plan_test

import (
	"context"
	"sync"
	"testing"

	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
)

// recordingBrightSource answers nothing and keeps the request it was sent.
type recordingBrightSource struct {
	mu  sync.Mutex
	req resolve.BrightRequest
}

func (r *recordingBrightSource) Capabilities() []resolve.Capability {
	return []resolve.Capability{resolve.CapMagnitudeBrowse}
}

func (r *recordingBrightSource) SearchBright(_ context.Context, req resolve.BrightRequest) resolve.SeqIterator[resolve.Target] {
	r.mu.Lock()
	r.req = req
	r.mu.Unlock()

	return resolve.SliceSeq[resolve.Target](nil)
}

// VisibleTonight dates its bright search inside the night it is planning, so
// a source that can tell where a body is then can leave out what cannot be
// bright tonight: SBDB's comets far from perihelion (#779).
func TestVisibleTonightDatesItsSearchWithinTheNight(t *testing.T) {
	t.Parallel()

	site := quintaCalixtoSite(t)
	src := &recordingBrightSource{}

	if _, err := plan.VisibleTonight(context.Background(), site, testNight, 2,
		[]resolve.BrightObjectSearcher{src}, ephemeris.Default()); err != nil {
		t.Fatalf("VisibleTonight: %v", err)
	}

	_, dusk, err := plan.AstronomicalDawnDusk(testNight, testNight.AddDate(0, 0, 1), site, ephemeris.Default())
	if err != nil || dusk == nil {
		t.Fatalf("dusk: %v", err)
	}

	dawn, _, err := plan.AstronomicalDawnDusk(dusk.Time, dusk.Time.AddDate(0, 0, 1), site, ephemeris.Default())
	if err != nil || dawn == nil {
		t.Fatalf("dawn: %v", err)
	}

	src.mu.Lock()
	at, maxV := src.req.At, src.req.MaxVMag
	src.mu.Unlock()

	if at.IsZero() || !at.After(dusk.Time) || !at.Before(dawn.Time) {
		t.Errorf("the search is dated %v, want an instant between dusk %v and dawn %v",
			at.Format(time.RFC3339), dusk.Time.Format(time.RFC3339), dawn.Time.Format(time.RFC3339))
	}

	if maxV != 2 {
		t.Errorf("MaxVMag = %g, want the magLimit 2", maxV)
	}
}
