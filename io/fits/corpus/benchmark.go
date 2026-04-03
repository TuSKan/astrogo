package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/TuSKan/astrogo/io/fits"
)

func main() {
	start := time.Now()

	// Robustly locate the file whether running from root via `go run` or inside the directory
	path := "hubble.fits"
	if _, err := os.Stat(path); os.IsNotExist(err) {
		path = filepath.Join("io", "fits", "corpus", "hubble.fits")
	}

	f, err := fits.Open(path)
	if err != nil {
		log.Fatalf("failed to open/read hubble fits: %v", err)
	}

	fmt.Printf("Successfully parsed %d HDUs in %v\n", len(f.HDUs), time.Since(start))
	
	for i, hdu := range f.HDUs {
		h := hdu.Header()
		fmt.Printf("\nHDU [%d]: %d\n", i, hdu.Type())
		
		// Print core header axes
		if ext, _ := h.GetString("EXTNAME"); ext != "" {
			fmt.Printf("EXTNAME: %s\n", ext)
		}
		
		bitpix, _ := h.GetInt("BITPIX")
		fmt.Printf("BITPIX: %d\n", bitpix)
		
		naxis, _ := h.GetInt("NAXIS")
		fmt.Printf("NAXIS: %d\n", naxis)
		for n := 1; n <= naxis; n++ {
			sz, _ := h.GetInt(fmt.Sprintf("NAXIS%d", n))
			fmt.Printf("  NAXIS%d: %d\n", n, sz)
		}
		
		// Validate checksums structurally if defined
		c, err := h.GetString("CHECKSUM")
		if err == nil {
			fmt.Printf("CHECKSUM Header Defined: %v\n", c)
		}
	}
}
