package architecture_test

import (
	"encoding/json"
	"os"
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
		modulePath + "/internal/adapter/tool/builtin",
		modulePath + "/internal/agent",
		modulePath + "/internal/headless",
		modulePath + "/internal/toolcall",
		modulePath + "/internal/tui",
		modulePath + "/internal/turn",
	}
	for _, core := range []string{
		modulePath + "/internal/modelprofile",
		modulePath + "/internal/permission",
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
	assertNoImports(t, packages, modulePath+"/internal/adapter/tool/builtin", []string{
		"net/http",
		modulePath + "/internal/agent",
		modulePath + "/internal/base/buildinfo",
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

func TestTUIDoesNotDependOnProjectDirectly(t *testing.T) {
	packages := listPackages(t)
	assertNoImports(t, packages, modulePath+"/internal/tui", []string{
		modulePath + "/internal/project",
	})
}

func TestTUIDoesNotControlAgentCoordinatorDirectly(t *testing.T) {
	root := repositoryRoot(t)
	cmd := exec.Command("rg", "(m|ui)\\.coordinator\\.[A-Z]", "internal/tui", "--glob", "*.go", "--glob", "!*_test.go")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("TUI controls agent coordinator directly:\n%s", output)
	}
	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 1 {
		t.Fatalf("search TUI coordinator controls: %v: %s", err, output)
	}
}

func TestInboundAdaptersDoNotPerformConfigPersistence(t *testing.T) {
	root := repositoryRoot(t)
	for _, adapter := range []string{"internal/tui", "internal/acp", "internal/headless"} {
		cmd := exec.Command("rg", "config\\.(Load|Save|Delete)", adapter, "--glob", "*.go", "--glob", "!*_test.go")
		cmd.Dir = root
		output, err := cmd.CombinedOutput()
		if err == nil {
			t.Errorf("%s performs config persistence directly:\n%s", adapter, output)
			continue
		}
		if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 1 {
			t.Fatalf("search %s config persistence: %v: %s", adapter, err, output)
		}
	}
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

func TestTUIDoesNotMutateUserProviderConfigDirectly(t *testing.T) {
	root := repositoryRoot(t)
	cmd := exec.Command("rg", "config\\.(SaveUser|DeleteUser)", "internal/tui", "--glob", "*.go", "--glob", "!*_test.go")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("TUI mutates user provider config directly:\n%s", output)
	}
	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 1 {
		t.Fatalf("search TUI user provider config mutations: %v: %s", err, output)
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

func TestApplicationDoesNotExposeAgentCoordinatorEscapeHatch(t *testing.T) {
	root := repositoryRoot(t)
	cmd := exec.Command("rg", `func \(.*Agents\) Coordinator\(\)`, "internal/app", "--glob", "*.go")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("application exposes concrete agent coordinator escape hatch:\n%s", output)
	}
	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 1 {
		t.Fatalf("search application coordinator escape hatch: %v: %s", err, output)
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

func TestHeadlessModeDoesNotDependOnTurn(t *testing.T) {
	root := repositoryRoot(t)
	cmd := exec.Command("rg", `"github\.com/projectTHORN/proton/internal/turn"`, "cmd/proton/headless_mode.go")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("cmd/proton/headless_mode.go imports internal/turn directly:\n%s", output)
	}
	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 1 {
		t.Fatalf("search headless_mode.go turn imports: %v: %s", err, output)
	}
}

func TestApplicationLayerFileStructure(t *testing.T) {
	root := repositoryRoot(t)
	entries, err := os.ReadDir(filepath.Join(root, "internal", "app"))
	if err != nil {
		t.Fatalf("read internal/app: %v", err)
	}
	expected := map[string]bool{
		"agents.go":            true,
		"appdirs":              true,
		"conversation.go":      true,
		"conversation_test.go": true,
		"models.go":            true,
		"projects.go":          true,
		"providers.go":         true,
		"sessions.go":          true,
	}
	for _, entry := range entries {
		if !expected[entry.Name()] {
			t.Errorf("unexpected entry in internal/app: %s", entry.Name())
		}
	}
}

func TestModelLayerFileStructure(t *testing.T) {
	root := repositoryRoot(t)
	entries, err := os.ReadDir(filepath.Join(root, "internal", "model"))
	if err != nil {
		t.Fatalf("read internal/model: %v", err)
	}
	expected := map[string]bool{
		"client_factory.go":      true,
		"client_factory_test.go": true,
		"model_profile.go":       true,
		"model_test.go":          true,
		"provider_catalog.go":    true,
		"provider_discovery.go":  true,
		"provider_preset.go":     true,
		"provider_test.go":       true,
		"sdk_adapter.go":         true,
		"sdk_adapter_test.go":    true,
	}
	for _, entry := range entries {
		if !expected[entry.Name()] {
			t.Errorf("unexpected entry in internal/model: %s", entry.Name())
		}
	}
}

func TestNoDomainIshDirectory(t *testing.T) {
	root := repositoryRoot(t)
	path := filepath.Join(root, "internal", "domain-ish")
	if _, err := os.Stat(path); err == nil {
		t.Errorf("internal/domain-ish directory must not exist")
	}
}

func TestAdapterToolDirectoryStructure(t *testing.T) {
	root := repositoryRoot(t)
	entries, err := os.ReadDir(filepath.Join(root, "internal", "adapter", "tool"))
	if err != nil {
		t.Fatalf("read internal/adapter/tool: %v", err)
	}
	expected := map[string]bool{
		"agent":   true,
		"builtin": true,
		"mcp":     true,
		"skill":   true,
		"todo":    true,
		"web":     true,
	}
	for _, entry := range entries {
		if !expected[entry.Name()] {
			t.Errorf("unexpected entry in internal/adapter/tool: %s", entry.Name())
		}
	}
}

func TestNoToolBuiltinDirectory(t *testing.T) {
	root := repositoryRoot(t)
	path := filepath.Join(root, "internal", "tool", "builtin")
	if _, err := os.Stat(path); err == nil {
		t.Errorf("internal/tool/builtin directory must not exist; moved to internal/adapter/tool/builtin")
	}
}

func TestNoRootMCPDirectory(t *testing.T) {
	root := repositoryRoot(t)
	path := filepath.Join(root, "internal", "mcp")
	if _, err := os.Stat(path); err == nil {
		t.Errorf("internal/mcp directory must not exist at internal root; moved to internal/adapter/tool/mcp")
	}
}

func TestBasePackagesHaveZeroInternalDependencies(t *testing.T) {
	packages := listPackages(t)
	basePackages := []string{
		modulePath + "/internal/base/buildinfo",
		modulePath + "/internal/base/contextutil",
		modulePath + "/internal/base/envconfig",
		modulePath + "/internal/base/failure",
		modulePath + "/internal/base/glob",
		modulePath + "/internal/base/runtimepolicy",
	}
	for _, basePkg := range basePackages {
		pkg, ok := packages[basePkg]
		if !ok {
			t.Fatalf("base package %s not found", basePkg)
		}
		for _, imported := range pkg.Imports {
			if strings.HasPrefix(imported, modulePath+"/internal/") || strings.HasPrefix(imported, modulePath+"/cmd/") {
				t.Errorf("base leaf package %s must not import internal package %s", basePkg, imported)
			}
		}
	}
}

func TestNoLingeringRootFoundationDirectories(t *testing.T) {
	root := repositoryRoot(t)
	for _, lingering := range []string{"buildinfo", "contextutil", "envconfig", "failure", "glob", "runtimepolicy"} {
		path := filepath.Join(root, "internal", lingering)
		if _, err := os.Stat(path); err == nil {
			t.Errorf("internal/%s must not exist at internal root; moved to internal/base/%s", lingering, lingering)
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
