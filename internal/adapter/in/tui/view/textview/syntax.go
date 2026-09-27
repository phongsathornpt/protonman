package textview

import (
	"strings"
	"unicode"

	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
)

var (
	syntaxKeywordStyle  = tuistyle.SyntaxKeywordStyle
	syntaxStringStyle   = tuistyle.SyntaxStringStyle
	syntaxCommentStyle  = tuistyle.SyntaxCommentStyle
	syntaxNumberStyle   = tuistyle.SyntaxNumberStyle
	syntaxTypeStyle     = tuistyle.SyntaxTypeStyle
	syntaxFunctionStyle = tuistyle.SyntaxFunctionStyle
	syntaxOperatorStyle = tuistyle.SyntaxOperatorStyle
	syntaxTextStyle     = tuistyle.SyntaxTextStyle

	syntaxKeywordInline  SimpleANSIStyle
	syntaxStringInline   SimpleANSIStyle
	syntaxCommentInline  SimpleANSIStyle
	syntaxNumberInline   SimpleANSIStyle
	syntaxTypeInline     SimpleANSIStyle
	syntaxFunctionInline SimpleANSIStyle
	syntaxOperatorInline SimpleANSIStyle
	syntaxTextInline     SimpleANSIStyle
)

type tokenKind int

const (
	tokPlain tokenKind = iota
	tokKeyword
	tokString
	tokComment
	tokNumber
	tokType
	tokFunction
	tokOperator
)

func renderToken(b *strings.Builder, kind tokenKind, text string) {
	if text == "" {
		return
	}
	switch kind {
	case tokKeyword:
		syntaxKeywordInline.WriteTo(b, syntaxKeywordStyle, text)
	case tokString:
		syntaxStringInline.WriteTo(b, syntaxStringStyle, text)
	case tokComment:
		syntaxCommentInline.WriteTo(b, syntaxCommentStyle, text)
	case tokNumber:
		syntaxNumberInline.WriteTo(b, syntaxNumberStyle, text)
	case tokType:
		syntaxTypeInline.WriteTo(b, syntaxTypeStyle, text)
	case tokFunction:
		syntaxFunctionInline.WriteTo(b, syntaxFunctionStyle, text)
	case tokOperator:
		syntaxOperatorInline.WriteTo(b, syntaxOperatorStyle, text)
	default:
		syntaxTextInline.WriteTo(b, syntaxTextStyle, text)
	}
}

// NormalizeLanguage canonicalizes common language names and aliases.
func NormalizeLanguage(raw string) string {
	clean := strings.ToLower(strings.TrimSpace(raw))
	switch clean {
	case "go", "golang":
		return "go"
	case "json", "jsonc":
		return "json"
	case "sh", "bash", "shell", "zsh":
		return "bash"
	case "py", "python", "python3":
		return "python"
	case "js", "javascript", "jsx", "mjs", "cjs":
		return "javascript"
	case "ts", "typescript", "tsx", "mts", "cts":
		return "typescript"
	case "md", "markdown":
		return "markdown"
	case "diff", "patch":
		return "diff"
	case "yaml", "yml":
		return "yaml"
	case "toml":
		return "toml"
	case "sql":
		return "sql"
	case "html", "htm", "xml", "svg":
		return "html"
	case "css", "scss", "sass", "less":
		return "css"
	case "rs", "rust":
		return "rust"
	case "c", "h":
		return "c"
	case "cpp", "cxx", "cc", "hpp":
		return "cpp"
	case "java":
		return "java"
	default:
		return "generic"
	}
}

// HighlightCodeLine highlights a single line of code according to the given language
// and updates the MarkdownState for multi-line comments or strings.
func HighlightCodeLine(line string, rawLang string, state *MarkdownState) string {
	lang := NormalizeLanguage(rawLang)
	switch lang {
	case "diff":
		return highlightDiff(line)
	case "json":
		return highlightJSON(line, state)
	case "yaml":
		return highlightYAML(line, state)
	case "toml":
		return highlightTOML(line, state)
	case "html":
		return highlightHTML(line, state)
	case "css":
		return highlightCSS(line, state)
	case "markdown":
		return highlightMarkdown(line, state)
	default:
		cfg := languageConfig(lang)
		return highlightGenericConfig(line, cfg, state)
	}
}

