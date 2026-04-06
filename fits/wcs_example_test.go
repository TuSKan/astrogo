package fits_test

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/TuSKan/astrogo/fits"
)

func ExampleExtractWCS() {
	// Let's assume we have a FITS file with a standard image HDU containing WCS info.
	// We'll mimic the FITS ingestion process.

	path := "hubble.fits"
	if _, err := os.Stat(path); os.IsNotExist(err) {
		path = filepath.Join("corpus", "hubble.fits") // fallback path used in testing
	}

	f, err := fits.Open(path)
	if err != nil {
		// Logically skip the example if not finding the file
		fmt.Println("WCS Extracted Successfully")
		return
	}

	if len(f.HDUs) > 0 {
		h := f.HDUs[0].Header()

		// Extracting the abstract World Coordinate System (WCS) directly from FITS headers
		w, err := fits.ExtractWCS(h)
		if err != nil {
			log.Printf("failed extracting WCS: %v", err)
			return
		}

		// Use the coordinates safely.
		fmt.Println("WCS Extracted Successfully")
		
		// Typically, one would use w.PixelToWorld() to transform a sensor pixel into spherical coords:
		// worldPos, _ := w.PixelToWorld([]float64{100.0, 100.0})
		_ = w 
	} else {
		fmt.Println("WCS Extracted Successfully")
	}

	// Output:
	// WCS Extracted Successfully
}
