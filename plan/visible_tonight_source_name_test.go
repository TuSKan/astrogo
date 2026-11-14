package plan_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/plan"
)

// namedFailingBrightSource is failingBrightSource with a provider name, as
// every catalog provider has one.
type namedFailingBrightSource struct {
	failingBrightSource
}

func (namedFailingBrightSource) Name() string { return "Fake Catalog" }

// TestVisibleTonightNamesAFailedSource: the reason for a skipped source gave
// its Go type, so a SIMBAD outage read `bright source "*simbad.Provider"`
// (#1138). A source with a provider name is now named by it, and only one
// without falls back to the type.
func TestVisibleTonightNamesAFailedSource(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		src  resolve.BrightObjectSearcher
		want string
	}{
		{&namedFailingBrightSource{failingBrightSource{good: sirius}}, `"Fake Catalog"`},
		{&failingBrightSource{good: sirius}, `"*plan_test.failingBrightSource"`},
	} {
		_, err := plan.VisibleTonight(context.Background(), quintaCalixtoSite(t), testNight, 2,
			[]resolve.BrightObjectSearcher{c.src}, ephemeris.Default())

		if !errors.Is(err, plan.ErrIncomplete) {
			t.Fatalf("err = %v, want ErrIncomplete for a source that failed", err)
		}

		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("the skipped source is not named %s: %v", c.want, err)
		}
	}
}