func highlightDiff(line string) string {
	if line == "" {
		return ""
	}
	switch {
	case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---") || strings.HasPrefix(line, "diff ") || strings.HasPrefix(line, "index "):
		return tuistyle.ToolTargetStyle.Render(line)
	case strings.HasPrefix(line, "@@"):
		return tuistyle.DiffHunkStyle.Render(line)
	case strings.HasPrefix(line, "+"):
		return tuistyle.DiffAddStyle.Render(line)
	case strings.HasPrefix(line, "-"):
		return tuistyle.DiffDeleteStyle.Render(line)
	default:
		var b strings.Builder
		renderToken(&b, tokPlain, line)
		return b.String()
	}
}

func highlightJSON(line string, state *MarkdownState) string {
	var b strings.Builder
	idx := 0
	runes := []rune(line)
	n := len(runes)

	for idx < n {
		r := runes[idx]
		if unicode.IsSpace(r) {
			b.WriteRune(r)
			idx++
			continue
		}

		// Support comments in JSONC
		if r == '/' && idx+1 < n && runes[idx+1] == '/' {
			renderToken(&b, tokComment, string(runes[idx:]))
			break
		}

		if r == '"' {
			start := idx
			idx++
			escaped := false
			for idx < n {
				cur := runes[idx]
				if escaped {
					escaped = false
					idx++
					continue
				}
				if cur == '\\' {
					escaped = true
					idx++
					continue
				}
				if cur == '"' {
					idx++
					break
				}
				idx++
			}
			strVal := string(runes[start:idx])

			// Peek ahead: is this string an object key?
			peek := idx
			for peek < n && unicode.IsSpace(runes[peek]) {
				peek++
			}
			if peek < n && runes[peek] == ':' {
				renderToken(&b, tokType, strVal)
			} else {
				renderToken(&b, tokString, strVal)
			}
			continue
		}

		if isDigit(r) || (r == '-' && idx+1 < n && isDigit(runes[idx+1])) {
			start := idx
			idx++
			for idx < n && (isDigit(runes[idx]) || runes[idx] == '.' || runes[idx] == 'e' || runes[idx] == 'E' || runes[idx] == '+' || runes[idx] == '-') {
				idx++
			}
			renderToken(&b, tokNumber, string(runes[start:idx]))
			continue
		}

		if isWordStart(r) {
			start := idx
			for idx < n && isWordChar(runes[idx]) {
				idx++
			}
			word := string(runes[start:idx])
			if word == "true" || word == "false" || word == "null" {
				renderToken(&b, tokKeyword, word)
			} else {
				renderToken(&b, tokPlain, word)
			}
			continue
		}

		renderToken(&b, tokOperator, string(r))
		idx++
	}
	return b.String()
}

func highlightYAML(line string, state *MarkdownState) string {
	var b strings.Builder
	runes := []rune(line)
	n := len(runes)
	idx := 0

	// Whitespace
	for idx < n && unicode.IsSpace(runes[idx]) {
		b.WriteRune(runes[idx])
		idx++
	}
	if idx >= n {
		return b.String()
	}

	// Comment
	if runes[idx] == '#' {
		renderToken(&b, tokComment, string(runes[idx:]))
		return b.String()
	}

	// Check if this line starts with a list marker "- "
	if runes[idx] == '-' && idx+1 < n && unicode.IsSpace(runes[idx+1]) {
		renderToken(&b, tokOperator, "- ")
		idx += 2
		for idx < n && unicode.IsSpace(runes[idx]) {
			b.WriteRune(runes[idx])
			idx++
		}
	}

	// Check if there is a key before ':'
	colonIdx := -1
	inQuote := rune(0)
	for i := idx; i < n; i++ {
		r := runes[i]
		if inQuote != 0 {
			if r == inQuote && runes[i-1] != '\\' {
				inQuote = 0
			}
			continue
		}
		if r == '"' || r == '\'' {
			inQuote = r
			continue
		}
		if r == ':' && (i+1 == n || unicode.IsSpace(runes[i+1])) {
			colonIdx = i
			break
		}
	}

	if colonIdx != -1 {
		renderToken(&b, tokType, string(runes[idx:colonIdx]))
		renderToken(&b, tokOperator, ":")
		idx = colonIdx + 1
	}

	for idx < n {
		r := runes[idx]
		if unicode.IsSpace(r) {
			b.WriteRune(r)
			idx++
			continue
		}
		if r == '#' {
			renderToken(&b, tokComment, string(runes[idx:]))
			break
		}
		if r == '"' || r == '\'' {
			start := idx
			quote := r
			idx++
			for idx < n {
				if runes[idx] == quote && runes[idx-1] != '\\' {
					idx++
					break
				}
				idx++
			}
			renderToken(&b, tokString, string(runes[start:idx]))
			continue
		}
		if isDigit(r) || ((r == '-' || r == '+') && idx+1 < n && isDigit(runes[idx+1])) {
			start := idx
			idx++
			for idx < n && (isDigit(runes[idx]) || runes[idx] == '.' || runes[idx] == 'e' || runes[idx] == 'E') {
				idx++
			}
			renderToken(&b, tokNumber, string(runes[start:idx]))
			continue
		}
		if isWordStart(r) {
			start := idx
			for idx < n && isWordChar(runes[idx]) {
				idx++
			}
			word := string(runes[start:idx])
			lower := strings.ToLower(word)
			if lower == "true" || lower == "false" || lower == "yes" || lower == "no" || lower == "null" || lower == "~" {
				renderToken(&b, tokKeyword, word)
			} else {
				renderToken(&b, tokPlain, word)
			}
			continue
		}
		renderToken(&b, tokOperator, string(r))
		idx++
	}
	return b.String()
}

