package composer

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// imageExtensions are the pasted-path suffixes treated as stageable images.
var imageExtensions = map[string]struct{}{
	".png":  {},
	".jpg":  {},
	".jpeg": {},
	".gif":  {},
	".webp": {},
}

// LocalImagePathFromPaste classifies pasted content as an image path. It
// rejects multi-line pastes and paths that do not resolve to a regular file
// with an image extension, returning ok=false so the caller inserts the paste
// as text instead.
func LocalImagePathFromPaste(content, workDir string) (string, bool) {
	if strings.TrimSpace(content) == "" || strings.ContainsAny(content, "\r\n") {
		return "", false
	}
	cleaned := NormalizePastedPath(content, workDir)
	candidate := strings.TrimSpace(cleaned)
	if candidate == "" {
		return "", false
	}
	path := candidate
	if !filepath.IsAbs(path) && strings.TrimSpace(workDir) != "" {
		path = filepath.Join(workDir, path)
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	if _, ok := imageExtensions[strings.ToLower(filepath.Ext(path))]; !ok {
		return "", false
	}
	return filepath.Clean(path), true
}

// NormalizePastedPath rewrites a single-line pasted path into a workspace
// relative form when the path exists inside the workspace. It handles shell
// quoting, file:// URLs, and backslash-escaped spaces. Content that does not
// resolve to an existing path is returned unchanged.
func NormalizePastedPath(content, workDir string) string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return content
	}
	if strings.Contains(trimmed, "\n") || strings.Contains(trimmed, "\r") {
		return content
	}

	if cleaned := cleanCandidate(trimmed, workDir); cleaned != "" {
		return cleaned
	}
	return content
}

func cleanCandidate(cand, workDir string) string {
	cand = strings.TrimSpace(cand)
	if cand == "" {
		return ""
	}
	if (strings.HasPrefix(cand, "'") && strings.HasSuffix(cand, "'") && len(cand) >= 2) ||
		(strings.HasPrefix(cand, "\"") && strings.HasSuffix(cand, "\"") && len(cand) >= 2) {
		cand = cand[1 : len(cand)-1]
	}
	if strings.HasPrefix(cand, "file://") {
		cand = strings.TrimPrefix(cand, "file://")
		if unescaped, err := url.PathUnescape(cand); err == nil {
			cand = unescaped
		}
	}
	if strings.Contains(cand, `\ `) {
		cand = strings.ReplaceAll(cand, `\ `, " ")
	}
	cand = filepath.Clean(cand)
	if _, err := os.Stat(cand); err != nil {
		return ""
	}
	if workDir == "" {
		return cand
	}
	if rel, err := filepath.Rel(workDir, cand); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	// A symlinked workspace root makes relative comparison fail even though the
	// file is genuinely inside it, so compare evaluated paths as a fallback.
	evalWorkDir, err := filepath.EvalSymlinks(workDir)
	if err != nil {
		return cand
	}
	evalCand, err := filepath.EvalSymlinks(cand)
	if err != nil {
		return cand
	}
	if rel, err := filepath.Rel(evalWorkDir, evalCand); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return cand
}
