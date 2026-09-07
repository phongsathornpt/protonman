package architecture_test

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

type listedPackage struct {
	ImportPath string
	Imports    []string
}

const modulePath = "github.com/projectTHORN/proton"

func TestCorePackagesDoNotDependOnOuterLayers(t *testing.T) {
	packages := listPackages(t)
	outer := []string{
		modulePath + "/cmd/proton",
		modulePath + "/internal/acp",
		modulePath + "/internal/agent",
		modulePath + "/internal/headless",
		modulePath + "/internal/tool/builtin",
		modulePath + "/internal/toolcall",
		modulePath + "/internal/tui",
		modulePath + "/internal/turn",
	}
	for _, core := range []string{
		modulePath + "/internal/modelprofile",
		modulePath + "/internal/permission",
		modulePath + "/internal/runtimepolicy",
		modulePath + "/internal/session",
		modulePath + "/internal/tool",
		modulePath + "/internal/workspace",
	} {
		assertNoImports(t, packages, core, outer)
	}
}

func TestInboundAdaptersUseApplicationConversationBoundary(t *testing.T) {
	packages := listPackages(t)
	for _, adapter := range []string{
		modulePath + "/internal/acp",
		modulePath + "/internal/headless",
		modulePath + "/internal/tui",
	} {
		assertNoImports(t, packages, adapter, []string{modulePath + "/internal/turn"})
	}
}

func TestBuiltinToolsDoNotDependOnFeatureSubsystems(t *testing.T) {
	packages := listPackages(t)
	assertNoImports(t, packages, modulePath+"/internal/tool/builtin", []string{
		"net/http",
		modulePath + "/internal/agent",
		modulePath + "/internal/buildinfo",
		modulePath + "/internal/skill",
		modulePath + "/internal/todo",
	})
}

func TestSessionDomainDoesNotOwnFilesystemPersistence(t *testing.T) {
	packages := listPackages(t)
	assertNoImports(t, packages, modulePath+"/internal/session", []string{
		"os",
		modulePath + "/internal/adapter/sessionfs",
	})
}

func TestTUIDoesNotDependOnSessionPersistenceDomain(t *testing.T) {
	packages := listPackages(t)
	assertNoImports(t, packages, modulePath+"/internal/tui", []string{
		modulePath + "/internal/session",
	})
}

func TestTUIDoesNotPerformProviderDiscoveryDirectly(t *testing.T) {
	root := repositoryRoot(t)
	cmd := exec.Command("rg", "FetchProviderModels", "internal/tui", "--glob", "*.go")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("TUI performs provider discovery directly:\n%s", output)
	}
	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 1 {
		t.Fatalf("search TUI provider discovery: %v: %s", err, output)
	}
}

func TestTUIDoesNotMutateProjectConfigPersistenceDirectly(t *testing.T) {
	root := repositoryRoot(t)
	cmd := exec.Command("rg", "config\\.SaveProject", "internal/tui", "--glob", "*.go")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("TUI mutates project config persistence directly:\n%s", output)
	}
	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 1 {
		t.Fatalf("search TUI project config mutations: %v: %s", err, output)
	}
}

func TestApplicationDoesNotDependOnInboundAdapters(t *testing.T) {
	packages := listPackages(t)
	assertNoImports(t, packages, modulePath+"/internal/app", []string{
		modulePath + "/cmd/proton",
		modulePath + "/internal/acp",
		modulePath + "/internal/headless",
		modulePath + "/internal/tui",
	})
}

func TestSDKDoesNotDependOnCLIInternals(t *testing.T) {
	packages := listPackages(t)
	for path := range packages {
		if path != modulePath+"/proton-sdk" && !strings.HasPrefix(path, modulePath+"/proton-sdk/") {
			continue
		}
		for _, imported := range packages[path].Imports {
			if strings.HasPrefix(imported, modulePath+"/internal/") || strings.HasPrefix(imported, modulePath+"/cmd/") {
				t.Errorf("%s imports CLI-owned package %s", path, imported)
			}
		}
	}
}

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
			t.Errorf("core package %s imports outer-layer package %s", source, imported)
		}
	}
}

func listPackages(t *testing.T) map[string]listedPackage {
	t.Helper()
	root := repositoryRoot(t)
	cmd := exec.Command("go", "list", "-json", "./...")
	cmd.Dir = root
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list ./...: %v", err)
	}
	dec := json.NewDecoder(strings.NewReader(string(output)))
	packages := map[string]listedPackage{}
	for dec.More() {
		var pkg listedPackage
		if err := dec.Decode(&pkg); err != nil {
			t.Fatalf("decode go list output: %v", err)
		}
		sort.Strings(pkg.Imports)
		packages[pkg.ImportPath] = pkg
	}
	return packages
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve architecture test path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
