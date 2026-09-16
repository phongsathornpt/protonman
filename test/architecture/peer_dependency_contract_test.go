package architecture_test

import (
	"strings"
	"testing"
)

// peerAllowedImports records the internal imports permitted to the
// engine/feature/platform families. Engine and feature are mutually aware by
// design: the turn engine consumes feature capabilities (skill catalog for
// prompt composition, task patch classification for permission evaluation,
// image preparation for multimodal input), while feature/agent drives engine
// execution for delegated work. Neither family is strictly inner to the other,
// so rather than asserting a layering the code does not implement, this guard
// freezes the permitted surface. A new cross-family edge, or a new outbound
// or app edge from these families, is an architecture decision and must extend
// this map deliberately with its rationale.
func peerAllowedImports() map[string][]string {
	allow := func(targets ...string) []string {
		resolved := make([]string, 0, len(targets))
		for _, target := range targets {
			resolved = append(resolved, modulePath+"/internal/"+target)
		}
		return resolved
	}
	return map[string][]string{
		modulePath + "/internal/engine/turn": {
			modulePath + "/internal/engine/prompt",
			modulePath + "/internal/engine/toolcall",
			modulePath + "/internal/feature/skill",
			modulePath + "/internal/feature/imageprep",
			modulePath + "/internal/adapter/out/model",
		},
		modulePath + "/internal/engine/toolcall": allow(
			"feature/todo",
		),
		modulePath + "/internal/feature/agent": {
			modulePath + "/internal/engine/prompt",
			modulePath + "/internal/engine/toolcall",
			modulePath + "/internal/engine/turn",
			modulePath + "/internal/feature/skill",
			modulePath + "/internal/adapter/out/model",
		},
		modulePath + "/internal/feature/project": allow("platform/appdirs"),
		modulePath + "/internal/feature/skill":   allow("platform/appdirs"),
	}
}

// peerScopeTarget reports whether an import target participates in the
// peer-architecture contract: the engine/feature/platform families plus the
// two outward edges those families use (model publication, on-disk locations).
// Core, base, and SDK imports remain covered by the layer contracts in
// dependency_test.go instead.
func peerScopeTarget(target string) bool {
	if !strings.HasPrefix(target, modulePath+"/internal/") {
		return false
	}
	family := strings.TrimPrefix(target, modulePath+"/internal/")
	if index := strings.Index(family, "/"); index >= 0 {
		family = family[:index]
	}
	switch family {
	case "engine", "feature", "platform":
		return true
	}
	return target == modulePath+"/internal/adapter/out/model" ||
		target == modulePath+"/internal/platform/appdirs"
}

func TestPeerArchitectureEdgesStayDeclared(t *testing.T) {
	packages := listPackages(t)
	allowed := peerAllowedImports()
	peerSources := map[string]bool{
		modulePath + "/internal/engine/turn":     true,
		modulePath + "/internal/engine/toolcall": true,
		modulePath + "/internal/feature/agent":   true,
		modulePath + "/internal/feature/project": true,
		modulePath + "/internal/feature/skill":   true,
	}
	observed := map[string]bool{}
	for sourcePath := range peerSources {
		pkg, ok := packages[sourcePath]
		if !ok {
			t.Fatalf("package %s not found in the package graph", sourcePath)
		}
		for _, target := range pkg.Imports {
			if !peerScopeTarget(target) {
				continue
			}
			observed[sourcePath+" -> "+target] = true
			permitted := false
			for _, allow := range allowed[sourcePath] {
				if allow == target {
					permitted = true
					break
				}
			}
			if !permitted {
				t.Errorf("undeclared peer edge %s imports %s; record it in peerAllowedImports with a rationale or route it through an application port", sourcePath, target)
			}
		}
	}
	for sourcePath, targets := range allowed {
		for _, target := range targets {
			if !observed[sourcePath+" -> "+target] {
				t.Errorf("declared peer edge %s imports %s no longer exists; remove it (for example after inverting the dependency behind a core port)", sourcePath, target)
			}
		}
	}
}

func TestPeerFamiliesDoNotGainUnlistedSources(t *testing.T) {
	packages := listPackages(t)
	scope := func(path string) bool {
		return strings.HasPrefix(path, modulePath+"/internal/engine/") ||
			strings.HasPrefix(path, modulePath+"/internal/feature/") ||
			strings.HasPrefix(path, modulePath+"/internal/platform/")
	}
	known := map[string]bool{
		modulePath + "/internal/engine/turn":        true,
		modulePath + "/internal/engine/toolcall":    true,
		modulePath + "/internal/engine/prompt":      true,
		modulePath + "/internal/feature/agent":      true,
		modulePath + "/internal/feature/project":    true,
		modulePath + "/internal/feature/skill":      true,
		modulePath + "/internal/feature/memory":     true,
		modulePath + "/internal/feature/todo":       true,
		modulePath + "/internal/feature/imageprep":  true,
		modulePath + "/internal/feature/desktop":    true,
		modulePath + "/internal/platform/sandbox":   true,
		modulePath + "/internal/platform/telemetry": true,
	}
	for sourcePath, pkg := range packages {
		if !scope(sourcePath) || known[sourcePath] {
			continue
		}
		for _, target := range pkg.Imports {
			if peerScopeTarget(target) {
				t.Errorf("package %s is a new peer-family source with scope import %s; extend peerAllowedImports and the known-source registry deliberately", sourcePath, target)
			}
		}
	}
}

// TestPlatformTelemetryDependsOnCoreContractOnly ensures the inverted
// observer boundary stays inverted: platform/telemetry may depend on the
// core-owned event contract, but never on the engine/toolcall implementation.
func TestPlatformTelemetryDependsOnCoreContractOnly(t *testing.T) {
	packages := listPackages(t)
	source := modulePath + "/internal/platform/telemetry"
	pkg, ok := packages[source]
	if !ok {
		t.Fatalf("package %s not found in the package graph", source)
	}
	for _, imported := range pkg.Imports {
		if packageWithin(imported, modulePath+"/internal/engine") {
			t.Errorf("package %s imports engine package %s; telemetry must depend on the core-owned observer port", source, imported)
		}
	}
	foundContract := false
	for _, imported := range pkg.Imports {
		if imported == modulePath+"/internal/core/telemetry" {
			foundContract = true
			break
		}
	}
	if !foundContract {
		t.Errorf("package %s must import the core-owned telemetry contract %s", source, modulePath+"/internal/core/telemetry")
	}
}
