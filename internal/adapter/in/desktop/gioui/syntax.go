//go:build desktop || desktop_gio

package gioui

import (
	"image/color"
	"strings"
	"unicode"

	"gioui.org/font"
	"gioui.org/x/richtext"
)

// TokenType represents a category of code token for syntax highlighting.
type TokenType uint8

const (
	TokenPlain TokenType = iota
	TokenKeyword
	TokenTypeIdent
	TokenString
	TokenComment
	TokenNumber
	TokenOperator
)

// Token represents a highlighted piece of source code text.
type Token struct {
	Type TokenType
	Text string
}

type markdownBlockKind uint8

const (
	markdownBlockText markdownBlockKind = iota
	markdownBlockCode
)

type markdownBlock struct {
	kind markdownBlockKind
	lang string
	code string
	text string
}

// splitMarkdownCodeBlocks partitions a markdown response into text blocks and fenced code blocks.
func splitMarkdownCodeBlocks(source string) []markdownBlock {
	lines := strings.Split(source, "\n")
	var blocks []markdownBlock
	var currentText []string
	var currentCode []string
	currentLang := ""
	inCode := false
	fenceChar := byte(0)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !inCode {
			if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
				inCode = true
				fenceChar = trimmed[0]
				currentLang = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(trimmed, "```"), "~~~"))
				if len(currentText) > 0 {
					blocks = append(blocks, markdownBlock{
						kind: markdownBlockText,
						text: strings.Join(currentText, "\n"),
					})
					currentText = nil
				}
				continue
			}
			currentText = append(currentText, line)
		} else {
			if (fenceChar == '`' && strings.HasPrefix(trimmed, "```")) ||
				(fenceChar == '~' && strings.HasPrefix(trimmed, "~~~")) {
				inCode = false
				blocks = append(blocks, markdownBlock{
					kind: markdownBlockCode,
					lang: currentLang,
					code: strings.Join(currentCode, "\n"),
				})
				currentCode = nil
				currentLang = ""
				fenceChar = 0
				continue
			}
			currentCode = append(currentCode, line)
		}
	}

	if inCode && len(currentCode) > 0 {
		blocks = append(blocks, markdownBlock{
			kind: markdownBlockCode,
			lang: currentLang,
			code: strings.Join(currentCode, "\n"),
		})
	} else if len(currentText) > 0 {
		blocks = append(blocks, markdownBlock{
			kind: markdownBlockText,
			text: strings.Join(currentText, "\n"),
		})
	}

	return blocks
}

var goKeywords = map[string]bool{
	"break": true, "default": true, "func": true, "interface": true, "select": true,
	"case": true, "defer": true, "go": true, "map": true, "struct": true,
	"chan": true, "else": true, "goto": true, "package": true, "switch": true,
	"const": true, "fallthrough": true, "if": true, "range": true, "type": true,
	"continue": true, "for": true, "import": true, "return": true, "var": true,
	"nil": true, "true": true, "false": true, "iota": true,
}

var goTypes = map[string]bool{
	"string": true, "int": true, "int64": true, "int32": true, "int16": true, "int8": true,
	"uint": true, "uint64": true, "uint32": true, "uint16": true, "uint8": true,
	"uintptr": true, "byte": true, "rune": true, "bool": true,
	"float64": true, "float32": true, "complex64": true, "complex128": true,
	"error": true, "any": true,
}

var rustKeywords = map[string]bool{
	"as": true, "break": true, "const": true, "continue": true, "crate": true,
	"else": true, "enum": true, "extern": true, "false": true, "fn": true,
	"for": true, "if": true, "impl": true, "in": true, "let": true,
	"loop": true, "match": true, "mod": true, "move": true, "mut": true,
	"pub": true, "ref": true, "return": true, "self": true, "Self": true,
	"static": true, "struct": true, "super": true, "trait": true, "true": true,
	"type": true, "unsafe": true, "use": true, "where": true, "while": true,
	"async": true, "await": true, "dyn": true,
}

var rustTypes = map[string]bool{
	"i8": true, "i16": true, "i32": true, "i64": true, "i128": true, "isize": true,
	"u8": true, "u16": true, "u32": true, "u64": true, "u128": true, "usize": true,
	"f32": true, "f64": true, "bool": true, "char": true, "str": true,
	"String": true, "Vec": true, "Option": true, "Some": true, "None": true,
	"Result": true, "Ok": true, "Err": true, "Box": true,
}

