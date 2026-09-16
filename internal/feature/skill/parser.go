package skill

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var (
	// ErrNoFrontmatter indicates that SKILL.md does not start with YAML frontmatter.
	ErrNoFrontmatter = errors.New("missing YAML frontmatter delimiters")
	// ErrMissingSKILLFile indicates the SKILL.md file is absent.
	ErrMissingSKILLFile = errors.New("SKILL.md file not found")
)

// ParseSkillFile reads a SKILL.md file and constructs a domain Skill.
func ParseSkillFile(filePath string, scope Scope) (Skill, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return Skill{}, fmt.Errorf("read skill file %q: %w", filePath, err)
	}

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		absPath = filePath
	}
	baseDir := filepath.Dir(absPath)

	fm, body, err := extractFrontmatterAndBody(data)
	if err != nil {
		return Skill{}, fmt.Errorf("parse %q: %w", filePath, err)
	}

	raw, err := parseYAMLFrontmatter(fm)
	if err != nil {
		return Skill{}, fmt.Errorf("decode YAML frontmatter in %q: %w", filePath, err)
	}

	name := stringVal(raw["name"])
	if name == "" {
		// Fallback to directory name if name was omitted
		name = filepath.Base(baseDir)
	}
	if err := ValidateName(name); err != nil {
		return Skill{}, fmt.Errorf("invalid skill name in %q: %w", filePath, err)
	}

	desc := stringVal(raw["description"])
	if err := ValidateDescription(desc); err != nil {
		return Skill{}, fmt.Errorf("invalid skill description in %q: %w", filePath, err)
	}

	allowedTools := parseAllowedTools(raw["allowed-tools"])
	resources := scanResources(baseDir)

	s := Skill{
		Name:          name,
		Description:   desc,
		Location:      absPath,
		BaseDir:       baseDir,
		Scope:         scope,
		License:       stringVal(raw["license"]),
		Compatibility: stringVal(raw["compatibility"]),
		Metadata:      parseMetadata(raw["metadata"]),
		AllowedTools:  allowedTools,
		Instructions:  strings.TrimSpace(body),
		Resources:     resources,
	}

	if err := s.Validate(); err != nil {
		return Skill{}, fmt.Errorf("validate skill in %q: %w", filePath, err)
	}

	return s, nil
}

func stringVal(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return fmt.Sprint(v)
}

func extractFrontmatterAndBody(content []byte) ([]byte, string, error) {
	scanner := bufio.NewScanner(bytes.NewReader(content))
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, "", err
	}

	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return nil, "", fmt.Errorf("%w: file must begin with '---'", ErrNoFrontmatter)
	}

	closingIndex := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			closingIndex = i
			break
		}
	}

	if closingIndex == -1 {
		return nil, "", fmt.Errorf("%w: closing '---' not found", ErrNoFrontmatter)
	}

	fmLines := lines[1:closingIndex]
	bodyLines := lines[closingIndex+1:]

	fm := []byte(strings.Join(fmLines, "\n"))
	body := strings.Join(bodyLines, "\n")
	return fm, body, nil
}

func parseAllowedTools(raw any) []string {
	if raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case string:
		fields := strings.Fields(v)
		if len(fields) == 0 {
			return nil
		}
		return fields
	case []any:
		var result []string
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				result = append(result, strings.TrimSpace(s))
			}
		}
		return result
	case []string:
		return v
	default:
		return nil
	}
}

func scanResources(baseDir string) []string {
	var resources []string
	const maxScanEntries = 500
	count := 0

	_ = filepath.WalkDir(baseDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if count >= maxScanEntries {
			return fs.SkipAll
		}
		if d.IsDir() {
			name := d.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" {
				return fs.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(baseDir, path)
		if err != nil {
			return nil
		}
		if rel == "SKILL.md" || strings.HasPrefix(rel, ".") {
			return nil
		}
		resources = append(resources, filepath.ToSlash(rel))
		count++
		return nil
	})

	sort.Strings(resources)
	return resources
}

func parseMetadata(raw any) map[string]any {
	if raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case map[string]any:
		return v
	case map[string]string:
		res := make(map[string]any, len(v))
		for k, val := range v {
			res[k] = val
		}
		return res
	case map[any]any:
		res := make(map[string]any, len(v))
		for k, val := range v {
			res[fmt.Sprint(k)] = val
		}
		return res
	default:
		return nil
	}
}
