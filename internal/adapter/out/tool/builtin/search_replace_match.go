package builtin

import (
	"fmt"
	"strings"
	"unicode"
)

type replaceMatch struct {
	startOffset int
	endOffset   int
	replacement string
	tier        int // 1: exact, 2: crlf-normalized, 3: whitespace-tolerant
}

// findReplacements locates all target spans for oldString in content according to
// multi-tier matching rules:
//
//	Tier 1: Exact byte match
//	Tier 2: CRLF / LF line-ending agnostic match
//	Tier 3: Whitespace & indentation tolerant match (requires uniqueness unless replaceAll)
//
// If no match is found across all tiers, a descriptive error with near-match diagnostics is returned.
func findReplacements(content, oldString, newString string, replaceAll bool, filePath string) ([]replaceMatch, error) {
	if oldString == "" {
		return nil, fmt.Errorf("oldString cannot be empty")
	}

	// Tier 1: Exact match
	if exactMatches := findExactMatches(content, oldString, newString); len(exactMatches) > 0 {
		if len(exactMatches) > 1 && !replaceAll {
			return nil, fmt.Errorf("oldString matched %d locations in %q; use replaceAll for multiple matches", len(exactMatches), filePath)
		}
		return exactMatches, nil
	}

	// Tier 2: CRLF / LF normalization match
	if crlfMatches := findCRLFMatches(content, oldString, newString); len(crlfMatches) > 0 {
		if len(crlfMatches) > 1 && !replaceAll {
			return nil, fmt.Errorf("oldString matched %d locations in %q (differing by line endings); use replaceAll for multiple matches", len(crlfMatches), filePath)
		}
		return crlfMatches, nil
	}

	// Tier 3: Whitespace & indentation tolerant match
	if wsMatches, wsErr := findWhitespaceMatches(content, oldString, newString, replaceAll, filePath); len(wsMatches) > 0 {
		return wsMatches, nil
	} else if wsErr != nil {
		return nil, wsErr
	}

	// Tier 4: No match across all tiers - build informative near-match diagnostic
	diagnostic := buildNearMatchDiagnostic(content, oldString, filePath)
	return nil, fmt.Errorf("%s", diagnostic)
}

func findExactMatches(content, oldString, newString string) []replaceMatch {
	count := strings.Count(content, oldString)
	if count == 0 {
		return nil
	}
	matches := make([]replaceMatch, 0, count)
	start := 0
	oldLen := len(oldString)
	for {
		idx := strings.Index(content[start:], oldString)
		if idx == -1 {
			break
		}
		absStart := start + idx
		absEnd := absStart + oldLen
		matches = append(matches, replaceMatch{
			startOffset: absStart,
			endOffset:   absEnd,
			replacement: newString,
			tier:        1,
		})
		start = absEnd
	}
	return matches
}

func findCRLFMatches(content, oldString, newString string) []replaceMatch {
	hasCRLFContent := strings.Contains(content, "\r\n")
	hasCRLFOld := strings.Contains(oldString, "\r\n")
	if !hasCRLFContent && !hasCRLFOld {
		return nil
	}

	normContent := strings.ReplaceAll(content, "\r\n", "\n")
	normOld := strings.ReplaceAll(oldString, "\r\n", "\n")
	if normOld == "" || strings.Count(normContent, normOld) == 0 {
		return nil
	}

	// If the file content uses CRLF, normalize newString to CRLF as well to keep file line endings consistent
	replacement := newString
	if hasCRLFContent {
		replacement = strings.ReplaceAll(strings.ReplaceAll(newString, "\r\n", "\n"), "\n", "\r\n")
	}

	normMatches := make([][2]int, 0)
	start := 0
	normOldLen := len(normOld)
	for {
		idx := strings.Index(normContent[start:], normOld)
		if idx == -1 {
			break
		}
		absStart := start + idx
		absEnd := absStart + normOldLen
		normMatches = append(normMatches, [2]int{absStart, absEnd})
		start = absEnd
	}

	matches := make([]replaceMatch, 0, len(normMatches))
	for _, nm := range normMatches {
		rawStart, rawEnd := mapNormRangeToRaw(content, nm[0], nm[1])
		matches = append(matches, replaceMatch{
			startOffset: rawStart,
			endOffset:   rawEnd,
			replacement: replacement,
			tier:        2,
		})
	}
	return matches
}

