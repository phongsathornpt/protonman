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
