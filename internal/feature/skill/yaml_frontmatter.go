package skill

import (
	"bufio"
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

type yamlLine struct {
	indent  int
	content string
	lineNum int
}

// parseYAMLFrontmatter parses simple YAML frontmatter into a map[string]any.
// It supports nested maps, sequences, quoted and unquoted strings, numbers,
// booleans, inline arrays, and lenient unquoted colons in values.
func parseYAMLFrontmatter(content []byte) (map[string]any, error) {
	lines, err := preprocessYAMLLines(content)
	if err != nil {
		return nil, err
	}
	if len(lines) == 0 {
		return make(map[string]any), nil
	}

	idx := 0
	val, err := parseYAMLBlock(lines, &idx, lines[0].indent)
	if err != nil {
		return nil, err
	}

	m, ok := val.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("top-level frontmatter must be a key-value mapping")
	}
	return m, nil
}

func preprocessYAMLLines(content []byte) ([]yamlLine, error) {
	scanner := bufio.NewScanner(bytes.NewReader(content))
	var lines []yamlLine
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		raw := scanner.Text()
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Count leading spaces for indentation
		indent := 0
		for _, r := range raw {
			if r == ' ' {
				indent++
			} else if r == '\t' {
				indent += 2
			} else {
				break
			}
		}

		// Strip trailing comments that are not inside quotes
		clean := stripYAMLComment(trimmed)
		clean = strings.TrimSpace(clean)
		if clean == "" {
			continue
		}

		lines = append(lines, yamlLine{
			indent:  indent,
			content: clean,
			lineNum: lineNum,
		})
	}

	return lines, scanner.Err()
}

func stripYAMLComment(line string) string {
	inDouble := false
	inSingle := false
	escaped := false

	for i, r := range line {
		if inDouble {
			if escaped {
				escaped = false
			} else if r == '\\' {
				escaped = true
			} else if r == '"' {
				inDouble = false
			}
		} else if inSingle {
			if r == '\'' {
				inSingle = false
			}
		} else {
			if r == '"' {
				inDouble = true
			} else if r == '\'' {
				inSingle = true
			} else if r == '#' {
				return line[:i]
			}
		}
	}
	return line
}

func parseYAMLBlock(lines []yamlLine, idx *int, baseIndent int) (any, error) {
	if *idx >= len(lines) {
		return make(map[string]any), nil
	}

	first := lines[*idx]
	if strings.HasPrefix(first.content, "- ") || first.content == "-" {
		return parseYAMLSequence(lines, idx, baseIndent)
	}
	return parseYAMLMapping(lines, idx, baseIndent)
}

func parseYAMLMapping(lines []yamlLine, idx *int, baseIndent int) (map[string]any, error) {
	result := make(map[string]any)

	for *idx < len(lines) {
		line := lines[*idx]
		if line.indent < baseIndent {
			break
		}
		// If at a deeper indent without a parent key, that's handled by recursive calls.
		// If line is at baseIndent:
		colonIdx := findYAMLColon(line.content)
		if colonIdx == -1 {
			// Might be a sequence item in mapping context or error
			return nil, fmt.Errorf("line %d: expected key-value mapping, got %q", line.lineNum, line.content)
		}

		key := strings.TrimSpace(line.content[:colonIdx])
		key = strings.Trim(key, "\"'")
		rawVal := strings.TrimSpace(line.content[colonIdx+1:])
		*idx++

		if rawVal == "" {
			// Nested block (map, list, or multiline string)
			if *idx < len(lines) && lines[*idx].indent > line.indent {
				childIndent := lines[*idx].indent
				childVal, err := parseYAMLBlock(lines, idx, childIndent)
				if err != nil {
					return nil, err
				}
				result[key] = childVal
			} else {
				result[key] = ""
			}
			continue
		}

		// Handle block scalar indicator (| or >)
		if rawVal == "|" || rawVal == ">" {
			var sb strings.Builder
			for *idx < len(lines) && lines[*idx].indent > line.indent {
				if sb.Len() > 0 {
					sb.WriteString("\n")
				}
				sb.WriteString(lines[*idx].content)
				*idx++
			}
			result[key] = sb.String()
			continue
		}

		val, err := parseYAMLScalar(rawVal)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line.lineNum, err)
		}
		result[key] = val
	}

	return result, nil
}