var pythonKeywords = map[string]bool{
	"and": true, "as": true, "assert": true, "async": true, "await": true,
	"break": true, "class": true, "continue": true, "def": true, "del": true,
	"elif": true, "else": true, "except": true, "finally": true, "for": true,
	"from": true, "global": true, "if": true, "import": true, "in": true,
	"is": true, "lambda": true, "nonlocal": true, "not": true, "or": true,
	"pass": true, "raise": true, "return": true, "try": true, "while": true,
	"with": true, "yield": true, "True": true, "False": true, "None": true,
}

var pythonTypes = map[string]bool{
	"int": true, "float": true, "str": true, "bool": true, "list": true,
	"dict": true, "set": true, "tuple": true, "bytes": true, "self": true,
}

var tsKeywords = map[string]bool{
	"break": true, "case": true, "catch": true, "class": true, "const": true,
	"continue": true, "debugger": true, "default": true, "delete": true, "do": true,
	"else": true, "export": true, "extends": true, "finally": true, "for": true,
	"function": true, "if": true, "import": true, "in": true, "instanceof": true,
	"new": true, "return": true, "super": true, "switch": true, "this": true,
	"throw": true, "try": true, "typeof": true, "var": true, "void": true,
	"while": true, "with": true, "yield": true, "let": true, "static": true,
	"async": true, "await": true, "true": true, "false": true, "null": true,
	"undefined": true, "from": true, "as": true,
}

var tsTypes = map[string]bool{
	"string": true, "number": true, "boolean": true, "any": true, "void": true,
	"never": true, "unknown": true, "Promise": true, "Array": true, "Record": true,
	"type": true, "interface": true,
}

var bashKeywords = map[string]bool{
	"if": true, "then": true, "else": true, "elif": true, "fi": true,
	"case": true, "esac": true, "for": true, "while": true, "until": true,
	"do": true, "done": true, "in": true, "function": true, "select": true,
	"time": true, "return": true, "exit": true, "echo": true, "local": true,
	"export": true,
}

var jsonKeywords = map[string]bool{
	"true": true, "false": true, "null": true,
}

var sqlKeywords = map[string]bool{
	"select": true, "from": true, "where": true, "insert": true, "into": true,
	"update": true, "delete": true, "create": true, "table": true, "drop": true,
	"alter": true, "join": true, "left": true, "right": true, "inner": true,
	"outer": true, "on": true, "group": true, "by": true, "order": true,
	"asc": true, "desc": true, "limit": true, "offset": true, "and": true,
	"or": true, "not": true, "in": true, "is": true, "null": true, "as": true,
	"having": true, "union": true,
}

func classifyWord(word, lang string) TokenType {
	lowerLang := strings.ToLower(lang)
	lowerWord := strings.ToLower(word)

	switch lowerLang {
	case "go", "golang":
		if goKeywords[word] {
			return TokenKeyword
		}
		if goTypes[word] {
			return TokenTypeIdent
		}
	case "rust", "rs":
		if rustKeywords[word] {
			return TokenKeyword
		}
		if rustTypes[word] {
			return TokenTypeIdent
		}
	case "python", "py":
		if pythonKeywords[word] {
			return TokenKeyword
		}
		if pythonTypes[word] {
			return TokenTypeIdent
		}
	case "javascript", "js", "typescript", "ts", "jsx", "tsx":
		if tsKeywords[word] {
			return TokenKeyword
		}
		if tsTypes[word] {
			return TokenTypeIdent
		}
	case "bash", "sh", "shell", "zsh":
		if bashKeywords[word] {
			return TokenKeyword
		}
	case "json":
		if jsonKeywords[word] {
			return TokenKeyword
		}
	case "sql":
		if sqlKeywords[lowerWord] {
			return TokenKeyword
		}
	default:
		if goKeywords[word] || rustKeywords[word] || pythonKeywords[word] || tsKeywords[word] {
			return TokenKeyword
		}
		if goTypes[word] || rustTypes[word] || pythonTypes[word] || tsTypes[word] {
			return TokenTypeIdent
		}
	}

	return TokenPlain
}

