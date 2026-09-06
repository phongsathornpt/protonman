package workspace

import (
	"path/filepath"
	"strings"
)

// ManagedFileKind classifies files commonly owned by generators or dependency tooling.
type ManagedFileKind string

const (
	ManagedFileRegular    ManagedFileKind = ""
	ManagedFileGenerated  ManagedFileKind = "generated"
	ManagedFileVendored   ManagedFileKind = "vendored"
	ManagedFileDependency ManagedFileKind = "dependency-managed"
	ManagedFileLockfile   ManagedFileKind = "lockfile"
)

// ClassifyManagedPath identifies conservative generated/dependency conventions.
func ClassifyManagedPath(path string) ManagedFileKind {
	clean := filepath.ToSlash(filepath.Clean(strings.TrimSpace(path)))
	lower := strings.ToLower(clean)
	parts := strings.Split(lower, "/")
	for i, part := range parts {
		if i == len(parts)-1 {
			break
		}
		switch part {
		case "vendor":
			return ManagedFileVendored
		case "node_modules":
			return ManagedFileDependency
		}
	}
	base := filepath.Base(lower)
	switch base {
	case "package-lock.json", "npm-shrinkwrap.json", "pnpm-lock.yaml", "yarn.lock",
		"cargo.lock", "composer.lock", "poetry.lock", "uv.lock", "go.sum":
		return ManagedFileLockfile
	}
	if strings.HasSuffix(base, ".pb.go") ||
		strings.HasSuffix(base, ".gen.go") ||
		strings.HasSuffix(base, "_generated.go") ||
		strings.Contains(base, ".generated.") {
		return ManagedFileGenerated
	}
	return ManagedFileRegular
}

// HasGeneratedHeader recognizes explicit generated-file ownership markers.
func HasGeneratedHeader(content []byte) bool {
	if len(content) > 4096 {
		content = content[:4096]
	}
	text := strings.ToLower(string(content))
	return strings.Contains(text, "generated") && strings.Contains(text, "do not edit")
}
