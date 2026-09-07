// Package glob implements high-performance wildcard matching (* and ?).
package glob

import (
	"strings"
	"unicode/utf8"
)

// Match reports whether value matches the shell pattern with * and ? wildcards.
func Match(pattern string, value string) bool {
	if pattern == "*" {
		return true
	}
	if pattern == value {
		return true
	}
	if pattern == "" {
		return value == ""
	}

	// Fast paths for common single-wildcard patterns
	if strings.HasPrefix(pattern, "*") && !strings.ContainsAny(pattern[1:], "*?") {
		return strings.HasSuffix(value, pattern[1:])
	}
	if strings.HasSuffix(pattern, "*") && !strings.ContainsAny(pattern[:len(pattern)-1], "*?") {
		return strings.HasPrefix(value, pattern[:len(pattern)-1])
	}
	if !strings.ContainsAny(pattern, "*?") {
		return pattern == value
	}

	// Byte-level backtracking matcher for ASCII
	if isASCII(pattern) && isASCII(value) {
		pIdx, vIdx := 0, 0
		lastWildcardIdx := -1
		backtrackIdx := -1
		pLen, vLen := len(pattern), len(value)

		for vIdx < vLen {
			if pIdx < pLen && (pattern[pIdx] == '?' || pattern[pIdx] == value[vIdx]) {
				pIdx++
				vIdx++
			} else if pIdx < pLen && pattern[pIdx] == '*' {
				lastWildcardIdx = pIdx
				backtrackIdx = vIdx
				pIdx++
			} else if lastWildcardIdx != -1 {
				pIdx = lastWildcardIdx + 1
				backtrackIdx++
				vIdx = backtrackIdx
			} else {
				return false
			}
		}

		for pIdx < pLen && pattern[pIdx] == '*' {
			pIdx++
		}

		return pIdx == pLen
	}

	return matchRunes(pattern, value)
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

func matchRunes(pattern, value string) bool {
	var pBuf [64]rune
	var vBuf [128]rune
	var pRunes []rune
	var vRunes []rune

	pLen := utf8.RuneCountInString(pattern)
	if pLen <= len(pBuf) {
		pRunes = pBuf[:0]
		for _, r := range pattern {
			pRunes = append(pRunes, r)
		}
	} else {
		pRunes = []rune(pattern)
	}

	vLen := utf8.RuneCountInString(value)
	if vLen <= len(vBuf) {
		vRunes = vBuf[:0]
		for _, r := range value {
			vRunes = append(vRunes, r)
		}
	} else {
		vRunes = []rune(value)
	}

	pIdx, vIdx := 0, 0
	lastWildcardIdx := -1
	backtrackIdx := -1

	for vIdx < len(vRunes) {
		if pIdx < len(pRunes) && (pRunes[pIdx] == '?' || pRunes[pIdx] == vRunes[vIdx]) {
			pIdx++
			vIdx++
		} else if pIdx < len(pRunes) && pRunes[pIdx] == '*' {
			lastWildcardIdx = pIdx
			backtrackIdx = vIdx
			pIdx++
		} else if lastWildcardIdx != -1 {
			pIdx = lastWildcardIdx + 1
			backtrackIdx++
			vIdx = backtrackIdx
		} else {
			return false
		}
	}

	for pIdx < len(pRunes) && pRunes[pIdx] == '*' {
		pIdx++
	}

	return pIdx == len(pRunes)
}
