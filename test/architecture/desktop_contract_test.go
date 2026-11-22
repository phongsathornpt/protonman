package architecture_test

import (
	"strings"
	"testing"
)

// The Gio desktop frontend is a package tree, not a fixed list of packages: the
// shell root, the ACP controller, and every component package sit under one
// prefix. Deriving the set from that prefix means a new package is covered by
// every boundary assertion the moment it exists, instead of silently escaping
// the guards because someone forgot to register it.
const (
	desktopGioShellPrefix = modulePath + "/internal/adapter/in/desktop/gioui"
	desktopGioDesktopCmd  = modulePath + "/cmd/protonman-desktop-gio"
)

func desktopGioPackages(t *testing.T) map[string]listedPackage {
	t.Helper()
	packages := listDesktopGioPackages(t)
	selected := make(map[string]listedPackage)
	for importPath, pkg := range packages {
		if importPath == desktopGioShellPrefix ||
			strings.HasPrefix(importPath, desktopGioShellPrefix+"/") ||
			importPath == desktopGioDesktopCmd {
			selected[importPath] = pkg
		}
	}
	if _, ok := selected[desktopGioDesktopCmd]; !ok {
		t.Fatalf("Gio desktop composition root %s missing from the desktop package graph", desktopGioDesktopCmd)
	}
	if _, ok := selected[desktopGioShellPrefix]; !ok {
		t.Fatalf("Gio desktop shell %s missing from the desktop package graph", desktopGioShellPrefix)
	}
	return selected
}

// TestDesktopGioPackagesStayVisibleToGuards fails if the desktop build tag
// stops exposing a desktop package, which would silently drop the subsystem out
// of package-graph coverage.
func TestDesktopGioPackagesStayVisibleToGuards(t *testing.T) {
	packages := desktopGioPackages(t)
	// A tree that collapses to only the shell and the composition root means
	// the component and controller packages stopped being built.
	if len(packages) < 4 {
		t.Fatalf("Gio desktop package graph collapsed to %d packages; the component tree is no longer reachable", len(packages))
	}
	for importPath := range packages {
		if importPath == desktopGioDesktopCmd || importPath == desktopGioShellPrefix {
			continue
		}
		if !strings.HasPrefix(importPath, desktopGioShellPrefix+"/") {
			t.Errorf("desktop package %s matched the Gio prefix but sits outside the frontend tree", importPath)
		}
	}
}

// The Gio desktop frontend is a fourth inbound adapter and must keep the same
// boundary discipline as the TUI, ACP, and headless adapters: no direct turn
// engine, session persistence, or project manipulation.
func TestDesktopGioInboundAdapterUsesApplicationBoundary(t *testing.T) {
	assertPackageNoImportPrefixes(
		t,
		desktopGioPackages(t),
		desktopGioShellPrefix,
		[]string{
			modulePath + "/internal/engine/turn",
			modulePath + "/internal/core/session",
			modulePath + "/internal/adapter/out/sessionfs",
			modulePath + "/internal/feature/project",
		},
	)
}

// The Gio desktop frontend drives the CLI runtime over ACP instead of embedding
// a second agent loop, so the outbound ACP client is its only permitted driven
// adapter. A new outbound adapter dependency is an architecture decision that
// must be recorded deliberately rather than added silently.
func TestDesktopGioInboundAdapterOnlyDependsOnACPClient(t *testing.T) {
	packages := desktopGioPackages(t)
	pkg, ok := packages[desktopGioShellPrefix]
	if !ok {
		t.Fatalf("package %s not found in the Gio desktop package graph", desktopGioShellPrefix)
	}
	allowed := map[string]bool{
		modulePath + "/internal/adapter/out/acpclient": true,
	}
	for _, imported := range pkg.Imports {
		if !packageWithin(imported, modulePath+"/internal/adapter/out/") {
			continue
		}
		if !allowed[imported] {
			t.Errorf("Gio desktop inbound adapter imports unexpected driven adapter %s", imported)
		}
	}
}