func highlightTOML(line string, state *MarkdownState) string {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
		var b strings.Builder
		prefix := line[:strings.Index(line, "[")]
		b.WriteString(prefix)
		renderToken(&b, tokKeyword, trimmed)
		return b.String()
	}
	return highlightYAML(line, state)
}

func highlightHTML(line string, state *MarkdownState) string {
	var b strings.Builder
	runes := []rune(line)
	n := len(runes)
	idx := 0

	if state != nil && state.inBlockComment {
		closeIdx := strings.Index(line, "-->")
		if closeIdx == -1 {
			renderToken(&b, tokComment, line)
			return b.String()
		}
		renderToken(&b, tokComment, line[:closeIdx+3])
		state.inBlockComment = false
		idx = closeIdx + 3
	}

	for idx < n {
		if idx+4 <= n && string(runes[idx:idx+4]) == "<!--" {
			closeIdx := strings.Index(string(runes[idx:]), "-->")
			if closeIdx == -1 {
				renderToken(&b, tokComment, string(runes[idx:]))
				if state != nil {
					state.inBlockComment = true
				}
				break
			}
			end := idx + closeIdx + 3
			renderToken(&b, tokComment, string(runes[idx:end]))
			idx = end
			continue
		}

		r := runes[idx]
		if r == '<' {
			start := idx
			idx++
			if idx < n && (runes[idx] == '/' || runes[idx] == '!' || runes[idx] == '?') {
				idx++
			}
			for idx < n && isWordChar(runes[idx]) {
				idx++
			}
			renderToken(&b, tokKeyword, string(runes[start:idx]))
			continue
		}

		if r == '>' || (r == '/' && idx+1 < n && runes[idx+1] == '>') {
			if r == '/' {
				renderToken(&b, tokKeyword, "/>")
				idx += 2
			} else {
				renderToken(&b, tokKeyword, ">")
				idx++
			}
			continue
		}

		if r == '"' || r == '\'' {
			start := idx
			quote := r
			idx++
			for idx < n {
				if runes[idx] == quote {
					idx++
					break
				}
				idx++
			}
			renderToken(&b, tokString, string(runes[start:idx]))
			continue
		}

		if isWordStart(r) {
			start := idx
			for idx < n && isWordChar(runes[idx]) {
				idx++
			}
			word := string(runes[start:idx])
			if idx < n && runes[idx] == '=' {
				renderToken(&b, tokType, word)
				renderToken(&b, tokOperator, "=")
				idx++
			} else {
				renderToken(&b, tokPlain, word)
			}
			continue
		}

		if unicode.IsSpace(r) {
			b.WriteRune(r)
			idx++
			continue
		}

		renderToken(&b, tokPlain, string(r))
		idx++
	}
	return b.String()
}

