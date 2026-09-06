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

func TestProviderProtocolOwnership(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(currentFile), ".."))
	goMod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(goMod), "github.com/openai/openai-go") {
		t.Fatal("official OpenAI SDK dependency must not return; proton-sdk owns the OpenAI protocol")
	}

	forbiddenTypes := map[string]bool{"Client": true, "Request": true, "Event": true, "Stream": true}
	modelDir := filepath.Join(root, "internal", "model")
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
		if _, err := os.Stat(filepath.Join(root, "internal", "model", legacy)); err == nil {
			t.Fatalf("legacy protocol implementation returned: internal/model/%s", legacy)
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
}
