package architecture_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestModelAdapterDoesNotRedefineSDKBoundaryTypes(t *testing.T) {
	root := repositoryRoot(t)
	modelDir := filepath.Join(root, "internal", "adapter", "out", "model")
	reserved := map[string]struct{}{
		"Client":        {},
		"Request":       {},
		"Response":      {},
		"Event":         {},
		"Stream":        {},
		"ToolCall":      {},
		"Usage":         {},
		"ModelMetadata": {},
		"ProviderError": {},
	}

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
				if !ok || typeSpec.Assign.IsValid() {
					continue
				}
				if _, blocked := reserved[typeSpec.Name.Name]; blocked {
					t.Errorf("model adapter redefines SDK-owned boundary type %s in %s", typeSpec.Name.Name, entry.Name())
				}
			}
		}
	}
}

// TestSDKOwnsProviderProtocol pins that proton-sdk owns the OpenAI and
// Anthropic wire protocols: no official OpenAI SDK dependency may return and
// legacy protocol implementations must stay deleted.
func TestSDKOwnsProviderProtocol(t *testing.T) {
	root := repositoryRoot(t)
	goMod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(goMod), "github.com/openai/openai-go") {
		t.Fatal("official OpenAI SDK dependency must not return; proton-sdk owns the OpenAI protocol")
	}
	for _, legacy := range []string{"openai_request.go", "openai_stream.go", "openai_official.go", "openai_official_chat.go"} {
		legacyPath := filepath.Join(root, "internal", "adapter", "out", "model", legacy)
		if _, err := os.Stat(legacyPath); err == nil {
			t.Fatalf("legacy protocol implementation returned: internal/adapter/out/model/%s", legacy)
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
}

// TestSDKDoesNotOwnAgentLoopPolicy pins that generic agent-loop stop policy
// stays in the runtime turn engine, not in the provider SDK.
func TestSDKDoesNotOwnAgentLoopPolicy(t *testing.T) {
	root := repositoryRoot(t)
	sdkDir := filepath.Join(root, "pkg", "proton-sdk")
	forbidden := map[string]bool{
		"StepContext": true, "StopCondition": true, "StopAfterSteps": true,
		"StopWhenNoToolCalls": true, "ShouldStop": true,
	}
	set := token.NewFileSet()
	err := filepath.WalkDir(sdkDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(set, path, nil, 0)
		if err != nil {
			return err
		}
		for _, decl := range file.Decls {
			switch node := decl.(type) {
			case *ast.GenDecl:
				for _, spec := range node.Specs {
					if typeSpec, ok := spec.(*ast.TypeSpec); ok && forbidden[typeSpec.Name.Name] {
						t.Errorf("agent-loop policy %s must live outside proton-sdk: %s", typeSpec.Name.Name, path)
					}
				}
			case *ast.FuncDecl:
				if forbidden[node.Name.Name] {
					t.Errorf("agent-loop policy %s must live outside proton-sdk: %s", node.Name.Name, path)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
