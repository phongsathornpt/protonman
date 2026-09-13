package architecture_test

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
)

type listedPackage struct {
	ImportPath string
	Imports    []string
}

const modulePath = "github.com/phongsathornpt/protonman"

var (
	packageGraphOnce sync.Once
	packageGraph     map[string]listedPackage
	packageGraphErr  error
)

func assertNoImports(t *testing.T, packages map[string]listedPackage, source string, forbidden []string) {
	t.Helper()
	pkg, ok := packages[source]
	if !ok {
		t.Fatalf("package %s not found", source)
	}
	blocked := make(map[string]struct{}, len(forbidden))
	for _, path := range forbidden {
		blocked[path] = struct{}{}
	}
	for _, imported := range pkg.Imports {
		if _, exists := blocked[imported]; exists {
			t.Errorf("package %s imports forbidden package %s", source, imported)
		}
	}
}

func assertNoImportPrefixes(t *testing.T, packages map[string]listedPackage, sourcePrefix string, forbiddenPrefixes []string) {
	t.Helper()
	foundSource := false
	for importPath, pkg := range packages {
		if !packageWithin(importPath, sourcePrefix) {
			continue
		}
		foundSource = true
		for _, imported := range pkg.Imports {
			for _, forbidden := range forbiddenPrefixes {
				if packageWithin(imported, forbidden) {
					t.Errorf("package %s imports forbidden package %s", importPath, imported)
				}
			}
		}
	}
	if !foundSource {
		t.Fatalf("package family %s not found", sourcePrefix)
	}
}

func assertPackageNoImportPrefixes(t *testing.T, packages map[string]listedPackage, source string, forbiddenPrefixes []string) {
	t.Helper()
	pkg, ok := packages[source]
	if !ok {
		t.Fatalf("package %s not found", source)
	}
	for _, imported := range pkg.Imports {
		for _, forbidden := range forbiddenPrefixes {
			if packageWithin(imported, forbidden) {
				t.Errorf("package %s imports forbidden package %s", source, imported)
			}
		}
	}
}

func packageWithin(importPath, packagePrefix string) bool {
	prefix := strings.TrimSuffix(packagePrefix, "/")
	return importPath == prefix || strings.HasPrefix(importPath, prefix+"/")
}

func listPackages(t *testing.T) map[string]listedPackage {
	t.Helper()
	root := repositoryRoot(t)
	packageGraphOnce.Do(func() {
		packageGraph, packageGraphErr = loadPackages(root)
	})
	if packageGraphErr != nil {
		t.Fatalf("load package graph: %v", packageGraphErr)
	}
	return packageGraph
}

func loadPackages(root string) (map[string]listedPackage, error) {
	cmd := exec.Command("go", "list", "-json", "./...")
	cmd.Dir = root
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list ./...: %w", err)
	}
	dec := json.NewDecoder(strings.NewReader(string(output)))
	packages := map[string]listedPackage{}
	for dec.More() {
		var pkg listedPackage
		if err := dec.Decode(&pkg); err != nil {
			return nil, fmt.Errorf("decode go list output: %w", err)
		}
		sort.Strings(pkg.Imports)
		packages[pkg.ImportPath] = pkg
	}
	return packages, nil
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve architecture test path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
