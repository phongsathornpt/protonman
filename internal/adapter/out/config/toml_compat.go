package config

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// decodeTOML decodes TOML bytes into target by converting the TOML structure
// into an intermediate map and unmarshaling it via JSON. This lightweight parser
// handles Protonman's configuration grammar without pulling in external dependencies.
func decodeTOML(data []byte, target any) error {
	m, err := parseTOMLToMap(data)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("serialize parsed TOML: %w", err)
	}
	return json.Unmarshal(encoded, target)
}

func parseTOMLToMap(data []byte) (map[string]any, error) {
	root := make(map[string]any)
	var currentPath []string
	isTableArray := false

	scanner := bufio.NewScanner(bytes.NewReader(data))
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Strip trailing comments that are not inside quotes
		line = stripComment(line)
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Check for array of tables: [[section.name]]
		if strings.HasPrefix(line, "[[") && strings.HasSuffix(line, "]]") {
			pathStr := strings.TrimSpace(line[2 : len(line)-2])
			if pathStr == "" {
				return nil, fmt.Errorf("line %d: empty array table header", lineNum)
			}
			parts := splitPath(pathStr)
			currentPath = parts
			isTableArray = true

			// Ensure parent tables exist
			if err := ensureTableArrayEntry(root, parts); err != nil {
				return nil, fmt.Errorf("line %d: %w", lineNum, err)
			}
			continue
		}

		// Check for standard table: [section.name]
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			pathStr := strings.TrimSpace(line[1 : len(line)-1])
			if pathStr == "" {
				return nil, fmt.Errorf("line %d: empty table header", lineNum)
			}
			parts := splitPath(pathStr)
			currentPath = parts
			isTableArray = false

			if err := ensureTable(root, parts); err != nil {
				return nil, fmt.Errorf("line %d: %w", lineNum, err)
			}
			continue
		}

		// Key-value pair
		eqIdx := strings.IndexByte(line, '=')
		if eqIdx == -1 {
			return nil, fmt.Errorf("line %d: invalid syntax %q (missing '=')", lineNum, line)
		}
		key := strings.TrimSpace(line[:eqIdx])
		rawVal := strings.TrimSpace(line[eqIdx+1:])
		if key == "" {
			return nil, fmt.Errorf("line %d: empty key", lineNum)
		}
		key = strings.Trim(key, "\"'")

		val, err := parseTOMLValue(rawVal, scanner, &lineNum)
		if err != nil {
			return nil, fmt.Errorf("line %d (key %q): %w", lineNum, key, err)
		}

		targetMap, err := resolveCurrentMap(root, currentPath, isTableArray)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNum, err)
		}
		targetMap[key] = val
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read TOML: %w", err)
	}
	return root, nil
}

func stripComment(line string) string {
	inQuote := false
	var quoteChar rune
	escaped := false

	for i, r := range line {
		if inQuote {
			if escaped {
				escaped = false
			} else if r == '\\' {
				escaped = true
			} else if r == quoteChar {
				inQuote = false
			}
		} else {
			if r == '"' || r == '\'' {
				inQuote = true
				quoteChar = r
			} else if r == '#' {
				return line[:i]
			}
		}
	}
	return line
}

