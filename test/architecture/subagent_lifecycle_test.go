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

func TestSubagentLifecycleStateWritesStayInReducer(t *testing.T) {
	root := filepath.Join(repositoryRoot(t), "internal", "feature", "agent")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read agent package: %v", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || name == "transition.go" {
			continue
		}
		assertNoLifecycleStateAssignment(t, filepath.Join(root, name))
	}
}
func assertNoLifecycleStateAssignment(t *testing.T, path string) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	ast.Inspect(file, func(node ast.Node) bool {
		assign, ok := node.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for _, lhs := range assign.Lhs {
			selector, ok := lhs.(*ast.SelectorExpr)
			if !ok || selector.Sel == nil || selector.Sel.Name != "State" {
				continue
			}
			pos := fset.Position(selector.Pos())
			t.Errorf("direct AgentStatus.State assignment outside reducer at %s:%d", filepath.Base(path), pos.Line)
		}
		return true
	})
}
