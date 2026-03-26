package visibility_test

import (
	"testing"
	"time"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/earth"
	"github.com/TuSKan/astrogo/observatory"
	atime "github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/visibility"
)

func BenchmarkVisibleIntervals(b *testing.B) {
	loc, _ := earth.NewGeodetic(angle.Deg(0), angle.Deg(45), 0)
	site, _ := observatory.NewSite("Test", loc, angle.Zero(), nil)
	obj := benchMock{c: coord.ICRS{RA: angle.Deg(0), Dec: angle.Deg(45)}}
	start := atime.FromJD(2460000.0, atime.UTC)
	end := start.AddDays(1.0)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = visibility.VisibleIntervals(obj, site, start, end, 10*time.Minute, angle.Deg(20))
	}
}

type benchMock struct {
	c coord.ICRS
}

func (m benchMock) ICRS(_ atime.Time) (coord.ICRS, error) {
	return m.c, nil
}
