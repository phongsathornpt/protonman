package architecture_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
)

type listedPackage struct {
	ImportPath string
	Imports    []string
}

const modulePath = "github.com/phongsathornpt/protonman"

func TestCorePackagesDoNotDependOnOuterLayers(t *testing.T) {
	packages := listPackages(t)
	outer := []string{
		modulePath + "/cmd/protonman",
		modulePath + "/internal/adapter/in/acp",
		modulePath + "/internal/adapter/in/headless",
		modulePath + "/internal/adapter/in/tui",
		modulePath + "/internal/adapter/out/config",
		modulePath + "/internal/adapter/out/model",
		modulePath + "/internal/adapter/out/sessionfs",
		modulePath + "/internal/adapter/out/tool/builtin",
		modulePath + "/internal/feature/agent",
		modulePath + "/internal/engine/prompt",
		modulePath + "/internal/engine/toolcall",
		modulePath + "/internal/engine/turn",
	}
	for _, core := range []string{
		modulePath + "/internal/core/modelprofile",
		modulePath + "/internal/core/permission",
		modulePath + "/internal/core/session",
		modulePath + "/internal/core/tool",
		modulePath + "/internal/core/workspace",
	} {
		assertNoImports(t, packages, core, outer)
	}
}

func TestInboundAdaptersUseApplicationConversationBoundary(t *testing.T) {
	packages := listPackages(t)
	for _, adapter := range []string{
		modulePath + "/internal/adapter/in/acp",
		modulePath + "/internal/adapter/in/headless",
		modulePath + "/internal/adapter/in/tui",
		modulePath + "/internal/adapter/in/tui/runtime",
	} {
		assertNoImports(t, packages, adapter, []string{modulePath + "/internal/engine/turn"})
	}
}

func TestBuiltinToolsDoNotDependOnFeatureSubsystems(t *testing.T) {
	packages := listPackages(t)
	assertNoImports(t, packages, modulePath+"/internal/adapter/out/tool/builtin", []string{
		"net/http",
		modulePath + "/internal/feature/agent",
		modulePath + "/internal/base/buildinfo",
		modulePath + "/internal/feature/skill",
		modulePath + "/internal/feature/todo",
	})
}

func TestSessionDomainDoesNotOwnFilesystemPersistence(t *testing.T) {
	packages := listPackages(t)
	assertNoImports(t, packages, modulePath+"/internal/core/session", []string{
		"os",
		modulePath + "/internal/adapter/out/sessionfs",
	})
}

func TestTUIDoesNotDependOnSessionPersistenceDomain(t *testing.T) {
	packages := listPackages(t)
	for _, pkgPath := range []string{
		modulePath + "/internal/adapter/in/tui",
		modulePath + "/internal/adapter/in/tui/runtime",
	} {
		assertNoImports(t, packages, pkgPath, []string{modulePath + "/internal/core/session"})
	}
}

func TestTUIDoesNotDependOnProjectDirectly(t *testing.T) {
	packages := listPackages(t)
	for _, pkgPath := range []string{
		modulePath + "/internal/adapter/in/tui",
		modulePath + "/internal/adapter/in/tui/runtime",
	} {
		assertNoImports(t, packages, pkgPath, []string{modulePath + "/internal/feature/project"})
	}
}

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