func mapNormRangeToRaw(rawContent string, normStart, normEnd int) (int, int) {
	rawStart := -1
	rawEnd := -1
	normIdx := 0
	rawIdx := 0
	rawLen := len(rawContent)

	for rawIdx < rawLen {
		if normIdx == normStart {
			rawStart = rawIdx
		}
		if normIdx == normEnd {
			rawEnd = rawIdx
			break
		}
		if rawIdx+1 < rawLen && rawContent[rawIdx] == '\r' && rawContent[rawIdx+1] == '\n' {
			rawIdx += 2
			normIdx++
		} else {
			rawIdx++
			normIdx++
		}
	}
	if rawStart == -1 && normIdx == normStart {
		rawStart = rawIdx
	}
	if rawEnd == -1 && normIdx == normEnd {
		rawEnd = rawIdx
	}
	return rawStart, rawEnd
}

// findWhitespaceMatches performs line-based whitespace-tolerant matching.
func findWhitespaceMatches(content, oldString, newString string, replaceAll bool, filePath string) ([]replaceMatch, error) {
	// Require non-whitespace content in oldString for tolerant matching
	if strings.TrimSpace(oldString) == "" {
		return nil, nil
	}

	normContent := strings.ReplaceAll(content, "\r\n", "\n")
	normOld := strings.ReplaceAll(oldString, "\r\n", "\n")

	contentLines := splitLinesWithOffsets(normContent)
	oldLines, oldTrailingNL := splitTolerantOldLines(normOld)

	if len(oldLines) > len(contentLines) {
		return nil, nil
	}

	// Sub-tier A: Trailing whitespace tolerance (lines identical after TrimRight space/tab)
	matchesA := scanLineMatches(contentLines, oldLines, func(s string) string {
		return strings.TrimRight(s, " \t\r")
	})
	if len(matchesA) > 0 {
		if len(matchesA) > 1 && !replaceAll {
			return nil, fmt.Errorf("oldString matched %d locations in %q (differing by trailing whitespace); use replaceAll for multiple matches", len(matchesA), filePath)
		}
		return buildTolerantReplacements(content, normContent, contentLines, oldLines, oldTrailingNL, matchesA, newString, 3), nil
	}

	// Sub-tier B: Leading/trailing whitespace and indentation tolerance (TrimSpace)
	matchesB := scanLineMatches(contentLines, oldLines, func(s string) string {
		return strings.TrimSpace(s)
	})
	if len(matchesB) > 0 {
		if len(matchesB) > 1 && !replaceAll {
			return nil, fmt.Errorf("oldString matched %d locations in %q (differing by indentation/whitespace); please provide more surrounding context", len(matchesB), filePath)
		}
		return buildTolerantReplacements(content, normContent, contentLines, oldLines, oldTrailingNL, matchesB, newString, 3), nil
	}

	return nil, nil
}

// splitTolerantOldLines splits a CRLF-normalized oldString into logical lines for
// whitespace-tolerant matching. A trailing newline marks the end of the matched
// block, not an additional empty line the file must contain, so it is reported
// separately. Splitting it into a trailing "" element required the file to end the
// matched block on an empty line, which made tolerant matching fail whenever the
// block was followed by more content or the file had no final newline.
func splitTolerantOldLines(normOld string) ([]string, bool) {
	lines := strings.Split(normOld, "\n")
	if len(lines) > 1 && lines[len(lines)-1] == "" {
		return lines[:len(lines)-1], true
	}
	return lines, false
}

type lineOffset struct {
	text        string
	startOffset int
	endOffset   int // including newline character if present
}

func splitLinesWithOffsets(text string) []lineOffset {
	if text == "" {
		return nil
	}
	var lines []lineOffset
	start := 0
	for {
		idx := strings.IndexByte(text[start:], '\n')
		if idx == -1 {
			lines = append(lines, lineOffset{
				text:        text[start:],
				startOffset: start,
				endOffset:   len(text),
			})
			break
		}
		end := start + idx + 1
		lines = append(lines, lineOffset{
			text:        text[start : end-1], // exclude \n in text
			startOffset: start,
			endOffset:   end,
		})
		start = end
		if start == len(text) {
			lines = append(lines, lineOffset{
				text:        "",
				startOffset: start,
				endOffset:   start,
			})
			break
		}
	}
	return lines
}

