package builtin

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/tool"
)

func TestBashRedirectsPythonImageInspectionToReadFile(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	launcher := &recordingLauncher{}
	handler := NewBash(ws, launcher)
	_, err := handler.Execute(context.Background(), newJSONCall(t, "image-inspect", "bash", map[string]any{
		"command": `python3 -c 'from PIL import Image; im=Image.open("screen.png"); print(im.size)'`,
	}))
	var toolErr *tool.ToolError
	if !errors.As(err, &toolErr) {
		t.Fatalf("Execute() error = %v, want ToolError", err)
	}
	if toolErr.Recovery == nil || toolErr.Recovery.Action != tool.RecoveryUseDedicatedTool || toolErr.Recovery.Tool != "read" {
		t.Fatalf("recovery = %+v", toolErr.Recovery)
	}
	var args map[string]any
	if err := json.Unmarshal(toolErr.Recovery.Arguments, &args); err != nil {
		t.Fatal(err)
	}
	if args["path"] != "screen.png" || args["view"] != "image" {
		t.Fatalf("recovery arguments = %#v", args)
	}
	if launcher.command != "" {
		t.Fatalf("launcher command = %q, want no process execution", launcher.command)
	}
}

func TestBashRedirectsPythonTextReadToReadFile(t *testing.T) {
	suggestion := dedicatedToolForCommand(`python3 -c 'print(open("main.go").read())'`)
	if suggestion == nil || suggestion.tool != "read" || suggestion.args["path"] != "main.go" {
		t.Fatalf("suggestion = %#v", suggestion)
	}
}

func TestBashRedirectsNodeTextReadToReadFile(t *testing.T) {
	suggestion := dedicatedToolForCommand(`node -e 'console.log(fs.readFileSync("package.json", "utf8"))'`)
	if suggestion == nil || suggestion.tool != "read" || suggestion.args["path"] != "package.json" {
		t.Fatalf("suggestion = %#v", suggestion)
	}
}
func TestBashLeavesGeneralPythonComputationAlone(t *testing.T) {
	if suggestion := dedicatedToolForCommand(`python3 -c 'print(sum(range(10)))'`); suggestion != nil {
		t.Fatalf("suggestion = %#v, want nil", suggestion)
	}
}

func TestBashDedicatedRecoveryResolvesCustomCwd(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	writeTestFile(t, ws.Root(), "internal/main.go", "package internal\n")
	launcher := &recordingLauncher{}
	handler := NewBash(ws, launcher)
	_, err := handler.Execute(context.Background(), newJSONCall(t, "cwd-read", "bash", map[string]any{
		"cwd": "internal", "command": `python3 -c 'print(open("main.go").read())'`,
	}))
	var toolErr *tool.ToolError
	if !errors.As(err, &toolErr) || toolErr.Recovery == nil {
		t.Fatalf("Execute() error = %v", err)
	}
	var args map[string]any
	if err := json.Unmarshal(toolErr.Recovery.Arguments, &args); err != nil {
		t.Fatal(err)
	}
	if args["path"] != "internal/main.go" {
		t.Fatalf("recovery path = %#v", args["path"])
	}
}

func TestBashRedirectsSimpleInspectionCommands(t *testing.T) {
	tests := []struct {
		command string
		tool    string
		args    map[string]any
	}{
		{`cat "main.go"`, "read", map[string]any{"path": "main.go"}},
		{`ls internal`, "ls", map[string]any{"path": "internal"}},
		{`rg "TODO" internal`, "grep", map[string]any{"pattern": "TODO", "path": "internal"}},
		{`grep -R "TODO" internal`, "grep", map[string]any{"pattern": "TODO", "path": "internal"}},
		{`find internal -name '*.go' -type f -maxdepth 3`, "find_files", map[string]any{
			"path": "internal", "pattern": "*.go", "type": "file", "max_depth": 3,
		}},
	}
	for _, test := range tests {
		t.Run(test.command, func(t *testing.T) {
			suggestion := dedicatedToolForCommand(test.command)
			if suggestion == nil || suggestion.tool != test.tool {
				t.Fatalf("suggestion = %#v, want tool %s", suggestion, test.tool)
			}
			for key, want := range test.args {
				if got := suggestion.args[key]; got != want {
					t.Fatalf("argument %s = %#v, want %#v", key, got, want)
				}
			}
		})
	}
}

func TestBashLeavesNonEquivalentShellInspectionAlone(t *testing.T) {
	for _, command := range []string{
		`cat main.go | sed -n '1,5p'`,
		`ls -la internal`,
		`grep "TODO" internal/main.go`,
		`rg -i "todo" internal`,
		`find internal -mtime -1`,
		`cat "$FILE"`,
	} {
		t.Run(command, func(t *testing.T) {
			if suggestion := dedicatedToolForCommand(command); suggestion != nil {
				t.Fatalf("suggestion = %#v, want nil", suggestion)
			}
		})
	}
}

func TestSplitSimpleShellWordsPreservesQuotedArguments(t *testing.T) {
	words, ok := splitSimpleShellWords(`rg "hello world" "src dir"`)
	if !ok {
		t.Fatal("splitSimpleShellWords rejected balanced quoting")
	}
	want := []string{"rg", "hello world", "src dir"}
	if len(words) != len(want) {
		t.Fatalf("words = %#v, want %#v", words, want)
	}
	for index := range want {
		if words[index] != want[index] {
			t.Fatalf("words = %#v, want %#v", words, want)
		}
	}
}

func TestBashRedirectsRuntimeDiscoveryScripts(t *testing.T) {
	tests := []struct {
		command string
		tool    string
		path    string
	}{
		{`python3 -c 'from pathlib import Path; print(list(Path("internal").rglob("*.go")))'`, "find_files", "internal"},
		{`python3 -c 'import os; print(list(os.walk("internal")))'`, "find_files", "internal"},
		{`python3 -c 'from pathlib import Path; print(list(Path("internal").iterdir()))'`, "ls", "internal"},
		{`node -e 'console.log(fs.readdirSync("internal"))'`, "ls", "internal"},
	}
	for _, test := range tests {
		t.Run(test.command, func(t *testing.T) {
			suggestion := dedicatedToolForCommand(test.command)
			if suggestion == nil || suggestion.tool != test.tool || suggestion.args["path"] != test.path {
				t.Fatalf("suggestion = %#v, want %s path %s", suggestion, test.tool, test.path)
			}
		})
	}
}

func TestBashDoesNotRedirectMutatingPythonDiscoveryScript(t *testing.T) {
	command := `python3 -c 'import os; list(os.walk("internal")); os.remove("internal/tmp.txt")'`
	if suggestion := dedicatedToolForCommand(command); suggestion != nil {
		t.Fatalf("suggestion = %#v, want nil", suggestion)
	}
}