func highlightCSS(line string, state *MarkdownState) string {
	var b strings.Builder
	runes := []rune(line)
	n := len(runes)
	idx := 0

	if state != nil && state.inBlockComment {
		closeIdx := strings.Index(line, "*/")
		if closeIdx == -1 {
			renderToken(&b, tokComment, line)
			return b.String()
		}
		renderToken(&b, tokComment, line[:closeIdx+2])
		state.inBlockComment = false
		idx = closeIdx + 2
	}

	for idx < n {
		if idx+2 <= n && string(runes[idx:idx+2]) == "/*" {
			closeIdx := strings.Index(string(runes[idx:]), "*/")
			if closeIdx == -1 {
				renderToken(&b, tokComment, string(runes[idx:]))
				if state != nil {
					state.inBlockComment = true
				}
				break
			}
			end := idx + closeIdx + 2
			renderToken(&b, tokComment, string(runes[idx:end]))
			idx = end
			continue
		}

		r := runes[idx]
		if unicode.IsSpace(r) {
			b.WriteRune(r)
			idx++
			continue
		}

		if r == '{' || r == '}' || r == ':' || r == ';' {
			renderToken(&b, tokOperator, string(r))
			idx++
			continue
		}

		if r == '.' || r == '#' {
			start := idx
			idx++
			for idx < n && isWordChar(runes[idx]) {
				idx++
			}
			renderToken(&b, tokType, string(runes[start:idx]))
			continue
		}

		if isDigit(r) {
			start := idx
			for idx < n && (isDigit(runes[idx]) || runes[idx] == '.' || isWordChar(runes[idx]) || runes[idx] == '%') {
				idx++
			}
			renderToken(&b, tokNumber, string(runes[start:idx]))
			continue
		}

		if isWordStart(r) {
			start := idx
			for idx < n && (isWordChar(runes[idx]) || runes[idx] == '-') {
				idx++
			}
			renderToken(&b, tokPlain, string(runes[start:idx]))
			continue
		}

		renderToken(&b, tokPlain, string(r))
		idx++
	}
	return b.String()
}

func highlightMarkdown(line string, state *MarkdownState) string {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "#") {
		return tuistyle.MarkdownHeadingStyle.Render(line)
	}
	if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") || strings.HasPrefix(trimmed, "+ ") {
		idx := strings.IndexAny(line, "-*+")
		var b strings.Builder
		b.WriteString(line[:idx])
		renderToken(&b, tokOperator, string(line[idx]))
		b.WriteString(line[idx+1:])
		return b.String()
	}
	if strings.HasPrefix(trimmed, ">") {
		return tuistyle.MarkdownQuoteStyle.Render(line)
	}
	var b strings.Builder
	renderToken(&b, tokPlain, line)
	return b.String()
}

type langConfig struct {
	lineComments    []string
	hasBlockComment bool
	hasBackticks    bool
	hasTripleQuotes bool
	hasBashVars     bool
	caseInsensitive bool
	keywords        map[string]bool
	types           map[string]bool
	builtins        map[string]bool
}

