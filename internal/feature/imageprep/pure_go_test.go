package imageprep

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var forbiddenImageRuntimeDependencies = []string{
	`"os/exec"`,
	"python",
	"pillow",
	"imagemagick",
	"ffmpeg",
}

func TestImagePipelineImplementationRemainsPureGo(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}
	packageDir := filepath.Dir(currentFile)
	repoRoot := filepath.Clean(filepath.Join(packageDir, "..", "..", ".."))

	entries, err := os.ReadDir(packageDir)
	if err != nil {
		t.Fatalf("ReadDir(%q): %v", packageDir, err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		assertPureGoImageFile(t, filepath.Join(packageDir, name))
	}

	for _, relativePath := range []string{
		"internal/adapter/out/tool/builtin/readfile/image.go",
		"internal/adapter/out/tool/builtin/readfile/vision.go",
		"internal/adapter/in/tui/runtime/clipboardimage/clipboard.go",
		"internal/adapter/in/tui/runtime/turn_runtime.go",
	} {
		assertPureGoImageFile(t, filepath.Join(repoRoot, filepath.FromSlash(relativePath)))
	}
}

func assertPureGoImageFile(t *testing.T, path string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", path, err)
	}
	lower := strings.ToLower(string(content))
	for _, forbidden := range forbiddenImageRuntimeDependencies {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("%s contains forbidden external image-processing dependency %q", path, forbidden)
		}
	}
}