// controller is the ACP-facing half of the frontend. It owns agent sessions and
// therefore talks to the ACP client directly; what it must never do is reach past
// the application boundary into the turn engine or session persistence, which
// would give the desktop a second agent loop.
func TestDesktopGioControllerOnlyDependsOnACPClient(t *testing.T) {
	packages := desktopGioPackages(t)
	controllerPath := desktopGioShellPrefix + "/controller"
	pkg, ok := packages[controllerPath]
	if !ok {
		t.Fatalf("package %s not found in the Gio desktop package graph", controllerPath)
	}
	allowed := map[string]bool{
		modulePath + "/internal/adapter/out/acpclient": true,
	}
	for _, imported := range pkg.Imports {
		if !packageWithin(imported, modulePath+"/internal/adapter/out/") {
			continue
		}
		if !allowed[imported] {
			t.Errorf("Gio desktop controller imports unexpected driven adapter %s", imported)
		}
	}
	for _, forbidden := range []string{
		modulePath + "/internal/engine/turn",
		modulePath + "/internal/core/session",
		modulePath + "/internal/adapter/out/sessionfs",
		modulePath + "/internal/feature/project",
	} {
		for _, imported := range pkg.Imports {
			if packageWithin(imported, forbidden) {
				t.Errorf("Gio desktop controller imports forbidden package %s", imported)
			}
		}
	}
}

// The controller owns application actions and the shell renders them. A
// controller that reached back into the shell would invert that direction and
// make the ACP state machine untestable without a Gio window.
func TestDesktopGioControllerDoesNotImportTheShell(t *testing.T) {
	packages := desktopGioPackages(t)
	controllerPath := desktopGioShellPrefix + "/controller"
	pkg, ok := packages[controllerPath]
	if !ok {
		t.Fatalf("package %s not found in the Gio desktop package graph", controllerPath)
	}
	for _, imported := range pkg.Imports {
		if imported == desktopGioShellPrefix || strings.HasPrefix(imported, desktopGioShellPrefix+"/shell") {
			t.Errorf("Gio desktop controller imports the presentation package %s", imported)
		}
	}
}

// shell owns presentation. It may read a controller snapshot, but it must obtain
// application actions through Bindings rather than importing a driven adapter,
// and it must not reach past the application boundary into the turn engine or
// session persistence.
func TestDesktopGioShellStaysInsideTheApplicationBoundary(t *testing.T) {
	packages := desktopGioPackages(t)
	const shellPath = desktopGioShellPrefix + "/shell"
	pkg, ok := packages[shellPath]
	if !ok {
		t.Fatalf("package %s not found in the Gio desktop package graph", shellPath)
	}
	for _, imported := range pkg.Imports {
		if packageWithin(imported, modulePath+"/internal/adapter/out/") &&
			imported != modulePath+"/internal/adapter/out/acpclient" {
			t.Errorf("Gio shell imports driven adapter %s; actions must arrive through Bindings", imported)
		}
		for _, forbidden := range []string{
			modulePath + "/internal/engine/turn",
			modulePath + "/internal/core/session",
			modulePath + "/internal/adapter/out/sessionfs",
			modulePath + "/internal/feature/project",
		} {
			if packageWithin(imported, forbidden) {
				t.Errorf("Gio shell imports forbidden package %s", imported)
			}
		}
	}
}

func TestDesktopGioComponentsUseApplicationBoundary(t *testing.T) {
	packages := desktopGioPackages(t)
	componentPrefix := desktopGioShellPrefix + "/component/"
	for importPath, pkg := range packages {
		if !strings.HasPrefix(importPath, componentPrefix) {
			continue
		}
		for _, imported := range pkg.Imports {
			for _, forbidden := range []string{
				modulePath + "/internal/engine/turn",
				modulePath + "/internal/core/session",
				modulePath + "/internal/adapter/out/sessionfs",
				modulePath + "/internal/feature/project",
			} {
				if packageWithin(imported, forbidden) {
					t.Errorf("Gio component %s imports forbidden package %s", importPath, imported)
				}
			}
			if strings.HasPrefix(imported, modulePath+"/internal/adapter/out/") {
				t.Errorf("Gio component %s imports driven adapter %s; application actions must come from the shell", importPath, imported)
			}
		}
	}
}

// component/uikit is the shared design-token and layout-primitive leaf. Every
// component package and the shell alias its Theme, Chrome, BoundedCache, and
// icon vocabulary from it, so the token set has exactly one home. If uikit
// starts importing a component, the shell, the controller, or any internal
// package, that boundary has been lost and this fails.
func TestDesktopGioUikitStaysALeaf(t *testing.T) {
	packages := desktopGioPackages(t)
	const uikitPath = modulePath + "/internal/adapter/in/desktop/gioui/component/uikit"
	pkg, ok := packages[uikitPath]
	if !ok {
		t.Fatalf("package %s not found in the Gio desktop package graph", uikitPath)
	}
	for _, imported := range pkg.Imports {
		if strings.HasPrefix(imported, modulePath) {
			t.Errorf("Gio uikit leaf %s imports project package %s; it must depend on Gio and stdlib only", uikitPath, imported)
		}
	}
}