func highlightGenericConfig(line string, cfg langConfig, state *MarkdownState) string {
	var b strings.Builder
	runes := []rune(line)
	n := len(runes)
	idx := 0

	// 1. Check if continuation of block comment
	if state != nil && state.inBlockComment {
		closeIdx := strings.Index(line, "*/")
		if closeIdx == -1 {
			renderToken(&b, tokComment, line)
			return b.String()
		}
		renderToken(&b, tokComment, line[:closeIdx+2])
		state.inBlockComment = false
		idx = closeIdx + 2
	}

	// 2. Check if continuation of multiline string
	if state != nil && state.inMultilineStr != 0 {
		quote := state.inMultilineStr
		var closeIdx int
		var delimLen int
		if quote == '"' && cfg.hasTripleQuotes {
			closeIdx = strings.Index(line, `"""`)
			delimLen = 3
		} else if quote == '\'' && cfg.hasTripleQuotes {
			closeIdx = strings.Index(line, `'''`)
			delimLen = 3
		} else {
			closeIdx = strings.IndexRune(line, quote)
			delimLen = 1
		}
		if closeIdx == -1 {
			renderToken(&b, tokString, line)
			return b.String()
		}
		renderToken(&b, tokString, line[:closeIdx+delimLen])
		state.inMultilineStr = 0
		idx = closeIdx + delimLen
	}

	for idx < n {
		r := runes[idx]

		// Whitespace
		if unicode.IsSpace(r) {
			b.WriteRune(r)
			idx++
			continue
		}

		// Single line comment
		commentFound := false
		for _, prefix := range cfg.lineComments {
			if strings.HasPrefix(string(runes[idx:]), prefix) {
				renderToken(&b, tokComment, string(runes[idx:]))
				return b.String()
			}
		}
		if commentFound {
			break
		}

		// Block comment
		if cfg.hasBlockComment && strings.HasPrefix(string(runes[idx:]), "/*") {
			closeIdx := strings.Index(string(runes[idx+2:]), "*/")
			if closeIdx == -1 {
				renderToken(&b, tokComment, string(runes[idx:]))
				if state != nil {
					state.inBlockComment = true
				}
				break
			}
			end := idx + 2 + closeIdx + 2
			renderToken(&b, tokComment, string(runes[idx:end]))
			idx = end
			continue
		}

		// Python triple quotes
		if cfg.hasTripleQuotes {
			if strings.HasPrefix(string(runes[idx:]), `"""`) {
				closeIdx := strings.Index(string(runes[idx+3:]), `"""`)
				if closeIdx == -1 {
					renderToken(&b, tokString, string(runes[idx:]))
					if state != nil {
						state.inMultilineStr = '"'
					}
					break
				}
				end := idx + 3 + closeIdx + 3
				renderToken(&b, tokString, string(runes[idx:end]))
				idx = end
				continue
			}
			if strings.HasPrefix(string(runes[idx:]), `'''`) {
				closeIdx := strings.Index(string(runes[idx+3:]), `'''`)
				if closeIdx == -1 {
					renderToken(&b, tokString, string(runes[idx:]))
					if state != nil {
						state.inMultilineStr = '\''
					}
					break
				}
				end := idx + 3 + closeIdx + 3
				renderToken(&b, tokString, string(runes[idx:end]))
				idx = end
				continue
			}
		}

		// Backtick string
		if cfg.hasBackticks && r == '`' {
			closeIdx := strings.IndexRune(string(runes[idx+1:]), '`')
			if closeIdx == -1 {
				renderToken(&b, tokString, string(runes[idx:]))
				if state != nil {
					state.inMultilineStr = '`'
				}
				break
			}
			end := idx + 1 + closeIdx + 1
			renderToken(&b, tokString, string(runes[idx:end]))
			idx = end
			continue
		}

		// Regular strings
		if r == '"' || r == '\'' {
			start := idx
			quote := r
			idx++
			escaped := false
			for idx < n {
				cur := runes[idx]
				if escaped {
					escaped = false
					idx++
					continue
				}
				if cur == '\\' {
					escaped = true
					idx++
					continue
				}
				if cur == quote {
					idx++
					break
				}
				idx++
			}
			renderToken(&b, tokString, string(runes[start:idx]))
			continue
		}

		// Bash variables
		if cfg.hasBashVars && r == '$' {
			start := idx
			idx++
			if idx < n && runes[idx] == '{' {
				for idx < n && runes[idx] != '}' {
					idx++
				}
				if idx < n && runes[idx] == '}' {
					idx++
				}
			} else if idx < n && (runes[idx] == '?' || runes[idx] == '@' || runes[idx] == '*' || runes[idx] == '#' || isDigit(runes[idx])) {
				idx++
			} else {
				for idx < n && isWordChar(runes[idx]) {
					idx++
				}
			}
			renderToken(&b, tokType, string(runes[start:idx]))
			continue
		}

		// Numbers
		if isDigit(r) || (r == '.' && idx+1 < n && isDigit(runes[idx+1])) {
			start := idx
			idx++
			for idx < n && (isDigit(runes[idx]) || runes[idx] == '.' || runes[idx] == 'x' || runes[idx] == 'X' ||
				runes[idx] == 'b' || runes[idx] == 'B' || runes[idx] == 'o' || runes[idx] == 'O' ||
				runes[idx] == 'e' || runes[idx] == 'E' || (runes[idx] >= 'a' && runes[idx] <= 'f') ||
				(runes[idx] >= 'A' && runes[idx] <= 'F') || runes[idx] == '_' || runes[idx] == 'n' || runes[idx] == 'f' || runes[idx] == 'L') {
				idx++
			}
			renderToken(&b, tokNumber, string(runes[start:idx]))
			continue
		}

		// Words / Identifiers
		if isWordStart(r) {
			start := idx
			for idx < n && isWordChar(runes[idx]) {
				idx++
			}
			word := string(runes[start:idx])
			lookup := word
			if cfg.caseInsensitive {
				lookup = strings.ToUpper(word)
			}

			// Check if function call (followed by '(')
			peek := idx
			for peek < n && unicode.IsSpace(runes[peek]) {
				peek++
			}
			isFuncCall := peek < n && runes[peek] == '('

			if cfg.keywords[lookup] {
				renderToken(&b, tokKeyword, word)
			} else if cfg.types[lookup] {
				renderToken(&b, tokType, word)
			} else if cfg.builtins[lookup] || isFuncCall {
				renderToken(&b, tokFunction, word)
			} else {
				renderToken(&b, tokPlain, word)
			}
			continue
		}

		// Operators & punctuation
		renderToken(&b, tokOperator, string(r))
		idx++
	}

	return b.String()
}

