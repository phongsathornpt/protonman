package pathutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSameResolvesSymlinkedAncestorsAndMissingLeaves(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(target, alias); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	same, err := Same(filepath.Join(alias, "missing", "file.txt"), filepath.Join(target, "missing", "file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !same {
		t.Fatal("Same() = false, want true")
	}
}

func TestWithinUsesPathBoundaries(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}

	inside, err := Within(filepath.Join(root, "nested", "file.txt"), root)
	if err != nil || !inside {
		t.Fatalf("Within(inside) = %v, %v", inside, err)
	}

	sibling := root + "-other"
	if err := os.Mkdir(sibling, 0o755); err != nil {
		t.Fatal(err)
	}
	inside, err = Within(filepath.Join(sibling, "file.txt"), root)
	if err != nil {
		t.Fatal(err)
	}
	if inside {
		t.Fatal("Within(sibling-prefix) = true, want false")
	}
}

func TestCanonicalNormalizesParentTraversal(t *testing.T) {
	root := t.TempDir()
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Canonical(filepath.Join(root, "a", "..", "b"))
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(canonicalRoot, "b")
	if got != want {
		t.Fatalf("Canonical() = %q, want %q", got, want)
	}
}
