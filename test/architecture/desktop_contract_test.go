package architecture_test

import (
	"strings"
	"testing"
)

// desktopPackages are the packages that make up the Wails desktop frontend.
// The Wails implementation is tag-gated, so these guards load a desktop-tagged
// graph instead of relying only on the default package graph.
var desktopPackages = []string{
	modulePath + "/cmd/protonman-desktop",
	modulePath + "/internal/adapter/in/desktop",
	modulePath + "/internal/feature/desktop",
}

// TestDesktopPackagesStayVisibleToGuards fails if the desktop build tag stops
// exposing a desktop package, which would silently drop the subsystem out of
// package-graph coverage.
func TestDesktopPackagesStayVisibleToGuards(t *testing.T) {
	packages := listDesktopPackages(t)
	for _, pkg := range desktopPackages {
		if _, ok := packages[pkg]; !ok {
			t.Fatalf("desktop package %s missing from the desktop-tagged package graph", pkg)
		}
	}
}

// The desktop frontend is a fourth inbound adapter and must keep the same
// boundary discipline as the TUI, ACP, and headless adapters: no direct turn
// engine, session persistence, or project manipulation.
func TestDesktopInboundAdapterUsesApplicationBoundary(t *testing.T) {
	packages := listDesktopPackages(t)
	assertPackageNoImportPrefixes(
		t,
		packages,
		modulePath+"/internal/adapter/in/desktop",
		[]string{
			modulePath + "/internal/engine/turn",
			modulePath + "/internal/core/session",
			modulePath + "/internal/adapter/out/sessionfs",
			modulePath + "/internal/feature/project",
		},
	)
}

// The desktop frontend drives the CLI runtime over ACP instead of embedding a
// second agent loop, so the outbound ACP client is its only permitted driven
// adapter. A new outbound adapter dependency is an architecture decision that
// must be recorded deliberately rather than added silently.
func TestDesktopInboundAdapterOnlyDependsOnACPClient(t *testing.T) {
	packages := listDesktopPackages(t)
	pkg, ok := packages[modulePath+"/internal/adapter/in/desktop"]
	if !ok {
		t.Fatalf("package %s not found in the desktop package graph", modulePath+"/internal/adapter/in/desktop")
	}
	allowed := map[string]bool{
		modulePath + "/internal/adapter/out/acpclient": true,
	}
	for _, imported := range pkg.Imports {
		if !strings.HasPrefix(imported, modulePath+"/internal/adapter/out/") {
			continue
		}
		if !allowed[imported] {
			t.Errorf("desktop inbound adapter imports unexpected driven adapter %s", imported)
		}
	}
}
