package remote

import (
	"strings"
	"testing"

	gofs "github.com/ungerik/go-fs"
)

func TestSaveLocalFilesystem(t *testing.T) {
	dest := gofs.File(t.TempDir()).Join("sub", "cache.txt")

	if err := Save(strings.NewReader("hello local fs"), dest); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, err := dest.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if string(data) != "hello local fs" {
		t.Errorf("content = %q, want %q", data, "hello local fs")
	}
}
