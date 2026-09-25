package architecture_test

import "testing"

var desktopGioPackages = []string{
	modulePath + "/cmd/protonman-desktop-gio",
	modulePath + "/internal/adapter/in/desktop/gioui",
}

// TestDesktopGioPackagesStayVisibleToGuards fails if the desktop build tag
// stops exposing a desktop package, which would silently drop the subsystem out
// of package-graph coverage.
func TestDesktopGioPackagesStayVisibleToGuards(t *testing.T) {
	packages := listDesktopGioPackages(t)
	for _, pkg := range desktopGioPackages {
		if _, ok := packages[pkg]; !ok {
			t.Fatalf("Gio desktop package %s missing from the desktop package graph", pkg)
		}
	}
}

// The Gio desktop frontend is a fourth inbound adapter and must keep the same
// boundary discipline as the TUI, ACP, and headless adapters: no direct turn
// engine, session persistence, or project manipulation.
func TestDesktopGioInboundAdapterUsesApplicationBoundary(t *testing.T) {
	packages := listDesktopGioPackages(t)
	assertPackageNoImportPrefixes(
		t,
		packages,
		modulePath+"/internal/adapter/in/desktop/gioui",
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
	packages := listDesktopGioPackages(t)
	pkg, ok := packages[modulePath+"/internal/adapter/in/desktop/gioui"]
	if !ok {
		t.Fatalf("package %s not found in the Gio desktop package graph", modulePath+"/internal/adapter/in/desktop/gioui")
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
