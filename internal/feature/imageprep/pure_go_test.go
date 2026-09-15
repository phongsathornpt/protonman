package imageprep

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestImagePrepImplementationRemainsPureGo(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}
	dir := filepath.Dir(currentFile)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%q): %v", dir, err)
	}

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("ReadFile(%q): %v", name, err)
		}
		lower := strings.ToLower(string(content))
		for _, forbidden := range []string{
			`"os/exec"`,
			"python",
			"pillow",
			"imagemagick",
			"ffmpeg",
		} {
			if strings.Contains(lower, forbidden) {
				t.Fatalf("%s contains forbidden external image-processing dependency %q", name, forbidden)
			}
		}
	}
}