func TestTUIFolderOrganization(t *testing.T) {
	root := repositoryRoot(t)
	tuiRoot := filepath.Join(root, "internal", "adapter", "in", "tui")
	entries, err := os.ReadDir(tuiRoot)
	if err != nil {
		t.Fatalf("read TUI root: %v", err)
	}
	allowedDirs := map[string]bool{"runtime": true, "state": true, "view": true}
	for _, entry := range entries {
		if entry.IsDir() {
			if !allowedDirs[entry.Name()] {
				t.Errorf("unexpected TUI root directory %s; group it under runtime, state, or view", entry.Name())
			}
			continue
		}
		if entry.Name() != "facade.go" {
			t.Errorf("unexpected TUI root file %s; root must only expose facade.go", entry.Name())
		}
	}

	viewRoot := filepath.Join(tuiRoot, "view")
	viewEntries, err := os.ReadDir(viewRoot)
	if err != nil {
		t.Fatalf("read TUI view root: %v", err)
	}
	allowedViews := map[string]bool{
		"diagnostic": true, "execview": true, "history": true, "pane": true,
		"slashview": true, "style": true, "textview": true, "toolview": true,
	}
	for _, entry := range viewEntries {
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
	pkgPath := modulePath + "/internal/adapter/in/tui/view/pane"
	assertNoImports(t, packages, pkgPath, []string{
		modulePath + "/internal/adapter/in/tui",
		modulePath + "/internal/adapter/out/config",
		modulePath + "/internal/app",
		modulePath + "/internal/engine/turn",
	})
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
	pkgPath := modulePath + "/internal/adapter/in/tui/view/toolview"
	assertNoImports(t, packages, pkgPath, []string{
		modulePath + "/internal/adapter/in/tui",
		modulePath + "/internal/app",
		modulePath + "/internal/engine/turn",
	})
}

func TestTUIDiagnosticDoesNotDependOnPresentationRoot(t *testing.T) {
	packages := listPackages(t)
	pkgPath := modulePath + "/internal/adapter/in/tui/view/diagnostic"
	assertNoImports(t, packages, pkgPath, []string{
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
	assertNoSourceMatch(t, "internal/adapter/in/tui", `(m|ui)\.coordinator\.[A-Z]`, false, "TUI controls agent coordinator directly")
}

func TestInboundAdaptersDoNotPerformConfigPersistence(t *testing.T) {
	for _, adapter := range []string{"internal/adapter/in/tui", "internal/adapter/in/acp", "internal/adapter/in/headless"} {
		assertNoSourceMatch(t, adapter, `config\.(Load|Save|Delete)`, false, adapter+" performs config persistence directly")
	}
}

func TestTUIDoesNotPerformProviderDiscoveryDirectly(t *testing.T) {
	assertNoSourceMatch(t, "internal/adapter/in/tui", `FetchProviderModels`, true, "TUI performs provider discovery directly")
}

func TestTUIDoesNotMutateUserProviderConfigDirectly(t *testing.T) {
	assertNoSourceMatch(t, "internal/adapter/in/tui", `config\.(SaveUser|DeleteUser)`, false, "TUI mutates user provider config directly")
}

func TestTUIDoesNotMutateProjectConfigPersistenceDirectly(t *testing.T) {
	assertNoSourceMatch(t, "internal/adapter/in/tui", `config\.SaveProject`, true, "TUI mutates project config persistence directly")
}

func TestApplicationDoesNotExposeAgentCoordinatorEscapeHatch(t *testing.T) {
	assertNoSourceMatch(t, "internal/app", `func \(.*Agents\) Coordinator\(\)`, true, "application exposes concrete agent coordinator escape hatch")
}

func TestApplicationDoesNotDependOnInboundAdapters(t *testing.T) {
	packages := listPackages(t)
	assertNoImports(t, packages, modulePath+"/internal/app", []string{
		modulePath + "/cmd/protonman",
		modulePath + "/internal/adapter/in/acp",
		modulePath + "/internal/adapter/in/headless",
		modulePath + "/internal/adapter/in/tui",
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
	assertNoSourceMatch(t, "cmd/protonman/headless_mode.go", regexp.QuoteMeta(modulePath+"/internal/engine/turn"), true, "cmd/protonman/headless_mode.go imports internal/engine/turn directly")
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
		"user_settings.go":     true,
	}
	for _, entry := range entries {
		if !expected[entry.Name()] {
			t.Errorf("unexpected entry in internal/app: %s", entry.Name())
		}
	}
}

func TestModelLayerFileStructure(t *testing.T) {
	root := repositoryRoot(t)
	entries, err := os.ReadDir(filepath.Join(root, "internal", "adapter", "out", "model"))
	if err != nil {
		t.Fatalf("read internal/adapter/out/model: %v", err)
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
			t.Errorf("unexpected entry in internal/adapter/out/model: %s", entry.Name())
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
	entries, err := os.ReadDir(filepath.Join(root, "internal", "adapter", "out", "tool"))
	if err != nil {
		t.Fatalf("read internal/adapter/out/tool: %v", err)
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
			t.Errorf("unexpected entry in internal/adapter/out/tool: %s", entry.Name())
		}
	}
}

func TestNoToolBuiltinDirectory(t *testing.T) {
	root := repositoryRoot(t)
	path := filepath.Join(root, "internal", "tool", "builtin")
	if _, err := os.Stat(path); err == nil {
		t.Errorf("internal/tool/builtin directory must not exist; moved to internal/adapter/out/tool/builtin")
	}
}

func TestNoRootMCPDirectory(t *testing.T) {
	root := repositoryRoot(t)
	path := filepath.Join(root, "internal", "mcp")
	if _, err := os.Stat(path); err == nil {
		t.Errorf("internal/mcp directory must not exist at internal root; moved to internal/adapter/out/tool/mcp")
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

func TestInternalTopLevelCleanArchitectureDirectories(t *testing.T) {
	root := repositoryRoot(t)
	entries, err := os.ReadDir(filepath.Join(root, "internal"))
	if err != nil {
		t.Fatalf("read internal: %v", err)
	}
	expected := map[string]bool{
		"adapter":  true,
		"app":      true,
		"base":     true,
		"core":     true,
		"engine":   true,
		"feature":  true,
		"platform": true,
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			t.Errorf("unexpected file at internal root: %s", entry.Name())
			continue
		}
		if !expected[entry.Name()] {
			t.Errorf("unexpected directory at internal root: %s (should be organized into Clean Architecture groups)", entry.Name())
		}
	}
}

func TestNoLingeringRootDirectories(t *testing.T) {
	root := repositoryRoot(t)
	formerRootDirs := []string{
		"acp", "agent", "agentprompt", "architecture", "checkpoint", "config", "headless", "mcp", "model",
		"modelprofile", "permission", "project", "sandbox", "session", "skill", "telemetry",
		"todo", "tool", "toolcall", "tui", "turn", "workspace",
		"buildinfo", "contextutil", "envconfig", "failure", "glob", "runtimepolicy",
	}
	for _, lingering := range formerRootDirs {
		path := filepath.Join(root, "internal", lingering)
		if _, err := os.Stat(path); err == nil {
			t.Errorf("internal/%s must not exist at internal root; moved to Clean Architecture subpackages", lingering)
		}
	}
}

func assertNoSourceMatch(t *testing.T, relativePath, pattern string, includeTests bool, message string) {
	t.Helper()
	root := repositoryRoot(t)
	target := filepath.Join(root, relativePath)
	re := regexp.MustCompile(pattern)
	matches := make([]string, 0)

	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("inspect %s: %v", relativePath, err)
	}
	visit := func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") || (!includeTests && strings.HasSuffix(path, "_test.go")) {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if re.Match(body) {
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			matches = append(matches, filepath.ToSlash(rel))
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