func scanLineMatches(contentLines []lineOffset, oldLines []string, normalize func(string) string) [][2]int {
	nOld := len(oldLines)
	if nOld == 0 || len(contentLines) < nOld {
		return nil
	}

	normOld := make([]string, nOld)
	for i, line := range oldLines {
		normOld[i] = normalize(line)
	}

	var matches [][2]int
	maxStart := len(contentLines) - nOld
	for i := 0; i <= maxStart; i++ {
		matched := true
		for j := 0; j < nOld; j++ {
			if normalize(contentLines[i+j].text) != normOld[j] {
				matched = false
				break
			}
		}
		if matched {
			matches = append(matches, [2]int{i, i + nOld - 1})
			if nOld > 1 {
				i += nOld - 1
			}
		}
	}
	return matches
}

func buildTolerantReplacements(rawContent, normContent string, contentLines []lineOffset, oldLines []string, oldHasTrailingNL bool, matchRanges [][2]int, newString string, tier int) []replaceMatch {
	hasCRLF := strings.Contains(rawContent, "\r\n")
	adjustedNew := newString
	if hasCRLF && !strings.Contains(newString, "\r\n") {
		adjustedNew = strings.ReplaceAll(strings.ReplaceAll(newString, "\r\n", "\n"), "\n", "\r\n")
	}

	matches := make([]replaceMatch, 0, len(matchRanges))
	for _, mr := range matchRanges {
		firstLine := contentLines[mr[0]]
		lastLine := contentLines[mr[1]]

		normStart := firstLine.startOffset
		normEnd := lastLine.endOffset

		// If oldString did not have a trailing newline, don't consume the file's trailing newline
		if !oldHasTrailingNL && strings.HasSuffix(normContent[normStart:normEnd], "\n") && !strings.HasSuffix(newString, "\n") {
			normEnd--
		}

		rawStart, rawEnd := mapNormRangeToRaw(rawContent, normStart, normEnd)

		// Indentation preservation: adapt leading indent and tab/space style across all lines
		matchedSlice := contentLines[mr[0] : mr[1]+1]
		replacement := adaptIndentation(matchedSlice, oldLines, adjustedNew)

		matches = append(matches, replaceMatch{
			startOffset: rawStart,
			endOffset:   rawEnd,
			replacement: replacement,
			tier:        tier,
		})
	}
	return matches
}

func adaptIndentation(matchedFileLines []lineOffset, oldLines []string, newString string) string {
	if len(matchedFileLines) == 0 || len(oldLines) == 0 || newString == "" {
		return newString
	}
	fileFirstLine := matchedFileLines[0].text
	oldFirstLine := oldLines[0]

	// 1. If first lines have base indentation differences, adapt base offset
	fileBase := leadingWhitespace(fileFirstLine)
	oldBase := leadingWhitespace(oldFirstLine)
	if fileBase != oldBase && oldBase != "" {
		newLines := strings.Split(newString, "\n")
		for i, l := range newLines {
			if strings.HasPrefix(l, oldBase) {
				newLines[i] = fileBase + l[len(oldBase):]
			}
		}
		newString = strings.Join(newLines, "\n")
	} else if fileBase != "" && oldBase == "" {
		newLines := strings.Split(newString, "\n")
		for i, l := range newLines {
			if strings.TrimSpace(l) != "" {
				newLines[i] = fileBase + l
			}
		}
		newString = strings.Join(newLines, "\n")
	}

	// 2. Detect tab vs space style between file and oldString
	fileUsesTabs := false
	fileUsesSpaces := false
	fileSpaceIndent := 0
	for _, fl := range matchedFileLines {
		ws := leadingWhitespace(fl.text)
		if strings.HasPrefix(ws, "\t") {
			fileUsesTabs = true
		} else if strings.HasPrefix(ws, "  ") {
			fileUsesSpaces = true
			if fileSpaceIndent == 0 || len(ws) < fileSpaceIndent {
				fileSpaceIndent = len(ws)
			}
		}
	}

	oldUsesTabs := false
	oldUsesSpaces := false
	oldSpaceIndent := 0
	for _, ol := range oldLines {
		ws := leadingWhitespace(ol)
		if strings.HasPrefix(ws, "\t") {
			oldUsesTabs = true
		} else if strings.HasPrefix(ws, "  ") {
			oldUsesSpaces = true
			if oldSpaceIndent == 0 || len(ws) < oldSpaceIndent {
				oldSpaceIndent = len(ws)
			}
		}
	}

	// Case A: File uses tabs, but oldString/newString uses spaces
	if fileUsesTabs && !fileUsesSpaces && oldUsesSpaces && !oldUsesTabs {
		indentSize := oldSpaceIndent
		if indentSize <= 0 {
			indentSize = 4
		}
		newLines := strings.Split(newString, "\n")
		for i, l := range newLines {
			ws := leadingWhitespace(l)
			if strings.HasPrefix(ws, " ") {
				tabCount := len(ws) / indentSize
				remSpaces := len(ws) % indentSize
				newLines[i] = strings.Repeat("\t", tabCount) + strings.Repeat(" ", remSpaces) + l[len(ws):]
			}
		}
		newString = strings.Join(newLines, "\n")
	}

	// Case B: File uses spaces, but oldString/newString uses tabs
	if fileUsesSpaces && !fileUsesTabs && oldUsesTabs && !oldUsesSpaces {
		indentSize := fileSpaceIndent
		if indentSize <= 0 {
			indentSize = 4
		}
		newLines := strings.Split(newString, "\n")
		for i, l := range newLines {
			ws := leadingWhitespace(l)
			if strings.HasPrefix(ws, "\t") {
				newLines[i] = strings.Repeat(" ", len(ws)*indentSize) + l[len(ws):]
			}
		}
		newString = strings.Join(newLines, "\n")
	}

	return newString
}

