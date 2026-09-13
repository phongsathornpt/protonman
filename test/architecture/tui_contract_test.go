package architecture_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTUIFacadeOnlyDependsOnRuntime(t *testing.T) {
	packages := listPackages(t)
	root := modulePath + "/internal/adapter/in/tui"
	pkg, ok := packages[root]
	if !ok {
		t.Fatalf("package %s not found", root)
	}
	allowed := modulePath + "/internal/adapter/in/tui/runtime"
	for _, imported := range pkg.Imports {
		if strings.HasPrefix(imported, modulePath+"/") && imported != allowed {
			t.Errorf("TUI facade imports %s; only runtime package is allowed", imported)
		}
	}
}

func TestTUIViewArchitectureContract(t *testing.T) {
	root := repositoryRoot(t)
	viewRoot := filepath.Join(root, "internal", "adapter", "in", "tui", "view")
	entries, err := os.ReadDir(viewRoot)
	if err != nil {
		t.Fatalf("read TUI view root: %v", err)
	}
	allowedViews := map[string]bool{
		"diagnostic":   true,
		"execview":     true,
		"history":      true,
		"pane":         true,
		"presentation": true,
		"slashview":    true,
		"style":        true,
		"textview":     true,
		"toolview":     true,
	}
	for _, entry := range entries {
		if !entry.IsDir() || !allowedViews[entry.Name()] {
			t.Errorf("unexpected TUI view entry %s", entry.Name())
		}
	}
}

func TestTUISubpackagesNeverImportPresentationRoot(t *testing.T) {
	packages := listPackages(t)
	root := modulePath + "/internal/adapter/in/tui"
	prefix := root + "/"
	for pkgPath, pkg := range packages {
		if !strings.HasPrefix(pkgPath, prefix) {
			continue
		}
		for _, imported := range pkg.Imports {
			if imported == root {
				t.Errorf("TUI subpackage %s imports parent presentation package %s", pkgPath, imported)
			}
		}
	}
}

func TestTUIPaneDoesNotOwnApplicationServices(t *testing.T) {
	packages := listPackages(t)
	prefix := modulePath + "/internal/adapter/in/tui/view/pane/"
	for pkgPath := range packages {
		if !strings.HasPrefix(pkgPath, prefix) {
			continue
		}
		assertNoImports(t, packages, pkgPath, []string{
			modulePath + "/internal/adapter/in/tui",
			modulePath + "/internal/adapter/out/config",
			modulePath + "/internal/app",
			modulePath + "/internal/engine/turn",
		})
	}
}

func TestTUISlashViewDoesNotOwnRuntimeState(t *testing.T) {
	packages := listPackages(t)
	pkgPath := modulePath + "/internal/adapter/in/tui/view/slashview"
	pkg, ok := packages[pkgPath]
	if !ok {
		t.Fatalf("package %s not found", pkgPath)
	}
	allowed := map[string]bool{
		modulePath + "/internal/adapter/in/tui/view/style":    true,
		modulePath + "/internal/adapter/in/tui/view/textview": true,
		modulePath + "/internal/core/tool":                    true,
	}
	for _, imported := range pkg.Imports {
		if strings.HasPrefix(imported, modulePath+"/internal/") && !allowed[imported] {
			t.Errorf("TUI slashview imports forbidden runtime package %s", imported)
		}
	}
}

func TestTUIHistoryDependsOnlyOnPresentationAndDomainLeaves(t *testing.T) {
	packages := listPackages(t)
	pkgPath := modulePath + "/internal/adapter/in/tui/view/history"
	pkg, ok := packages[pkgPath]
	if !ok {
		t.Fatalf("package %s not found", pkgPath)
	}
	allowed := map[string]bool{
		modulePath + "/internal/adapter/in/tui/view/diagnostic": true,
		modulePath + "/internal/adapter/in/tui/view/execview":   true,
		modulePath + "/internal/adapter/in/tui/view/style":      true,
		modulePath + "/internal/adapter/in/tui/view/textview":   true,
		modulePath + "/internal/adapter/in/tui/view/toolview":   true,
		modulePath + "/internal/core/tool":                      true,
		modulePath + "/internal/feature/agent":                  true,
	}
	for _, imported := range pkg.Imports {
		if strings.HasPrefix(imported, modulePath+"/internal/") && !allowed[imported] {
			t.Errorf("TUI history imports forbidden application package %s", imported)
		}
	}
}

