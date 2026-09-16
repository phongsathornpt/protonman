package architecture_test

import (
	"strings"
	"testing"
)

// appAllowedInternalImports is the ratchet for the application layer.
// internal/app currently assembles engine primitives and feature concrete
// types during the clean-architecture inversion; each edge below is
// grandfathered and must be removed only by introducing an application-owned
// port (as was done for telemetry, appdirs, and agentprofile). New internal
// imports are forbidden: an adapter, a new engine/feature/platform package,
// or the composition root appearing here means the boundary moved the wrong
// direction. Shrink this set as ports are extracted; never grow it.
func appAllowedInternalImports() map[string]bool {
	allow := func(targets ...string) map[string]bool {
		out := make(map[string]bool, len(targets))
		for _, target := range targets {
			out[modulePath+"/internal/"+target] = true
		}
		return out
	}
	return allow(
		// core contracts are always allowed: they are the inward direction.
		"core/agentprofile",
		"core/memory",
		"core/modelcatalog",
		"core/modelclient",
		"core/modelconfig",
		"core/permission",
		"core/project",
		"core/session",
		"core/todo",
		"core/workspace",
		// engine primitives currently assembled by BuildConversation and the
		// Agents wrapper; to be replaced by application-owned ports.
		"engine/prompt",
		"engine/toolcall",
		"engine/turn",
		// feature concrete types consumed until ports exist.
		"feature/agent",
		"feature/skill",
		// platform infrastructure resolved through appdirs.
		"platform/appdirs",
	)
}

func TestAppInternalImportsStayRatcheted(t *testing.T) {
	packages := listPackages(t)
	source := modulePath + "/internal/app"
	pkg, ok := packages[source]
	if !ok {
		t.Fatalf("package %s not found in the package graph", source)
	}
	allowed := appAllowedInternalImports()
	for _, imported := range pkg.Imports {
		if !strings.HasPrefix(imported, modulePath+"/internal/") {
			continue
		}
		if strings.Contains(imported, "/internal/adapter/") || strings.Contains(imported, "/internal/cmd/") {
			t.Errorf("package %s imports adapter/composition package %s; application must not depend on outbound adapters or the composition root", source, imported)
			continue
		}
		if !allowed[imported] {
			t.Errorf("package %s gained new internal import %s; application dependencies must be ratcheted inward via a core port or an application-owned interface, not by importing new engine/feature/platform packages", source, imported)
		}
	}
}

// TestAppImportsDoNotReachAdaptersEvenWithDesktopTag covers the same ratchet
// with the desktop build tag enabled, so tag-gated code paths cannot introduce
// an application-to-adapter edge invisible to the default package graph.
func TestAppImportsDoNotReachAdaptersEvenWithDesktopTag(t *testing.T) {
	packages := listDesktopPackages(t)
	source := modulePath + "/internal/app"
	pkg, ok := packages[source]
	if !ok {
		t.Fatalf("package %s not found in the desktop package graph", source)
	}
	for _, imported := range pkg.Imports {
		if strings.Contains(imported, "/internal/adapter/") {
			t.Errorf("package %s imports adapter package %s in the desktop build", source, imported)
		}
	}
}
