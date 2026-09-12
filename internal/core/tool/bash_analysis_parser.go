package tool

import (
	"path/filepath"
	"strings"
	"unicode"
)

func splitSimpleShell(command string) ([]string, []string, bool) {
	var segments, operators []string
	var b strings.Builder
	var quote rune
	for i := 0; i < len(command); i++ {
		c := rune(command[i])
		if quote != 0 {
			if quote == '"' && (c == '`' || (c == '$' && i+1 < len(command) && command[i+1] == '(')) {
				return nil, nil, false
			}
			b.WriteByte(command[i])
			if c == quote {
				quote = 0
			}
			continue
		}
		if c == '\'' || c == '"' {
			quote = c
			b.WriteByte(command[i])
			continue
		}
		if c == '`' || c == '$' || c == '(' || c == ')' || c == '{' || c == '}' || c == '\n' {
			return nil, nil, false
		}
		if c == ';' {
			return nil, nil, false
		}
		if c == '&' {
			if i > 0 && command[i-1] == '>' {
				b.WriteByte(command[i])
				continue
			}
			if i+1 < len(command) && command[i+1] == '>' {
				b.WriteByte(command[i])
				continue
			}
			if i+1 >= len(command) || command[i+1] != '&' {
				return nil, nil, false
			}
			segments = append(segments, strings.TrimSpace(b.String()))
			b.Reset()
			operators = append(operators, "&&")
			i++
			continue
		}
		if c == '|' {
			if i > 0 && command[i-1] == '>' {
				b.WriteByte(command[i])
				continue
			}
			if i+1 < len(command) && command[i+1] == '|' {
				segments = append(segments, strings.TrimSpace(b.String()))
				b.Reset()
				operators = append(operators, "||")
				i++
				continue
			}
			if i+1 < len(command) && command[i+1] == '&' {
				segments = append(segments, strings.TrimSpace(b.String()))
				b.Reset()
				operators = append(operators, "|&")
				i++
				continue
			}
			segments = append(segments, strings.TrimSpace(b.String()))
			b.Reset()
			operators = append(operators, "|")
			continue
		}
		b.WriteByte(command[i])
	}
	if quote != 0 {
		return nil, nil, false
	}
	segments = append(segments, strings.TrimSpace(b.String()))
	for _, s := range segments {
		if s == "" {
			return nil, nil, false
		}
	}
	return segments, operators, true
}

func shellWords(segment string) ([]string, []string, bool) {
	var words, redirects []string
	var b strings.Builder
	var quote rune
	flush := func() {
		if b.Len() > 0 {
			words = append(words, b.String())
			b.Reset()
		}
	}
	for i := 0; i < len(segment); i++ {
		c := rune(segment[i])
		if quote != 0 {
			if c == quote {
				quote = 0
			} else {
				b.WriteByte(segment[i])
			}
			continue
		}
		if c == '\'' || c == '"' {
			quote = c
			continue
		}
		if unicode.IsSpace(c) {
			flush()
			continue
		}
		if c == '&' && i+1 < len(segment) && segment[i+1] == '>' {
			continue
		}
		if c == '>' {
			if b.Len() > 0 && allDigits(b.String()) {
				b.Reset()
			} else {
				flush()
			}
			if i+1 < len(segment) && (segment[i+1] == '>' || segment[i+1] == '|') {
				i++
			}
			for i+1 < len(segment) && unicode.IsSpace(rune(segment[i+1])) {
				i++
			}
			j := i + 1
			for j < len(segment) && !unicode.IsSpace(rune(segment[j])) {
				j++
			}
			if j == i+1 {
				return nil, nil, false
			}
			redirects = append(redirects, strings.Trim(segment[i+1:j], "'\""))
			i = j - 1
			continue
		}
		if c == '<' {
			return nil, nil, false
		}
		if c == '\\' && i+1 < len(segment) {
			i++
			b.WriteByte(segment[i])
			continue
		}
		b.WriteByte(segment[i])
	}
	if quote != 0 {
		return nil, nil, false
	}
	flush()
	return words, redirects, true
}

func literalMutationPaths(name string, args []string) []string {
	paths := make([]string, 0, len(args))
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") || !isLiteralWorkspacePath(arg) {
			continue
		}
		paths = appendUniquePaths(paths, arg)
	}
	return paths
}

func copyMovePaths(args []string, includeSource bool) []string {
	pos := make([]string, 0, len(args))
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") && isLiteralWorkspacePath(arg) {
			pos = append(pos, arg)
		}
	}
	if len(pos) < 2 {
		return nil
	}
	if includeSource {
		return appendUniquePaths(nil, pos...)
	}
	return appendUniquePaths(nil, pos[len(pos)-1])
}

func isStreamOnlyRedirect(target string) bool {
	target = strings.TrimSpace(target)
	if strings.HasPrefix(target, "&") {
		if len(target) > 1 && allDigits(target[1:]) {
			return true
		}
	}
	switch target {
	case "/dev/null", "/dev/stdout", "/dev/stderr":
		return true
	default:
		return false
	}
}

func allDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func teePaths(args []string) ([]string, bool) {
	paths := make([]string, 0, len(args))
	hasTarget := false
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		hasTarget = true
		if isLiteralWorkspacePath(arg) {
			paths = appendUniquePaths(paths, arg)
		}
	}
	return paths, hasTarget
}

func isLiteralWorkspacePath(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" || filepath.IsAbs(path) || strings.ContainsAny(path, "$*?[]{}~`)\n\r") {
		return false
	}
	clean := filepath.Clean(path)
	return clean != "." && clean != ".." && !strings.HasPrefix(clean, ".."+string(filepath.Separator))
}

func appendUniquePaths(dst []string, paths ...string) []string {
	for _, path := range paths {
		path = filepath.Clean(strings.TrimSpace(path))
		if path == "" || path == "." {
			continue
		}
		seen := false
		for _, existing := range dst {
			if existing == path {
				seen = true
				break
			}
		}
		if !seen {
			dst = append(dst, path)
		}
	}
	return dst
}
