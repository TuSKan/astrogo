package remotetest_test

import (
	"errors"
	"testing"

	"github.com/TuSKan/astrogo/internal/remotetest"
	"github.com/TuSKan/astrogo/remote"
)

// TestRefuseDownloadsRefusesOnlyWhatItIsGiven: named endpoints are refused
// with ErrRefused inside remote.ErrDownloadDenied, and the others are not.
func TestRefuseDownloadsRefusesOnlyWhatItIsGiven(t *testing.T) {
	remotetest.RefuseDownloads(t, remote.NAIFSPK)

	err := remote.CheckDownload(remote.NAIFSPK, "de440s.bsp", 32<<20)
	if !errors.Is(err, remote.ErrDownloadDenied) || !errors.Is(err, remotetest.ErrRefused) {
		t.Errorf("CheckDownload(NAIFSPK) = %v, want ErrDownloadDenied wrapping ErrRefused", err)
	}

	if err := remote.CheckDownload(remote.OpenNGC, "NGC.csv", 4<<20); err != nil {
		t.Errorf("CheckDownload(OpenNGC) = %v, want nil: only NAIFSPK was refused", err)
	}
}

// TestRefuseDownloadsWithNoIDsRefusesEverything covers the empty list.
func TestRefuseDownloadsWithNoIDsRefusesEverything(t *testing.T) {
	remotetest.RefuseDownloads(t)

	for _, id := range []remote.EndpointID{remote.NAIFSPK, remote.OpenNGC, remote.IERSFinals2000A} {
		if err := remote.CheckDownload(id, "x", 1); !errors.Is(err, remotetest.ErrRefused) {
			t.Errorf("CheckDownload(%s) = %v, want ErrRefused", id, err)
		}
	}
}

// TestRefuseDownloadsPutsTheDefaultBack: once the test that refused ends,
// downloads are allowed again.
func TestRefuseDownloadsPutsTheDefaultBack(t *testing.T) {
	t.Run("refusing", func(t *testing.T) {
		remotetest.RefuseDownloads(t)
	})

	if err := remote.CheckDownload(remote.NAIFSPK, "de440s.bsp", 32<<20); err != nil {
		t.Errorf("after the refusing test ended, CheckDownload = %v, want nil", err)
	}
}