// tokenizeLine parses a single line of source code into syntax tokens.
func tokenizeLine(line, lang string) []Token {
	runes := []rune(line)
	var tokens []Token
	idx := 0
	n := len(runes)

	commentPrefix := "//"
	switch strings.ToLower(lang) {
	case "python", "py", "bash", "sh", "shell", "zsh", "yaml", "yml", "toml", "dockerfile", "ruby", "rb", "perl":
		commentPrefix = "#"
	case "sql":
		commentPrefix = "--"
	}

	for idx < n {
		if unicode.IsSpace(runes[idx]) {
			start := idx
			for idx < n && unicode.IsSpace(runes[idx]) {
				idx++
			}
			tokens = append(tokens, Token{Type: TokenPlain, Text: string(runes[start:idx])})
			continue
		}

		if strings.HasPrefix(string(runes[idx:]), commentPrefix) {
			tokens = append(tokens, Token{Type: TokenComment, Text: string(runes[idx:])})
			break
		}

		if runes[idx] == '"' || runes[idx] == '\'' || runes[idx] == '`' {
			quote := runes[idx]
			start := idx
			idx++
			for idx < n {
				if runes[idx] == '\\' && quote != '`' && idx+1 < n {
					idx += 2
					continue
				}
				if runes[idx] == quote {
					idx++
					break
				}
				idx++
			}
			tokens = append(tokens, Token{Type: TokenString, Text: string(runes[start:idx])})
			continue
		}

		if unicode.IsDigit(runes[idx]) || (runes[idx] == '.' && idx+1 < n && unicode.IsDigit(runes[idx+1])) {
			start := idx
			if runes[idx] == '0' && idx+1 < n && (runes[idx+1] == 'x' || runes[idx+1] == 'X') {
				idx += 2
				for idx < n && (unicode.IsDigit(runes[idx]) || (runes[idx] >= 'a' && runes[idx] <= 'f') || (runes[idx] >= 'A' && runes[idx] <= 'F') || runes[idx] == '_') {
					idx++
				}
			} else {
				for idx < n && (unicode.IsDigit(runes[idx]) || runes[idx] == '.' || runes[idx] == '_' || runes[idx] == 'e' || runes[idx] == 'E' || ((runes[idx] == '+' || runes[idx] == '-') && idx > 0 && (runes[idx-1] == 'e' || runes[idx-1] == 'E'))) {
					idx++
				}
			}
			tokens = append(tokens, Token{Type: TokenNumber, Text: string(runes[start:idx])})
			continue
		}

		if unicode.IsLetter(runes[idx]) || runes[idx] == '_' {
			start := idx
			for idx < n && (unicode.IsLetter(runes[idx]) || unicode.IsDigit(runes[idx]) || runes[idx] == '_') {
				idx++
			}
			word := string(runes[start:idx])
			tokenType := classifyWord(word, lang)
			tokens = append(tokens, Token{Type: tokenType, Text: word})
			continue
		}

		if idx+1 < n {
			two := string(runes[idx : idx+2])
			switch two {
			case ":=", "==", "!=", "<=", ">=", "&&", "||", "->", "=>", "+=", "-=", "*=", "/=", "::", "??":
				tokens = append(tokens, Token{Type: TokenOperator, Text: two})
				idx += 2
				continue
			}
		}

		tokens = append(tokens, Token{Type: TokenPlain, Text: string(runes[idx : idx+1])})
		idx++
	}

	return tokens
}

// tokenizeCode splits code into lines and tokenizes each line.
func tokenizeCode(code, lang string) [][]Token {
	lines := strings.Split(code, "\n")
	result := make([][]Token, len(lines))
	for i, line := range lines {
		result[i] = tokenizeLine(line, lang)
	}
	return result
}

func tokenColor(t *theme, kind TokenType) color.NRGBA {
	switch kind {
	case TokenKeyword:
		return t.primary
	case TokenTypeIdent:
		return t.tertiary
	case TokenString:
		return t.strength
	case TokenComment:
		return t.secondary
	case TokenNumber:
		return t.agility
	case TokenOperator:
		return t.onSurfaceVariant
	default:
		return t.onSurface
	}
}

// highlightCodeSpans converts source code into syntax-highlighted richtext spans.
func highlightCodeSpans(t *theme, lang, code string) []richtext.SpanStyle {
	tokensByLine := tokenizeCode(code, lang)
	monoFont := t.monoFont(font.Normal)
	fontSize := textBodySmall

	var spans []richtext.SpanStyle
	for lineIdx, lineTokens := range tokensByLine {
		if lineIdx > 0 {
			spans = append(spans, richtext.SpanStyle{
				Font:    monoFont,
				Size:    fontSize,
				Color:   t.onSurface,
				Content: "\n",
			})
		}
		for _, tok := range lineTokens {
			c := tokenColor(t, tok.Type)
			spans = append(spans, richtext.SpanStyle{
				Font:    monoFont,
				Size:    fontSize,
				Color:   c,
				Content: tok.Text,
			})
		}
	}
	return spans
}
