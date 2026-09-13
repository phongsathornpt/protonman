package architecture_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	pathpkg "path"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func assertNoImportedPackageCallPrefixes(t *testing.T, relativePath, targetImport string, forbiddenPrefixes []string, includeTests bool, message string) {
	t.Helper()
	root := repositoryRoot(t)
	target := filepath.Join(root, relativePath)
	matches := make([]string, 0)

	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("inspect %s: %v", relativePath, err)
	}
	visit := func(filePath string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() || !strings.HasSuffix(filePath, ".go") || (!includeTests && strings.HasSuffix(filePath, "_test.go")) {
			return nil
		}

		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, filePath, nil, 0)
		if err != nil {
			return err
		}
		aliases := map[string]struct{}{}
		for _, spec := range file.Imports {
			importPath, err := strconv.Unquote(spec.Path.Value)
			if err != nil || importPath != targetImport {
				continue
			}
			alias := pathpkg.Base(importPath)
			if spec.Name != nil {
				alias = spec.Name.Name
			}
			if alias == "_" {
				continue
			}
			if alias == "." {
				rel, relErr := filepath.Rel(root, filePath)
				if relErr != nil {
					return relErr
				}
				matches = append(matches, filepath.ToSlash(rel)+": dot-imports "+targetImport)
				continue
			}
			aliases[alias] = struct{}{}
		}
		if len(aliases) == 0 {
			return nil
		}

		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			ident, ok := selector.X.(*ast.Ident)
			if !ok {
				return true
			}
			if _, ok := aliases[ident.Name]; !ok {
				return true
			}
			for _, prefix := range forbiddenPrefixes {
				if strings.HasPrefix(selector.Sel.Name, prefix) {
					rel, relErr := filepath.Rel(root, filePath)
					if relErr == nil {
						matches = append(matches, filepath.ToSlash(rel)+":"+selector.Sel.Name)
					}
					break
				}
			}
			return true
		})
		return nil
	}

	if info.IsDir() {
		err = filepath.Walk(target, visit)
	} else {
		err = visit(target, info, nil)
	}
	if err != nil {
		t.Fatalf("scan %s: %v", relativePath, err)
	}
	if len(matches) > 0 {
		t.Fatalf("%s: %s", message, strings.Join(matches, ", "))
	}
}

func assertNoCallsThroughField(t *testing.T, relativePath, fieldName string, includeTests bool, message string) {
	t.Helper()
	root := repositoryRoot(t)
	target := filepath.Join(root, relativePath)
	matches := make([]string, 0)

	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("inspect %s: %v", relativePath, err)
	}
	visit := func(filePath string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() || !strings.HasSuffix(filePath, ".go") || (!includeTests && strings.HasSuffix(filePath, "_test.go")) {
			return nil
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, filePath, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			method, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			field, ok := method.X.(*ast.SelectorExpr)
			if !ok || field.Sel.Name != fieldName {
				return true
			}
			rel, relErr := filepath.Rel(root, filePath)
			if relErr == nil {
				matches = append(matches, filepath.ToSlash(rel)+":"+fieldName+"."+method.Sel.Name)
			}
			return true
		})
		return nil
	}

	if info.IsDir() {
		err = filepath.Walk(target, visit)
	} else {
		err = visit(target, info, nil)
	}
	if err != nil {
		t.Fatalf("scan %s: %v", relativePath, err)
	}
	if len(matches) > 0 {
		t.Fatalf("%s: %s", message, strings.Join(matches, ", "))
	}
}

func assertNoMethod(t *testing.T, relativePath, receiverType, methodName string, includeTests bool, message string) {
	t.Helper()
	root := repositoryRoot(t)
	target := filepath.Join(root, relativePath)
	matches := make([]string, 0)

	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("inspect %s: %v", relativePath, err)
	}
	visit := func(filePath string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() || !strings.HasSuffix(filePath, ".go") || (!includeTests && strings.HasSuffix(filePath, "_test.go")) {
			return nil
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, filePath, nil, 0)
		if err != nil {
			return err
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Name.Name != methodName || len(fn.Recv.List) == 0 {
				continue
			}
			if receiverBaseName(fn.Recv.List[0].Type) != receiverType {
				continue
			}
			rel, relErr := filepath.Rel(root, filePath)
			if relErr != nil {
				return relErr
			}
			matches = append(matches, filepath.ToSlash(rel)+":"+receiverType+"."+methodName)
		}
		return nil
	}

	if info.IsDir() {
		err = filepath.Walk(target, visit)
	} else {
		err = visit(target, info, nil)
	}
	if err != nil {
		t.Fatalf("scan %s: %v", relativePath, err)
	}
	if len(matches) > 0 {
		t.Fatalf("%s: %s", message, strings.Join(matches, ", "))
	}
}

func receiverBaseName(expr ast.Expr) string {
	switch value := expr.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.StarExpr:
		return receiverBaseName(value.X)
	case *ast.IndexExpr:
		return receiverBaseName(value.X)
	case *ast.IndexListExpr:
		return receiverBaseName(value.X)
	default:
		return ""
	}
}
