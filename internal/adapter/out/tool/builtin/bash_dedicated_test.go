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
	if toolErr.Recovery == nil || toolErr.Recovery.Action != tool.RecoveryUseDedicatedTool || toolErr.Recovery.Tool != "read_file" {
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
	if suggestion == nil || suggestion.tool != "read_file" || suggestion.args["path"] != "main.go" {
		t.Fatalf("suggestion = %#v", suggestion)
	}
}

func TestBashRedirectsNodeTextReadToReadFile(t *testing.T) {
	suggestion := dedicatedToolForCommand(`node -e 'console.log(fs.readFileSync("package.json", "utf8"))'`)
	if suggestion == nil || suggestion.tool != "read_file" || suggestion.args["path"] != "package.json" {
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
