package builtin

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSearchReplaceCRLFLineEndingTolerance(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	handler := NewSearchReplace(ws, &recordingCheckpointStore{id: "cp-crlf"})

	// File on disk has Windows CRLF line endings
	crlfContent := "func main() {\r\n\tprintln(\"hello world\")\r\n}\r\n"
	targetFile := "main_crlf.go"
	if err := os.WriteFile(filepath.Join(ws.Root(), targetFile), []byte(crlfContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Model sends Unix LF line endings in oldString and newString
	oldStringLF := "func main() {\n\tprintln(\"hello world\")\n}"
	newStringLF := "func main() {\n\tprintln(\"hello protonman\")\n}"

	_, err := handler.Execute(context.Background(), newJSONCall(t, "call-1", "edit", map[string]any{
		"filePath":  targetFile,
		"oldString": oldStringLF,
		"newString": newStringLF,
	}))
	if err != nil {
		t.Fatalf("expected CRLF tolerance to succeed, got error: %v", err)
	}

	updated, err := os.ReadFile(filepath.Join(ws.Root(), targetFile))
	if err != nil {
		t.Fatal(err)
	}
	// Verify content was updated AND preserved CRLF line endings
	if !strings.Contains(string(updated), "hello protonman") {
		t.Fatalf("updated content missing new text: %q", string(updated))
	}
	if !strings.Contains(string(updated), "\r\n") {
		t.Fatalf("expected CRLF line endings to be preserved in file: %q", string(updated))
	}
}

func TestSearchReplaceTrailingWhitespaceTolerance(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	handler := NewSearchReplace(ws, &recordingCheckpointStore{id: "cp-ws"})

	// File on disk has trailing spaces on lines
	fileContent := "package main   \n\nfunc run() {  \n    return\n}   \n"
	targetFile := "trailing.go"
	if err := os.WriteFile(filepath.Join(ws.Root(), targetFile), []byte(fileContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Model sends clean code without trailing spaces
	oldClean := "func run() {\n    return\n}"
	newClean := "func run() {\n    doSomething()\n    return\n}"

	_, err := handler.Execute(context.Background(), newJSONCall(t, "call-ws", "edit", map[string]any{
		"filePath":  targetFile,
		"oldString": oldClean,
		"newString": newClean,
	}))
	if err != nil {
		t.Fatalf("expected trailing whitespace tolerance to succeed, got error: %v", err)
	}

	updated, err := os.ReadFile(filepath.Join(ws.Root(), targetFile))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(updated), "doSomething()") {
		t.Fatalf("updated content missing new text: %q", string(updated))
	}
}

func TestSearchReplaceTabSpaceIndentationTolerance(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	handler := NewSearchReplace(ws, &recordingCheckpointStore{id: "cp-indent"})

	// File on disk uses tabs
	fileContent := "func compute() int {\n\tx := 1\n\treturn x\n}\n"
	targetFile := "indent.go"
	if err := os.WriteFile(filepath.Join(ws.Root(), targetFile), []byte(fileContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Model sends 4 spaces instead of tabs
	oldSpaces := "func compute() int {\n    x := 1\n    return x\n}"
	newSpaces := "func compute() int {\n    x := 42\n    return x\n}"

	_, err := handler.Execute(context.Background(), newJSONCall(t, "call-indent", "edit", map[string]any{
		"filePath":  targetFile,
		"oldString": oldSpaces,
		"newString": newSpaces,
	}))
	if err != nil {
		t.Fatalf("expected tab/space indentation tolerance to succeed, got error: %v", err)
	}

	updated, err := os.ReadFile(filepath.Join(ws.Root(), targetFile))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(updated), "x := 42") {
		t.Fatalf("updated content missing new text: %q", string(updated))
	}
}

func TestSearchReplaceWhitespaceToleranceAcceptsTrailingNewlineInOldString(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	handler := NewSearchReplace(ws, &recordingCheckpointStore{id: "cp-tol-nl"})

	// File uses tabs and has content after the target block.
	fileContent := "package main\n\nfunc compute() {\n\tx := 1\n}\n\nfunc other() {}\n"
	targetFile := "tolerant_nl.go"
	if err := os.WriteFile(filepath.Join(ws.Root(), targetFile), []byte(fileContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// The model sends spaces and a trailing newline. The trailing newline ends the
	// matched block; it must not require an extra empty line in the file.
	oldString := "func compute() {\n    x := 1\n}\n"
	newString := "func compute() {\n    x := 42\n}\n"

	if _, err := handler.Execute(context.Background(), newJSONCall(t, "call-tol-nl", "edit", map[string]any{
		"filePath":  targetFile,
		"oldString": oldString,
		"newString": newString,
	})); err != nil {
		t.Fatalf("expected indentation tolerance with trailing newline to succeed, got error: %v", err)
	}

	updated, err := os.ReadFile(filepath.Join(ws.Root(), targetFile))
	if err != nil {
		t.Fatal(err)
	}
	want := "package main\n\nfunc compute() {\n\tx := 42\n}\n\nfunc other() {}\n"
	if string(updated) != want {
		t.Fatalf("updated content = %q, want %q", string(updated), want)
	}
}

func TestSearchReplaceWhitespaceToleranceTrailingNewlineWithoutFileFinalNewline(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	handler := NewSearchReplace(ws, &recordingCheckpointStore{id: "cp-tol-eof"})

	// File has no final newline and uses indentation differing from oldString.
	fileContent := "package main\n\nfunc compute() {\n\tx := 1\n}"
	targetFile := "tolerant_eof.go"
	if err := os.WriteFile(filepath.Join(ws.Root(), targetFile), []byte(fileContent), 0o644); err != nil {
		t.Fatal(err)
	}

	oldString := "func compute() {\n    x := 1\n}\n"
	newString := "func compute() {\n    x := 42\n}\n"

	if _, err := handler.Execute(context.Background(), newJSONCall(t, "call-tol-eof", "edit", map[string]any{
		"filePath":  targetFile,
		"oldString": oldString,
		"newString": newString,
	})); err != nil {
		t.Fatalf("expected indentation tolerance at EOF without final newline to succeed, got error: %v", err)
	}

	updated, err := os.ReadFile(filepath.Join(ws.Root(), targetFile))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(updated), "x := 42") {
		t.Fatalf("updated content missing new text: %q", string(updated))
	}
}

func TestSearchReplaceAmbiguousWhitespaceMatchFailsSafely(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	handler := NewSearchReplace(ws, &recordingCheckpointStore{id: "cp-ambig"})

	// File has two identical blocks differing only in whitespace
	fileContent := "func a() {\n    return 0\n}\n\nfunc b() {\n    return 0\n}\n"
	targetFile := "ambig.go"
	if err := os.WriteFile(filepath.Join(ws.Root(), targetFile), []byte(fileContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// oldString matches both func a and func b if whitespace-tolerant
	oldString := "return 0"

	_, err := handler.Execute(context.Background(), newJSONCall(t, "call-ambig", "edit", map[string]any{
		"filePath":  targetFile,
		"oldString": oldString,
		"newString": "return 1",
	}))
	if err == nil {
		t.Fatal("expected error on ambiguous multi-match without replaceAll, got nil")
	}
	if !strings.Contains(err.Error(), "matched 2 locations") {
		t.Fatalf("expected error mentioning 2 locations, got: %v", err)
	}
}

func TestSearchReplaceNearMatchDiagnosticReportsDifferences(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	handler := NewSearchReplace(ws, &recordingCheckpointStore{id: "cp-diag"})

	// File has specific lines
	fileContent := "package server\n\nfunc Start() error {\n    port := 8080\n    return nil\n}\n"
	targetFile := "server.go"
	if err := os.WriteFile(filepath.Join(ws.Root(), targetFile), []byte(fileContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Model makes a slight typo (e.g. port := 9090 instead of 8080)
	oldWithTypo := "func Start() error {\n    port := 9090\n    return nil\n}"

	_, err := handler.Execute(context.Background(), newJSONCall(t, "call-diag", "edit", map[string]any{
		"filePath":  targetFile,
		"oldString": oldWithTypo,
		"newString": "func Start() error {\n    return nil\n}",
	}))
	if err == nil {
		t.Fatal("expected error for mismatched oldString, got nil")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "oldString was not found in \"server.go\"") {
		t.Fatalf("expected 'oldString was not found' in error, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "nearest match at lines") {
		t.Fatalf("expected nearest match line numbers in error, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "port := 8080") || !strings.Contains(errMsg, "port := 9090") {
		t.Fatalf("expected difference detail (8080 vs 9090) in error, got: %s", errMsg)
	}
}

func TestSearchReplaceCRLFWithWhitespaceTolerancePreservesTrailingLines(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	handler := NewSearchReplace(ws, &recordingCheckpointStore{id: "cp-crlf-ws"})

	// File on disk has CRLF and subsequent code
	crlfContent := "package main\r\n\r\nfunc compute() int {\r\n    x := 1   \r\n    return x\r\n}\r\n\r\nfunc next() string {\r\n    return \"ok\"\r\n}\r\n"
	targetFile := "crlf_trailing.go"
	if err := os.WriteFile(filepath.Join(ws.Root(), targetFile), []byte(crlfContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Model sends clean LF without trailing spaces, no trailing newline
	oldClean := "func compute() int {\n    x := 1\n    return x\n}"
	newClean := "func compute() int {\n    x := 42\n    return x\n}"

	_, err := handler.Execute(context.Background(), newJSONCall(t, "call-crlf-ws", "edit", map[string]any{
		"filePath":  targetFile,
		"oldString": oldClean,
		"newString": newClean,
	}))
	if err != nil {
		t.Fatalf("expected CRLF with whitespace tolerance to succeed, got: %v", err)
	}

	updated, err := os.ReadFile(filepath.Join(ws.Root(), targetFile))
	if err != nil {
		t.Fatal(err)
	}
	contentStr := string(updated)
	if !strings.Contains(contentStr, "x := 42") {
		t.Fatalf("missing replacement: %q", contentStr)
	}
	// Verify subsequent function next() was not merged onto the same line as closing brace
	if !strings.Contains(contentStr, "}\r\n\r\nfunc next()") {
		t.Fatalf("expected closing brace and next function to be separated cleanly by CRLFs, got: %q", contentStr)
	}
}

func TestSearchReplaceReplaceAllNonOverlappingMatches(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	handler := NewSearchReplace(ws, &recordingCheckpointStore{id: "cp-overlap"})

	// File has repeated lines
	fileContent := "line\nline\nline\nline\n"
	targetFile := "repeat.txt"
	if err := os.WriteFile(filepath.Join(ws.Root(), targetFile), []byte(fileContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// 2-line pattern should match [0,1] and [2,3] without overlapping
	oldPattern := "line\nline"
	newPattern := "replaced"

	_, err := handler.Execute(context.Background(), newJSONCall(t, "call-rep", "edit", map[string]any{
		"filePath":   targetFile,
		"oldString":  oldPattern,
		"newString":  newPattern,
		"replaceAll": true,
	}))
	if err != nil {
		t.Fatalf("replaceAll failed: %v", err)
	}

	updated, err := os.ReadFile(filepath.Join(ws.Root(), targetFile))
	if err != nil {
		t.Fatal(err)
	}
	want := "replaced\nreplaced\n"
	if string(updated) != want {
		t.Fatalf("got %q, want %q", string(updated), want)
	}
}

func TestSearchReplaceTabIndentationPreservedAcrossMultipleLines(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	handler := NewSearchReplace(ws, &recordingCheckpointStore{id: "cp-multitab"})

	// File on disk uses tabs
	fileContent := "func process() {\n\ta := 1\n\tb := 2\n\tc := 3\n}\n"
	targetFile := "multitab.go"
	if err := os.WriteFile(filepath.Join(ws.Root(), targetFile), []byte(fileContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Model sends spaces on all lines
	oldSpaces := "func process() {\n    a := 1\n    b := 2\n    c := 3\n}"
	newSpaces := "func process() {\n    a := 10\n    b := 20\n    c := 30\n}"

	_, err := handler.Execute(context.Background(), newJSONCall(t, "call-multitab", "edit", map[string]any{
		"filePath":  targetFile,
		"oldString": oldSpaces,
		"newString": newSpaces,
	}))
	if err != nil {
		t.Fatalf("edit failed: %v", err)
	}

	updated, err := os.ReadFile(filepath.Join(ws.Root(), targetFile))
	if err != nil {
		t.Fatal(err)
	}
	want := "func process() {\n\ta := 10\n\tb := 20\n\tc := 30\n}\n"
	if string(updated) != want {
		t.Fatalf("got %q, want tabs preserved on all lines %q", string(updated), want)
	}
}

func TestSearchReplaceBlankLinesDoNotCauseFalsePositiveNearMatch(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	handler := NewSearchReplace(ws, &recordingCheckpointStore{id: "cp-blank"})

	// File has several blank lines between sections
	fileContent := "package main\n\n\n\n\nfunc target() {\n    runActual()\n}\n"
	targetFile := "blank.go"
	if err := os.WriteFile(filepath.Join(ws.Root(), targetFile), []byte(fileContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Model tries to match something that only differs from target() by one line
	// and should NOT match the blank lines at the top of the file
	oldString := "func target() {\n    runTypo()\n}"

	_, err := handler.Execute(context.Background(), newJSONCall(t, "call-blank", "edit", map[string]any{
		"filePath":  targetFile,
		"oldString": oldString,
		"newString": "func target() {\n    runActual()\n}",
	}))
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// The diagnostic should point to lines 6-8 (target), not lines 1-5 (blanks)
	if !strings.Contains(err.Error(), "nearest match at lines 6-8") {
		t.Fatalf("expected nearest match at lines 6-8, got: %v", err)
	}
}
