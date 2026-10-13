package luminosity_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/skybrightness/dataset/luminosity"
)

// serveCIE points remote.CIELuminosity at a bucket holding the two CIE files
// with the given bodies, and grants the download consent Open needs.
func serveCIE(t *testing.T, photopic, scotopic string) {
	t.Helper()

	t.Cleanup(remote.Capture().Restore)

	ctx := context.Background()
	src := testutil.FileURL(t, t.TempDir())

	if err := remote.SetURL(remote.CIELuminosity, src); err != nil {
		t.Fatalf("SetURL: %v", err)
	}

	bucket, err := remote.OpenFS(ctx, src)
	if err != nil {
		t.Fatalf("open source bucket: %v", err)
	}

	for name, body := range map[string]string{
		"CIE_sle_photopic.csv": photopic,
		"CIE_sle_scotopic.csv": scotopic,
	} {
		if err := remote.WriteFile(ctx, bucket, name, bytes.NewReader([]byte(body))); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}

	remote.EnableDownloads(0, remote.CIELuminosity)
	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))
}

// TestOpenReadsEachCIEFileAndCitesItsDOI runs Open end to end without the
// network (#740): each Vision fetches its own CIE file, parses it, and names
// the dataset and its DOI as the passband's reference. The rows are the first
// three of each real file, read 2026-10-13.
func TestOpenReadsEachCIEFileAndCitesItsDOI(t *testing.T) {
	serveCIE(t,
		"360,0.0000039170000\n361,0.0000043935810\n362,0.0000049296040\n",
		"380,0.0005890000\n381,0.0006650000\n382,0.0007520000\n",
	)

	for _, c := range []struct {
		vision    luminosity.Vision
		name, doi string
		first     float64
	}{
		{luminosity.Photopic, "CIE photopic", "10.25039/CIE.DS.dktna2s3", 360},
		{luminosity.Scotopic, "CIE scotopic", "10.25039/CIE.DS.gr6w4b5g", 380},
	} {
		band, err := luminosity.Open(context.Background(), c.vision)
		if err != nil {
			t.Fatalf("Open %v: %v", c.vision, err)
		}

		if band.Name != c.name {
			t.Errorf("%v: Name %q, want %q", c.vision, band.Name, c.name)
		}

		if !strings.Contains(band.Reference, c.doi) || !strings.Contains(band.Reference, "CC BY-SA 4.0") {
			t.Errorf("%v: Reference %q does not cite %s under CC BY-SA 4.0", c.vision, band.Reference, c.doi)
		}

		if len(band.WavelengthNM) != 3 || float64(band.WavelengthNM[0]) != c.first {
			t.Errorf("%v: read %v, want three rows from %g nm", c.vision, band.WavelengthNM, c.first)
		}
	}

	if _, err := luminosity.Open(context.Background(), luminosity.Vision(99)); !errors.Is(err, luminosity.ErrCurve) {
		t.Errorf("unknown vision: %v, want ErrCurve", err)
	}
}