func splitPath(pathStr string) []string {
	var parts []string
	var current strings.Builder
	inQuote := false
	var quoteChar rune

	for _, r := range pathStr {
		if inQuote {
			if r == quoteChar {
				inQuote = false
			} else {
				current.WriteRune(r)
			}
		} else {
			if r == '"' || r == '\'' {
				inQuote = true
				quoteChar = r
			} else if r == '.' {
				if current.Len() > 0 {
					parts = append(parts, current.String())
					current.Reset()
				}
			} else {
				current.WriteRune(r)
			}
		}
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts
}

func ensureTable(root map[string]any, parts []string) error {
	curr := root
	for _, p := range parts {
		existing, ok := curr[p]
		if !ok {
			next := make(map[string]any)
			curr[p] = next
			curr = next
		} else if m, ok := existing.(map[string]any); ok {
			curr = m
		} else {
			return fmt.Errorf("conflict: path element %q is not a table", p)
		}
	}
	return nil
}

func ensureTableArrayEntry(root map[string]any, parts []string) error {
	if len(parts) == 0 {
		return fmt.Errorf("empty table array path")
	}
	curr := root
	for i := 0; i < len(parts)-1; i++ {
		p := parts[i]
		existing, ok := curr[p]
		if !ok {
			next := make(map[string]any)
			curr[p] = next
			curr = next
		} else if m, ok := existing.(map[string]any); ok {
			curr = m
		} else {
			return fmt.Errorf("conflict: parent path element %q is not a table", p)
		}
	}

	last := parts[len(parts)-1]
	existing, ok := curr[last]
	newEntry := make(map[string]any)
	if !ok {
		curr[last] = []any{newEntry}
	} else if arr, ok := existing.([]any); ok {
		curr[last] = append(arr, newEntry)
	} else {
		return fmt.Errorf("conflict: path element %q is not an array of tables", last)
	}
	return nil
}

func resolveCurrentMap(root map[string]any, path []string, isTableArray bool) (map[string]any, error) {
	if len(path) == 0 {
		return root, nil
	}
	curr := root
	for i := 0; i < len(path)-1; i++ {
		p := path[i]
		val, ok := curr[p]
		if !ok {
			return nil, fmt.Errorf("path element %q missing", p)
		}
		if arr, ok := val.([]any); ok && len(arr) > 0 {
			lastEntry, ok := arr[len(arr)-1].(map[string]any)
			if !ok {
				return nil, fmt.Errorf("path element %q array entry is not a map", p)
			}
			curr = lastEntry
		} else if m, ok := val.(map[string]any); ok {
			curr = m
		} else {
			return nil, fmt.Errorf("path element %q is not a map or array", p)
		}
	}

	last := path[len(path)-1]
	val, ok := curr[last]
	if !ok {
		return nil, fmt.Errorf("path element %q missing", last)
	}
	if isTableArray {
		arr, ok := val.([]any)
		if !ok || len(arr) == 0 {
			return nil, fmt.Errorf("path element %q is not a populated array", last)
		}
		entry, ok := arr[len(arr)-1].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("path element %q last entry is not a map", last)
		}
		return entry, nil
	}

	m, ok := val.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("path element %q is not a map", last)
	}
	return m, nil
}

func parseTOMLValue(val string, scanner *bufio.Scanner, lineNum *int) (any, error) {
	val = strings.TrimSpace(val)
	if val == "" {
		return nil, fmt.Errorf("empty value")
	}

	// Multi-line array handling: if array starts but doesn't end on same line
	if strings.HasPrefix(val, "[") && !strings.HasSuffix(val, "]") {
		for scanner.Scan() {
			*lineNum++
			nextLine := stripComment(scanner.Text())
			val += " " + strings.TrimSpace(nextLine)
			if strings.HasSuffix(strings.TrimSpace(val), "]") {
				break
			}
		}
	}

	// Boolean
	if val == "true" {
		return true, nil
	}
	if val == "false" {
		return false, nil
	}

	// String: double quotes or single quotes
	if (strings.HasPrefix(val, "\"") && strings.HasSuffix(val, "\"") && len(val) >= 2) ||
		(strings.HasPrefix(val, "'") && strings.HasSuffix(val, "'") && len(val) >= 2) {
		unquoted, err := strconv.Unquote(val)
		if err == nil {
			return unquoted, nil
		}
		// Fallback for single-quoted strings where Unquote might fail
		return val[1 : len(val)-1], nil
	}

	// Integer
	if num, err := strconv.ParseInt(val, 10, 64); err == nil {
		return int(num), nil
	}

	// Float
	if flt, err := strconv.ParseFloat(val, 64); err == nil {
		return flt, nil
	}

	// Array: [item, item]
	if strings.HasPrefix(val, "[") && strings.HasSuffix(val, "]") {
		inner := strings.TrimSpace(val[1 : len(val)-1])
		if inner == "" {
			return []any{}, nil
		}
		items, err := splitArrayItems(inner)
		if err != nil {
			return nil, err
		}
		var result []any
		for _, item := range items {
			parsed, err := parseTOMLValue(item, scanner, lineNum)
			if err != nil {
				return nil, err
			}
			result = append(result, parsed)
		}
		return result, nil
	}

	return nil, fmt.Errorf("unrecognized value syntax: %s", val)
}

func splitArrayItems(s string) ([]string, error) {
	var items []string
	var current strings.Builder
	inQuote := false
	var quoteChar rune
	depth := 0

	for _, r := range s {
		if inQuote {
			current.WriteRune(r)
			if r == quoteChar {
				inQuote = false
			}
		} else {
			if r == '"' || r == '\'' {
				inQuote = true
				quoteChar = r
				current.WriteRune(r)
			} else if r == '[' {
				depth++
				current.WriteRune(r)
			} else if r == ']' {
				depth--
				current.WriteRune(r)
			} else if r == ',' && depth == 0 {
				trimmed := strings.TrimSpace(current.String())
				if trimmed != "" {
					items = append(items, trimmed)
				}
				current.Reset()
			} else if !unicode.IsSpace(r) || current.Len() > 0 {
				current.WriteRune(r)
			}
		}
	}
	trimmed := strings.TrimSpace(current.String())
	if trimmed != "" {
		items = append(items, trimmed)
	}
	return items, nil
}