func isDigit(r rune) bool {
	return r >= '0' && r <= '9'
}

func isWordStart(r rune) bool {
	return r == '_' || unicode.IsLetter(r)
}

func isWordChar(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func languageConfig(lang string) langConfig {
	switch lang {
	case "go":
		return langConfig{
			lineComments:    []string{"//"},
			hasBlockComment: true,
			hasBackticks:    true,
			keywords: toSet(
				"break", "case", "chan", "const", "continue", "default", "defer", "else",
				"fallthrough", "for", "func", "go", "goto", "if", "import", "interface",
				"map", "package", "range", "return", "select", "struct", "switch", "type", "var",
			),
			types: toSet(
				"bool", "byte", "complex64", "complex128", "error", "float32", "float64",
				"int", "int8", "int16", "int32", "int64", "rune", "string", "uint",
				"uint8", "uint16", "uint32", "uint64", "uintptr", "any", "comparable",
				"true", "false", "iota", "nil",
			),
			builtins: toSet(
				"append", "cap", "clear", "close", "complex", "copy", "delete", "imag",
				"len", "make", "max", "min", "new", "panic", "print", "println", "real", "recover",
			),
		}
	case "python":
		return langConfig{
			lineComments:    []string{"#"},
			hasTripleQuotes: true,
			keywords: toSet(
				"def", "class", "import", "from", "as", "return", "if", "elif", "else",
				"while", "for", "in", "try", "except", "finally", "raise", "with",
				"yield", "lambda", "pass", "break", "continue", "global", "nonlocal",
				"assert", "async", "await", "match", "case", "del", "not", "and", "or", "is",
			),
			types: toSet(
				"True", "False", "None", "self", "cls", "int", "float", "str", "bool",
				"list", "dict", "set", "tuple", "bytes", "type", "object",
			),
			builtins: toSet(
				"print", "len", "range", "enumerate", "zip", "map", "filter", "sorted",
				"reversed", "open", "super", "isinstance", "issubclass", "sum", "min", "max",
			),
		}
	case "javascript", "typescript":
		return langConfig{
			lineComments:    []string{"//"},
			hasBlockComment: true,
			hasBackticks:    true,
			keywords: toSet(
				"async", "await", "break", "case", "catch", "class", "const", "continue",
				"debugger", "default", "delete", "do", "else", "export", "extends", "finally",
				"for", "function", "if", "import", "in", "instanceof", "let", "new",
				"return", "super", "switch", "this", "throw", "try", "typeof", "var",
				"void", "while", "with", "yield", "type", "interface", "enum", "implements",
				"declare", "namespace", "from", "as", "readonly",
			),
			types: toSet(
				"true", "false", "null", "undefined", "NaN", "Infinity", "string", "number",
				"boolean", "any", "unknown", "never", "void", "symbol", "bigint", "Promise",
				"Array", "Object", "Record", "Map", "Set",
			),
			builtins: toSet(
				"console", "window", "document", "process", "Math", "JSON", "require",
			),
		}
	case "bash":
		return langConfig{
			lineComments: []string{"#"},
			hasBashVars:  true,
			keywords: toSet(
				"if", "then", "else", "elif", "fi", "case", "esac", "for", "while",
				"until", "do", "done", "in", "function", "select", "time", "return", "exit",
				"export", "source", "local", "alias", "read", "echo", "cd", "pwd", "ls",
				"grep", "sed", "awk", "find", "cat", "rm", "cp", "mv", "chmod", "curl", "git",
			),
			types: toSet(
				"true", "false",
			),
			builtins: toSet(
				"set", "unset", "shift", "test", "exec",
			),
		}
	case "sql":
		return langConfig{
			lineComments:    []string{"--"},
			hasBlockComment: true,
			caseInsensitive: true,
			keywords: toSet(
				"SELECT", "FROM", "WHERE", "INSERT", "INTO", "UPDATE", "DELETE", "CREATE",
				"TABLE", "DROP", "ALTER", "JOIN", "INNER", "LEFT", "RIGHT", "OUTER", "CROSS",
				"ON", "GROUP", "BY", "ORDER", "ASC", "DESC", "HAVING", "LIMIT", "OFFSET",
				"UNION", "ALL", "AND", "OR", "NOT", "IN", "IS", "NULL", "AS", "SET",
				"VALUES", "PRIMARY", "KEY", "FOREIGN", "REFERENCES", "INDEX", "VIEW",
				"CASE", "WHEN", "THEN", "ELSE", "END", "EXISTS", "BETWEEN", "LIKE", "DISTINCT", "WITH",
			),
			types: toSet(
				"INT", "INTEGER", "VARCHAR", "CHAR", "TEXT", "BOOLEAN", "DATE", "TIMESTAMP", "FLOAT", "DOUBLE",
			),
			builtins: toSet(
				"COUNT", "SUM", "AVG", "MIN", "MAX", "COALESCE", "NOW", "LOWER", "UPPER",
			),
		}
	case "rust":
		return langConfig{
			lineComments:    []string{"//"},
			hasBlockComment: true,
			keywords: toSet(
				"as", "break", "const", "continue", "crate", "else", "enum", "extern",
				"false", "fn", "for", "if", "impl", "in", "let", "loop", "match", "mod",
				"move", "mut", "pub", "ref", "return", "self", "Self", "static", "struct",
				"super", "trait", "true", "type", "unsafe", "use", "where", "while", "async", "await",
			),
			types: toSet(
				"i8", "i16", "i32", "i64", "i128", "isize", "u8", "u16", "u32", "u64", "u128", "usize",
				"f32", "f64", "str", "bool", "char", "Option", "Result", "Some", "None", "Ok", "Err",
				"String", "Vec", "Box",
			),
			builtins: toSet(
				"println", "print", "format", "panic", "vec",
			),
		}
	case "c", "cpp":
		return langConfig{
			lineComments:    []string{"//"},
			hasBlockComment: true,
			keywords: toSet(
				"auto", "break", "case", "char", "const", "continue", "default", "do",
				"double", "else", "enum", "extern", "float", "for", "goto", "if", "int",
				"long", "register", "return", "short", "signed", "sizeof", "static",
				"struct", "switch", "typedef", "union", "unsigned", "void", "volatile", "while",
				"class", "public", "private", "protected", "virtual", "template", "typename",
				"namespace", "using", "new", "delete", "nullptr", "bool", "true", "false",
			),
			types: toSet(
				"size_t", "uint8_t", "uint16_t", "uint32_t", "uint64_t",
				"int8_t", "int16_t", "int32_t", "int64_t", "string", "vector", "map",
			),
			builtins: toSet(
				"printf", "scanf", "cout", "cin", "endl", "malloc", "free",
			),
		}
	case "java":
		return langConfig{
			lineComments:    []string{"//"},
			hasBlockComment: true,
			keywords: toSet(
				"abstract", "assert", "boolean", "break", "byte", "case", "catch", "char",
				"class", "const", "continue", "default", "do", "double", "else", "enum",
				"extends", "final", "finally", "float", "for", "goto", "if", "implements",
				"import", "instanceof", "int", "interface", "long", "native", "new",
				"package", "private", "protected", "public", "return", "short", "static",
				"strictfp", "super", "switch", "synchronized", "this", "throw", "throws",
				"transient", "try", "void", "volatile", "while",
			),
			types: toSet(
				"String", "Integer", "Long", "Boolean", "Double", "Float", "Object", "List",
				"Map", "Set", "true", "false", "null",
			),
			builtins: toSet(
				"System", "out", "println", "print",
			),
		}
	default:
		return langConfig{
			lineComments:    []string{"//", "#", "--"},
			hasBlockComment: true,
			hasBackticks:    true,
			keywords: toSet(
				"if", "else", "for", "while", "return", "func", "function", "class",
				"def", "var", "let", "const", "true", "false", "nil", "null", "import", "export",
			),
			types: toSet(
				"int", "string", "bool", "float",
			),
			builtins: toSet(),
		}
	}
}

func toSet(items ...string) map[string]bool {
	set := make(map[string]bool, len(items))
	for _, item := range items {
		set[item] = true
	}
	return set
}
