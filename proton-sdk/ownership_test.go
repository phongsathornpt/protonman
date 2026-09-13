package protonsdk_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(currentFile), ".."))
}

func TestProviderProtocolOwnership(t *testing.T) {
	root := repositoryRoot(t)
	goMod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(goMod), "github.com/openai/openai-go") {
		t.Fatal("official OpenAI SDK dependency must not return; proton-sdk owns the OpenAI protocol")
	}

	forbiddenTypes := map[string]bool{"Client": true, "Request": true, "Event": true, "Stream": true}
	modelDir := filepath.Join(root, "internal", "adapter", "out", "model")
	set := token.NewFileSet()
	entries, err := os.ReadDir(modelDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(set, filepath.Join(modelDir, entry.Name()), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.TYPE {
				continue
			}
			for _, spec := range gen.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if ok && forbiddenTypes[typeSpec.Name.Name] {
					t.Fatalf("legacy model boundary type returned: internal/model.%s", typeSpec.Name.Name)
				}
			}
		}
	}
	for _, legacy := range []string{"openai_request.go", "openai_stream.go", "openai_official.go", "openai_official_chat.go"} {
		if _, err := os.Stat(filepath.Join(root, "internal", "adapter", "out", "model", legacy)); err == nil {
			t.Fatalf("legacy protocol implementation returned: internal/adapter/out/model/%s", legacy)
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
}

func TestSDKDoesNotOwnAgentLoopPolicy(t *testing.T) {
	root := repositoryRoot(t)
	sdkDir := filepath.Join(root, "proton-sdk")
	forbidden := map[string]bool{
		"StepContext": true, "StopCondition": true, "StopAfterSteps": true,
		"StopWhenNoToolCalls": true, "ShouldStop": true,
	}
	set := token.NewFileSet()
	entries, err := os.ReadDir(sdkDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(set, filepath.Join(sdkDir, entry.Name()), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			switch node := decl.(type) {
			case *ast.GenDecl:
				for _, spec := range node.Specs {
					if typeSpec, ok := spec.(*ast.TypeSpec); ok && forbidden[typeSpec.Name.Name] {
						t.Fatalf("agent-loop policy %s must live outside proton-sdk", typeSpec.Name.Name)
					}
				}
			case *ast.FuncDecl:
				if forbidden[node.Name.Name] {
					t.Fatalf("agent-loop policy %s must live outside proton-sdk", node.Name.Name)
				}
			}
		}
	}
}
