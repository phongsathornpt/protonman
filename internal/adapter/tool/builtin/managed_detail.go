package builtin

import (
	"fmt"
	"strings"

	"github.com/projectTHORN/proton/internal/workspace"
)

func managedFileDetail(path string, proposed []byte) string {
	path = strings.TrimSpace(path)
	kind := workspace.ClassifyManagedPath(path)
	if workspace.HasGeneratedHeader(proposed) {
		kind = workspace.ManagedFileGenerated
	}
	if kind == workspace.ManagedFileRegular {
		return path
	}
	return fmt.Sprintf("%s · %s", path, managedFileLabel(kind))
}

func managedFileLabel(kind workspace.ManagedFileKind) string {
	switch kind {
	case workspace.ManagedFileGenerated:
		return "generated file"
	case workspace.ManagedFileVendored:
		return "vendored file"
	case workspace.ManagedFileDependency:
		return "dependency-managed file"
	case workspace.ManagedFileLockfile:
		return "lockfile"
	default:
		return "managed file"
	}
}