func leadingWhitespace(s string) string {
	for i, r := range s {
		if !unicode.IsSpace(r) {
			return s[:i]
		}
	}
	return s
}

// buildNearMatchDiagnostic finds the closest matching region in content to help the model recover.
func buildNearMatchDiagnostic(content, oldString, filePath string) string {
	normContent := strings.ReplaceAll(content, "\r\n", "\n")
	normOld := strings.ReplaceAll(oldString, "\r\n", "\n")
	contentLines := strings.Split(normContent, "\n")
	oldLines := strings.Split(normOld, "\n")

	if len(contentLines) == 0 || len(oldLines) == 0 {
		return fmt.Sprintf("oldString was not found in %q", filePath)
	}

	bestScore := 0.0
	bestLine := -1
	targetLen := len(oldLines)
	if targetLen > len(contentLines) {
		targetLen = len(contentLines)
	}

	// Scan sliding windows
	for i := 0; i <= len(contentLines)-targetLen; i++ {
		score := windowSimilarity(contentLines[i:i+targetLen], oldLines[:targetLen])
		if score > bestScore {
			bestScore = score
			bestLine = i
		}
	}

	if bestScore < 0.35 || bestLine == -1 {
		return fmt.Sprintf("oldString was not found in %q", filePath)
	}

	lineNum := bestLine + 1
	endLineNum := bestLine + targetLen
	differDetail := describeDifference(contentLines[bestLine:bestLine+targetLen], oldLines[:targetLen], lineNum)

	return fmt.Sprintf("oldString was not found in %q; nearest match at lines %d-%d (%.0f%% similar): %s",
		filePath, lineNum, endLineNum, bestScore*100, differDetail)
}

func windowSimilarity(a, b []string) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	n := min(len(a), len(b))
	matchingLines := 0
	for i := 0; i < n; i++ {
		trimA := strings.TrimSpace(a[i])
		trimB := strings.TrimSpace(b[i])
		if trimA == trimB {
			matchingLines++
		} else if trimA != "" && trimB != "" && (strings.Contains(trimA, trimB) || strings.Contains(trimB, trimA)) {
			matchingLines += 1
		}
	}
	return float64(matchingLines) / float64(max(len(a), len(b)))
}

func describeDifference(fileLines, oldLines []string, startLine int) string {
	n := min(len(fileLines), len(oldLines))
	for i := 0; i < n; i++ {
		if strings.TrimSpace(fileLines[i]) != strings.TrimSpace(oldLines[i]) {
			f := strings.TrimSpace(fileLines[i])
			o := strings.TrimSpace(oldLines[i])
			if len(f) > 40 {
				f = f[:37] + "..."
			}
			if len(o) > 40 {
				o = o[:37] + "..."
			}
			return fmt.Sprintf("line %d differed: file has %q vs oldString %q", startLine+i, f, o)
		}
	}
	return "differed by whitespace or formatting"
}

func applyReplacements(content string, matches []replaceMatch) string {
	if len(matches) == 0 {
		return content
	}
	var sb strings.Builder
	lastEnd := 0
	for _, m := range matches {
		if m.startOffset > lastEnd {
			sb.WriteString(content[lastEnd:m.startOffset])
		}
		sb.WriteString(m.replacement)
		lastEnd = m.endOffset
	}
	if lastEnd < len(content) {
		sb.WriteString(content[lastEnd:])
	}
	return sb.String()
}
