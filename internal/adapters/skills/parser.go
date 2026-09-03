// Package skills implements discovery, parsing, and loading of Agent Skills.
package skills

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

	"gopkg.in/yaml.v3"

	"github.com/projectTHORN/proton/internal/domain/skill"
)

var (
	// ErrNoFrontmatter indicates that SKILL.md does not start with YAML frontmatter.
	ErrNoFrontmatter = errors.New("missing YAML frontmatter delimiters")
	// ErrMissingSKILLFile indicates the SKILL.md file is absent.
	ErrMissingSKILLFile = errors.New("SKILL.md file not found")
)

type rawFrontmatter struct {
	Name          string            `yaml:"name"`
	Description   string            `yaml:"description"`
	License       string            `yaml:"license"`
	Compatibility string            `yaml:"compatibility"`
	Metadata      map[string]string `yaml:"metadata"`
	AllowedTools  any               `yaml:"allowed-tools"`
}

// ParseSkillFile reads a SKILL.md file and constructs a domain Skill.
func ParseSkillFile(filePath string, scope skill.Scope) (skill.Skill, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return skill.Skill{}, fmt.Errorf("read skill file %q: %w", filePath, err)
	}

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		absPath = filePath
	}
	baseDir := filepath.Dir(absPath)

	fm, body, err := extractFrontmatterAndBody(data)
	if err != nil {
		return skill.Skill{}, fmt.Errorf("parse %q: %w", filePath, err)
	}

	var raw rawFrontmatter
	if err := yaml.Unmarshal(fm, &raw); err != nil {
		// Attempt lenient fix for unquoted colons
		fixedFm := fixLenientYAML(fm)
		if retryErr := yaml.Unmarshal(fixedFm, &raw); retryErr != nil {
			return skill.Skill{}, fmt.Errorf("decode YAML frontmatter in %q: %w", filePath, err)
		}
	}

	name := strings.TrimSpace(raw.Name)
	if name == "" {
		// Fallback to directory name if name was omitted
		name = filepath.Base(baseDir)
	}
	if err := skill.ValidateName(name); err != nil {
		return skill.Skill{}, fmt.Errorf("invalid skill name in %q: %w", filePath, err)
	}

	desc := strings.TrimSpace(raw.Description)
	if err := skill.ValidateDescription(desc); err != nil {
		return skill.Skill{}, fmt.Errorf("invalid skill description in %q: %w", filePath, err)
	}

	allowedTools := parseAllowedTools(raw.AllowedTools)
	resources := scanResources(baseDir)

	s := skill.Skill{
		Name:          name,
		Description:   desc,
		Location:      absPath,
		BaseDir:       baseDir,
		Scope:         scope,
		License:       strings.TrimSpace(raw.License),
		Compatibility: strings.TrimSpace(raw.Compatibility),
		Metadata:      raw.Metadata,
		AllowedTools:  allowedTools,
		Instructions:  strings.TrimSpace(body),
		Resources:     resources,
	}

	if err := s.Validate(); err != nil {
		return skill.Skill{}, fmt.Errorf("validate skill in %q: %w", filePath, err)
	}

	return s, nil
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

func fixLenientYAML(content []byte) []byte {
	var out []string
	scanner := bufio.NewScanner(bytes.NewReader(content))
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		// If line starts with "description:" and has additional unquoted colons, quote the rest
		if strings.HasPrefix(trimmed, "description:") {
			parts := strings.SplitN(trimmed, ":", 2)
			val := strings.TrimSpace(parts[1])
			if !strings.HasPrefix(val, "\"") && !strings.HasPrefix(val, "'") {
				indent := line[:strings.Index(line, "description:")]
				escapedVal := strings.ReplaceAll(val, "\"", "\\\"")
				line = fmt.Sprintf("%sdescription: \"%s\"", indent, escapedVal)
			}
		}
		out = append(out, line)
	}
	return []byte(strings.Join(out, "\n"))
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