func parseYAMLSequence(lines []yamlLine, idx *int, baseIndent int) ([]any, error) {
	var result []any

	for *idx < len(lines) {
		line := lines[*idx]
		if line.indent < baseIndent {
			break
		}

		if !strings.HasPrefix(line.content, "- ") && line.content != "-" {
			break
		}

		itemContent := strings.TrimSpace(strings.TrimPrefix(line.content, "-"))
		*idx++

		if itemContent == "" {
			// Sub-block item
			if *idx < len(lines) && lines[*idx].indent > line.indent {
				childVal, err := parseYAMLBlock(lines, idx, lines[*idx].indent)
				if err != nil {
					return nil, err
				}
				result = append(result, childVal)
			} else {
				result = append(result, nil)
			}
			continue
		}

		// If itemContent is a nested key: value pair
		if colonIdx := findYAMLColon(itemContent); colonIdx != -1 {
			// Map item in list
			k := strings.TrimSpace(itemContent[:colonIdx])
			k = strings.Trim(k, "\"'")
			vStr := strings.TrimSpace(itemContent[colonIdx+1:])
			subMap := make(map[string]any)
			if vStr == "" && *idx < len(lines) && lines[*idx].indent > line.indent {
				childVal, err := parseYAMLBlock(lines, idx, lines[*idx].indent)
				if err != nil {
					return nil, err
				}
				subMap[k] = childVal
			} else {
				v, err := parseYAMLScalar(vStr)
				if err != nil {
					return nil, err
				}
				subMap[k] = v
			}
			result = append(result, subMap)
			continue
		}

		val, err := parseYAMLScalar(itemContent)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line.lineNum, err)
		}
		result = append(result, val)
	}

	return result, nil
}

func findYAMLColon(s string) int {
	inDouble := false
	inSingle := false
	escaped := false

	for i, r := range s {
		if inDouble {
			if escaped {
				escaped = false
			} else if r == '\\' {
				escaped = true
			} else if r == '"' {
				inDouble = false
			}
		} else if inSingle {
			if r == '\'' {
				inSingle = false
			}
		} else {
			if r == '"' {
				inDouble = true
			} else if r == '\'' {
				inSingle = true
			} else if r == ':' {
				if i+1 == len(s) || unicode.IsSpace(rune(s[i+1])) {
					return i
				}
			}
		}
	}
	return -1
}

func parseYAMLScalar(val string) (any, error) {
	val = strings.TrimSpace(val)
	if val == "" {
		return "", nil
	}

	// Boolean
	if val == "true" {
		return true, nil
	}
	if val == "false" {
		return false, nil
	}

	// Quoted string (double quotes)
	if strings.HasPrefix(val, "\"") && strings.HasSuffix(val, "\"") && len(val) >= 2 {
		unquoted, err := strconv.Unquote(val)
		if err == nil {
			return unquoted, nil
		}
		return val[1 : len(val)-1], nil
	}

	// Quoted string (single quotes)
	if strings.HasPrefix(val, "'") && strings.HasSuffix(val, "'") && len(val) >= 2 {
		return val[1 : len(val)-1], nil
	}

	// Empty array
	if val == "[]" {
		return []any{}, nil
	}

	// Empty map
	if val == "{}" {
		return make(map[string]any), nil
	}

	// Inline array: [a, b, c]
	if strings.HasPrefix(val, "[") && strings.HasSuffix(val, "]") {
		inner := strings.TrimSpace(val[1 : len(val)-1])
		if inner == "" {
			return []any{}, nil
		}
		rawParts := strings.Split(inner, ",")
		var arr []any
		for _, p := range rawParts {
			item, err := parseYAMLScalar(strings.TrimSpace(p))
			if err != nil {
				return nil, err
			}
			arr = append(arr, item)
		}
		return arr, nil
	}

	// Integer
	if num, err := strconv.ParseInt(val, 10, 64); err == nil {
		return int(num), nil
	}

	// Fallback to raw string
	return val, nil
}