func TestTUIToolViewDoesNotDependOnPresentationRoot(t *testing.T) {
	packages := listPackages(t)
	assertNoImports(t, packages, modulePath+"/internal/adapter/in/tui/view/toolview", []string{
		modulePath + "/internal/adapter/in/tui",
		modulePath + "/internal/app",
		modulePath + "/internal/engine/turn",
	})
}

func TestTUIDiagnosticDoesNotDependOnPresentationRoot(t *testing.T) {
	packages := listPackages(t)
	assertNoImports(t, packages, modulePath+"/internal/adapter/in/tui/view/diagnostic", []string{
		modulePath + "/internal/adapter/in/tui",
		modulePath + "/internal/adapter/in/tui/view/history",
		modulePath + "/internal/adapter/in/tui/view/pane",
	})
}

func TestTUIStyleAndTextViewDependenciesStayAcyclic(t *testing.T) {
	packages := listPackages(t)
	stylePath := modulePath + "/internal/adapter/in/tui/view/style"
	textPath := modulePath + "/internal/adapter/in/tui/view/textview"

	stylePkg, ok := packages[stylePath]
	if !ok {
		t.Fatalf("package %s not found", stylePath)
	}
	for _, imported := range stylePkg.Imports {
		if strings.HasPrefix(imported, modulePath+"/internal/") || strings.HasPrefix(imported, modulePath+"/cmd/") {
			t.Errorf("TUI style leaf imports application package %s", imported)
		}
	}

	textPkg, ok := packages[textPath]
	if !ok {
		t.Fatalf("package %s not found", textPath)
	}
	for _, imported := range textPkg.Imports {
		if imported == stylePath {
			continue
		}
		if strings.HasPrefix(imported, modulePath+"/internal/") || strings.HasPrefix(imported, modulePath+"/cmd/") {
			t.Errorf("TUI textview imports forbidden application package %s", imported)
		}
	}
}

func TestTUIExecViewIsPresentationLeaf(t *testing.T) {
	packages := listPackages(t)
	pkgPath := modulePath + "/internal/adapter/in/tui/view/execview"
	pkg, ok := packages[pkgPath]
	if !ok {
		t.Fatalf("package %s not found", pkgPath)
	}
	for _, imported := range pkg.Imports {
		if strings.HasPrefix(imported, modulePath+"/internal/") || strings.HasPrefix(imported, modulePath+"/cmd/") {
			t.Errorf("TUI execution presentation leaf %s imports application package %s", pkgPath, imported)
		}
	}
}

func TestTUIDoesNotControlAgentCoordinatorDirectly(t *testing.T) {
	assertNoCallsThroughField(t, "internal/adapter/in/tui", "coordinator", false, "TUI controls agent coordinator directly")
}

func TestTUIRuntimeSubpackagesDoNotImportRuntimeRoot(t *testing.T) {
	packages := listPackages(t)
	runtimeRoot := modulePath + "/internal/adapter/in/tui/runtime"
	for importPath, pkg := range packages {
		if !strings.HasPrefix(importPath, runtimeRoot+"/") {
			continue
		}
		for _, imported := range pkg.Imports {
			if imported == runtimeRoot {
				t.Errorf("TUI runtime subpackage %s must not import root runtime package %s; keep dependencies flowing from orchestration shell into focused ownership packages", importPath, runtimeRoot)
			}
		}
	}
}

func TestTUIRuntimeRootStaysWithinStructuralBudget(t *testing.T) {
	root := repositoryRoot(t)
	runtimeRoot := filepath.Join(root, "internal", "adapter", "in", "tui", "runtime")
	entries, err := os.ReadDir(runtimeRoot)
	if err != nil {
		t.Fatalf("read TUI runtime root: %v", err)
	}
	const maxProductionFiles = 32
	productionFiles := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		productionFiles++
	}
	if productionFiles > maxProductionFiles {
		t.Fatalf("TUI runtime root has %d production files; structural budget is %d; extract ownership into focused subpackages instead of growing the root", productionFiles, maxProductionFiles)
	}
}
