package todo

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMarkdownStorePreservesDocumentAroundManagedSection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "TODO.md")
	original := "# Project\n\nKeep this paragraph.\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := OpenMarkdownStore(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	items := []Item{{ID: "a", Text: "first", Status: StatusInProgress}, {ID: "b", Text: "second", Status: StatusCompleted}}
	if _, err := store.Replace(context.Background(), items); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	if !strings.HasPrefix(text, original) {
		t.Fatalf("document prefix changed:\n%s", text)
	}
	if !strings.Contains(text, "- [~] [a] first") || !strings.Contains(text, "- [x] [b] second") {
		t.Fatalf("managed tasks missing:\n%s", text)
	}
	reopened, err := OpenMarkdownStore(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.Snapshot(); len(got.Items) != 2 || got.Items[0] != items[0] || got.Items[1] != items[1] {
		t.Fatalf("reopened = %#v", got)
	}
}

func TestMarkdownStoreLoadsLegacyCheckboxes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "TODO.md")
	if err := os.WriteFile(path, []byte("# old\n- [ ] one\n- [x] two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := OpenMarkdownStore(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	got := store.Snapshot().Items
	if len(got) != 2 || got[0].Status != StatusPending || got[1].Status != StatusCompleted {
		t.Fatalf("legacy = %#v", got)
	}
}

func TestMarkdownStoreRejectsMalformedManagedSection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "TODO.md")
	if err := os.WriteFile(path, []byte(managedStart+"\n- [ ] missing-id\n"+managedEnd), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenMarkdownStore(context.Background(), path); err == nil {
		t.Fatal("expected malformed managed section error")
	}
}

func TestMarkdownStoreCanceledReplaceKeepsState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "TODO.md")
	store, err := OpenMarkdownStore(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = store.Replace(ctx, []Item{{ID: "a", Text: "one", Status: StatusPending}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
	if got := store.Snapshot(); got.Revision != 0 || len(got.Items) != 0 {
		t.Fatalf("state mutated: %#v", got)
	}
}

func TestMarkdownStoreWriteFailureKeepsMemory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "TODO.md")
	store, err := OpenMarkdownStore(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err = store.Replace(context.Background(), []Item{{ID: "a", Text: "one", Status: StatusPending}})
	if err == nil {
		t.Fatal("expected persistence error")
	}
	if got := store.Snapshot(); got.Revision != 0 || len(got.Items) != 0 {
		t.Fatalf("memory changed after persistence failure: %#v", got)
	}
}

func TestMarkdownStoreRejectsDuplicateManagedSections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "TODO.md")
	body := managedStart + "\n- [ ] [a] one\n" + managedEnd + "\n" + managedStart + "\n- [ ] [b] two\n" + managedEnd + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenMarkdownStore(context.Background(), path); err == nil {
		t.Fatal("expected duplicate managed section error")
	}
}

func TestMarkdownStorePreservesFileMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "TODO.md")
	if err := os.WriteFile(path, []byte("# todo\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	store, err := OpenMarkdownStore(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompareAndReplace(context.Background(), 0, []Item{{ID: "a", Text: "one", Status: StatusPending}}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o640 {
		t.Fatalf("mode=%#o, want 0640", got)
	}
}

func TestMarkdownStoreCompareAndReplaceRejectsStaleRevision(t *testing.T) {
	path := filepath.Join(t.TempDir(), "TODO.md")
	store, err := OpenMarkdownStore(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompareAndReplace(context.Background(), 0, []Item{{ID: "a", Text: "one", Status: StatusPending}}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompareAndReplace(context.Background(), 0, []Item{{ID: "b", Text: "two", Status: StatusPending}}); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("error=%v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "[b]") {
		t.Fatalf("stale update reached disk: %s", content)
	}
}
