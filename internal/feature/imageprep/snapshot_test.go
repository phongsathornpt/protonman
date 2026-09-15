package imageprep

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSnapshotLocalRejectsUnsafeDimensionsBeforeTurn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "huge.png")
	if err := os.WriteFile(path, pngHeaderOnly(uint32(MaxSourceDimension+1), 1), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := SnapshotLocal(path)
	if err == nil || !strings.Contains(err.Error(), "safe decode limit") {
		t.Fatalf("SnapshotLocal() error = %v, want safe decode limit", err)
	}
}

func TestSnapshotLocalRejectsUnsafePixelAreaBeforeTurn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "huge-area.png")
	if err := os.WriteFile(path, pngHeaderOnly(4096, 4096), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := SnapshotLocal(path)
	if err == nil || !strings.Contains(err.Error(), "safe decode limit") {
		t.Fatalf("SnapshotLocal() error = %v, want safe decode limit", err)
	}
}
