package readfile

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const maxStructuredUniqueValues = 1024

type fieldProfileSummary struct {
	Type         string `json:"type"`
	NonNull      int    `json:"non_null"`
	Unique       int    `json:"unique"`
	UniqueCapped bool   `json:"unique_capped,omitempty"`
}

type fieldProfile struct {
	types        map[string]struct{}
	nonNull      int
	sawNull      bool
	unique       map[string]struct{}
	uniqueCapped bool
}

func (p *fieldProfile) addJSON(value any) {
	kind, uniqueKey := jsonFieldValue(value)
	p.add(kind, uniqueKey)
}

func (p *fieldProfile) addDelimited(value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		p.add("null", "")
		return
	}
	if _, err := strconv.ParseBool(value); err == nil {
		p.add("boolean", "b:"+strings.ToLower(value))
		return
	}
	if number, err := strconv.ParseFloat(value, 64); err == nil {
		p.add("number", "n:"+strconv.FormatFloat(number, 'g', -1, 64))
		return
	}
	p.add("string", "s:"+value)
}

func (p *fieldProfile) add(kind, uniqueKey string) {
	if kind == "null" {
		p.sawNull = true
		return
	}
	p.nonNull++
	if p.types == nil {
		p.types = make(map[string]struct{}, 2)
	}
	p.types[kind] = struct{}{}
	if uniqueKey == "" || p.uniqueCapped {
		return
	}
	if p.unique == nil {
		p.unique = make(map[string]struct{}, 8)
	}
	if _, exists := p.unique[uniqueKey]; exists {
		return
	}
	if len(p.unique) >= maxStructuredUniqueValues {
		p.uniqueCapped = true
		return
	}
	p.unique[uniqueKey] = struct{}{}
}

func (p fieldProfile) summary() fieldProfileSummary {
	typeName := "unknown"
	if len(p.types) == 0 && p.sawNull {
		typeName = "null"
	} else if len(p.types) == 1 {
		for kind := range p.types {
			typeName = kind
		}
	} else if len(p.types) > 1 {
		typeName = "mixed"
	}
	return fieldProfileSummary{
		Type: typeName, NonNull: p.nonNull,
		Unique: len(p.unique), UniqueCapped: p.uniqueCapped,
	}
}

func jsonFieldValue(value any) (string, string) {
	switch typed := value.(type) {
	case nil:
		return "null", ""
	case bool:
		return "boolean", "b:" + strconv.FormatBool(typed)
	case json.Number:
		if number, err := typed.Float64(); err == nil {
			return "number", "n:" + strconv.FormatFloat(number, 'g', -1, 64)
		}
		return "number", "n:" + typed.String()
	case float64:
		return "number", "n:" + strconv.FormatFloat(typed, 'g', -1, 64)
	case float32:
		return "number", "n:" + strconv.FormatFloat(float64(typed), 'g', -1, 64)
	case int:
		return "number", "n:" + strconv.Itoa(typed)
	case int64:
		return "number", "n:" + strconv.FormatInt(typed, 10)
	case string:
		return "string", "s:" + typed
	case []any:
		return "array", ""
	case map[string]any:
		return "object", ""
	default:
		return "unknown", fmt.Sprintf("u:%T:%v", value, value)
	}
}

func profilesForObjects(rows []any, fields []string) map[string]fieldProfileSummary {
	profiles := make(map[string]*fieldProfile, len(fields))
	for _, row := range rows {
		object, ok := row.(map[string]any)
		if !ok {
			continue
		}
		for _, field := range fields {
			profile := profiles[field]
			if profile == nil {
				profile = &fieldProfile{}
				profiles[field] = profile
			}
			value, exists := object[field]
			if !exists {
				value = nil
			}
			profile.addJSON(value)
		}
	}
	return finalizeFieldProfiles(profiles, fields)
}

func profilesForObject(object map[string]any, fields []string) map[string]fieldProfileSummary {
	return profilesForObjects([]any{object}, fields)
}

func finalizeFieldProfiles(profiles map[string]*fieldProfile, fields []string) map[string]fieldProfileSummary {
	if len(fields) == 0 {
		return nil
	}
	ordered := append([]string(nil), fields...)
	sort.Strings(ordered)
	result := make(map[string]fieldProfileSummary, len(ordered))
	for _, field := range ordered {
		profile := profiles[field]
		if profile == nil {
			profile = &fieldProfile{sawNull: true}
		}
		result[field] = profile.summary()
	}
	return result
}
