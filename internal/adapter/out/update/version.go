package update

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var tagPattern = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z][0-9A-Za-z.-]*)?$`)

func NormalizeTag(version string) string {
	trimmed := strings.TrimSpace(version)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "v") {
		return trimmed
	}
	return "v" + trimmed
}

func ValidateTag(version string) (string, error) {
	normalized := NormalizeTag(version)
	if !tagPattern.MatchString(normalized) {
		return "", fmt.Errorf("invalid release version: %q", strings.TrimSpace(version))
	}
	return normalized, nil
}

func Plain(version string) string {
	return strings.TrimPrefix(strings.TrimSpace(version), "v")
}

type parsedVersion struct {
	major int
	minor int
	patch int
	pre   string
}

func parsePlain(version string) parsedVersion {
	plain := Plain(version)
	core, pre, _ := strings.Cut(plain, "-")
	parts := strings.Split(core, ".")
	parsed := parsedVersion{pre: pre}
	if len(parts) == 3 {
		parsed.major, _ = strconv.Atoi(parts[0])
		parsed.minor, _ = strconv.Atoi(parts[1])
		patch, _, _ := strings.Cut(parts[2], "-")
		parsed.patch, _ = strconv.Atoi(patch)
	}
	return parsed
}

func Compare(a, b string) int {
	left, right := parsePlain(a), parsePlain(b)
	for _, pair := range [][2]int{
		{left.major, right.major},
		{left.minor, right.minor},
		{left.patch, right.patch},
	} {
		if pair[0] != pair[1] {
			if pair[0] < pair[1] {
				return -1
			}
			return 1
		}
	}
	switch {
	case left.pre == right.pre:
		return 0
	case left.pre == "":
		return 1
	case right.pre == "":
		return -1
	default:
		return compareIdentifiers(left.pre, right.pre)
	}
}

func compareIdentifiers(a, b string) int {
	left, right := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(left) && i < len(right); i++ {
		ln, lok := strconv.Atoi(left[i])
		rn, rok := strconv.Atoi(right[i])
		switch {
		case lok == nil && rok == nil:
			if ln != rn {
				if ln < rn {
					return -1
				}
				return 1
			}
		case lok == nil:
			return -1
		case rok == nil:
			return 1
		default:
			if left[i] != right[i] {
				if left[i] < right[i] {
					return -1
				}
				return 1
			}
		}
	}
	switch {
	case len(left) < len(right):
		return -1
	case len(left) > len(right):
		return 1
	default:
		return 0
	}
}
