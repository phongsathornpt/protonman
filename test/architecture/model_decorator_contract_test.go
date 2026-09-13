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

func TestModelDecoratorsPreserveMetadataBoundary(t *testing.T) {
	root := repositoryRoot(t)
	contracts := map[string][]string{
		filepath.Join(root, "internal", "adapter", "out", "model"): {
			"sessionBoundModel",
			"profiledLanguageModel",
			"lowConcurrencyModel",
			"emptyStreamRetryModel",
			"capabilityOverrideModel",
		},
		filepath.Join(root, "internal", "feature", "memory"): {
			"memoryLanguageModel",
		},
	}

	for dir, typeNames := range contracts {
		methods := packageMethods(t, dir)
		for _, typeName := range typeNames {
			if _, ok := methods[typeName]["Metadata"]; !ok {
				t.Errorf("model decorator %s must implement Metadata() to preserve the canonical SDK metadata boundary", typeName)
			}
		}
	}
}

func packageMethods(t *testing.T, dir string) map[string]map[string]struct{} {
	t.Helper()
	result := map[string]map[string]struct{}{}
	set := token.NewFileSet()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(set, filepath.Join(dir, entry.Name()), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || len(fn.Recv.List) != 1 {
				continue
			}
			typeName := receiverTypeName(fn.Recv.List[0].Type)
			if typeName == "" {
				continue
			}
			if result[typeName] == nil {
				result[typeName] = map[string]struct{}{}
			}
			result[typeName][fn.Name.Name] = struct{}{}
		}
	}
	return result
}

func receiverTypeName(expr ast.Expr) string {
	switch node := expr.(type) {
	case *ast.Ident:
		return node.Name
	case *ast.StarExpr:
		if ident, ok := node.X.(*ast.Ident); ok {
			return ident.Name
		}
	}
	return ""
}
