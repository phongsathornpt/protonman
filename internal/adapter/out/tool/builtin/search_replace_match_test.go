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
