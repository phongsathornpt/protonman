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

func TestMarkdownStoreRevisionPersistsAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session", "todo.md")
	store, err := OpenMarkdownStore(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.CompareAndReplace(context.Background(), 0, []Item{{ID: "a", Text: "one", Status: StatusPending}})
	if err != nil {
		t.Fatal(err)
	}
	if first.Revision != 1 {
		t.Fatalf("revision=%d, want 1", first.Revision)
	}
	reopened, err := OpenMarkdownStore(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.Snapshot(); got.Revision != 1 || len(got.Items) != 1 {
		t.Fatalf("reopened=%+v", got)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "proton:todo version=1 revision=1") {
		t.Fatalf("missing durable revision marker: %s", content)
	}
}

func TestMarkdownStoreRejectsCrossProcessStaleRevision(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session", "todo.md")
	first, err := OpenMarkdownStore(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := OpenMarkdownStore(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.CompareAndReplace(context.Background(), 0, []Item{{ID: "a", Text: "one", Status: StatusPending}}); err != nil {
		t.Fatal(err)
	}
	if _, err := second.CompareAndReplace(context.Background(), 0, []Item{{ID: "b", Text: "two", Status: StatusPending}}); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("error=%v, want revision conflict", err)
	}
	if got := second.Snapshot(); got.Revision != 1 || len(got.Items) != 1 || got.Items[0].ID != "a" {
		t.Fatalf("stale store did not refresh authoritative state: %+v", got)
	}
}

func TestMarkdownStoreCreatesPrivateSessionFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "session", "todo.md")
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
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("todo mode=%#o, want 0600", got)
	}
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("session directory mode=%#o, want 0700", got)
	}
}

func TestMarkdownStoreConcurrentWritersAllowSingleRevisionWinner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session", "todo.md")
	first, err := OpenMarkdownStore(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := OpenMarkdownStore(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	errs := make(chan error, 2)
	go func() {
		<-start
		_, err := first.CompareAndReplace(context.Background(), 0, []Item{{ID: "a", Text: "one", Status: StatusPending}})
		errs <- err
	}()
	go func() {
		<-start
		_, err := second.CompareAndReplace(context.Background(), 0, []Item{{ID: "b", Text: "two", Status: StatusPending}})
		errs <- err
	}()
	close(start)
	var succeeded, conflicted int
	for range 2 {
		err := <-errs
		if err == nil {
			succeeded++
		} else if errors.Is(err, ErrRevisionConflict) {
			conflicted++
		} else {
			t.Fatalf("writer error=%v", err)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("succeeded=%d conflicted=%d", succeeded, conflicted)
	}
	store, err := OpenMarkdownStore(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if got := store.Snapshot(); got.Revision != 1 || len(got.Items) != 1 {
		t.Fatalf("snapshot=%+v", got)
	}
}

func TestMarkdownStoreCompareAndPatchRefreshesAndCommitsAtomically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session", "todo.md")
	first, err := OpenMarkdownStore(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := OpenMarkdownStore(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	before, after, err := first.CompareAndPatch(context.Background(), 0, []Operation{{Op: PatchAdd, ID: "a", Text: "one", Status: StatusPending}})
	if err != nil {
		t.Fatal(err)
	}
	if before.Revision != 0 || after.Revision != 1 || len(after.Items) != 1 || after.Items[0].ID != "a" {
		t.Fatalf("before=%+v after=%+v", before, after)
	}
	staleBefore, staleAfter, err := second.CompareAndPatch(context.Background(), 0, []Operation{{Op: PatchAdd, ID: "b", Text: "two", Status: StatusPending}})
	if !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("error=%v, want revision conflict", err)
	}
	if staleBefore.Revision != 1 || staleAfter.Revision != 1 || len(second.Snapshot().Items) != 1 || second.Snapshot().Items[0].ID != "a" {
		t.Fatalf("stale before=%+v after=%+v snapshot=%+v", staleBefore, staleAfter, second.Snapshot())
	}
}
