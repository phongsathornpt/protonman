package architecture_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInternalTopLevelArchitectureContract(t *testing.T) {
	root := repositoryRoot(t)
	entries, err := os.ReadDir(filepath.Join(root, "internal"))
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[string]struct{}{
		"adapter":  {},
		"app":      {},
		"base":     {},
		"core":     {},
		"engine":   {},
		"feature":  {},
		"platform": {},
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			t.Errorf("internal root must contain packages only, found file %s", entry.Name())
			continue
		}
		if _, ok := allowed[entry.Name()]; !ok {
			t.Errorf("unexpected internal architecture group %s", entry.Name())
		}
	}
}

func TestTUIRootArchitectureContract(t *testing.T) {
	root := repositoryRoot(t)
	tuiRoot := filepath.Join(root, "internal", "adapter", "in", "tui")
	entries, err := os.ReadDir(tuiRoot)
	if err != nil {
		t.Fatal(err)
	}
	allowedDirs := map[string]struct{}{
		"runtime": {},
		"state":   {},
		"view":    {},
	}
	for _, entry := range entries {
		if entry.IsDir() {
			if _, ok := allowedDirs[entry.Name()]; !ok {
				t.Errorf("unexpected TUI architecture group %s", entry.Name())
			}
			continue
		}
		if entry.Name() != "facade.go" {
			t.Errorf("TUI root must only expose facade.go, found %s", entry.Name())
		}
	}
}

func TestApplicationStructureIsNotFilenameLocked(t *testing.T) {
	root := repositoryRoot(t)
	entries, err := os.ReadDir(filepath.Join(root, "internal", "app"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			if entry.Name() != "appdirs" {
				t.Errorf("unexpected application subpackage %s; add a deliberate architecture boundary before introducing it", entry.Name())
			}
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".go") {
			t.Errorf("unexpected non-Go application entry %s", entry.Name())
		}
	}
}

func TestModelAdapterStructureIsNotFilenameLocked(t *testing.T) {
	root := repositoryRoot(t)
	modelRoot := filepath.Join(root, "internal", "adapter", "out", "model")
	entries, err := os.ReadDir(modelRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("model adapter package must not be empty")
	}
	for _, entry := range entries {
		if entry.IsDir() {
			t.Errorf("model adapter package must remain cohesive; unexpected subdirectory %s", entry.Name())
		}
	}
}
