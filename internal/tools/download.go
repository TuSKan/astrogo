package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// The generic unified structured download tool enforcing exactly mapped time constraints natively.
func main() {
	if len(os.Args) != 3 {
		fmt.Fprintf(os.Stderr, "Strict parameters violated seamlessly. Usage: go run download.go <URL> <OUTPUT_PATH>\n")
		os.Exit(1)
	}

	url := os.Args[1]
	outputPath := os.Args[2]

	dataDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Permissions restricted mapping generic data boundary organically at %s: %v\n", dataDir, err)
		os.Exit(1)
	}

	// 60-Minute native limit handling vast structurally mapped multi-gigabyte government drops flawlessly.
	client := &http.Client{
		Timeout: 60 * time.Minute,
	}

	resp, err := client.Get(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Timeout/DNS rejection wrapping %s natively: %v\n", url, err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "Strict HTTP protocol reject mapping to URL natively: %s status %s\n", url, resp.Status)
		os.Exit(1)
	}

	out, err := os.Create(outputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "OS rejection mounting configuration native map chunk -> %s: %v\n", outputPath, err)
		os.Exit(1)
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		fmt.Fprintf(os.Stderr, "TCP flush failed streaming structurally to %s natively: %v\n", outputPath, err)
		os.Exit(1)
	}

	fmt.Printf("Zero defect transmission synced natively -> %s\n", outputPath)
}
