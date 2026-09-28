package providerutil

import (
	"encoding/json"
	"fmt"
)

func MarshalWithOptions(base any, raw json.RawMessage, protected ...string) ([]byte, error) {
	encoded, err := json.Marshal(base)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return encoded, nil
	}
	var target map[string]any
	if err := json.Unmarshal(encoded, &target); err != nil {
		return nil, err
	}
	var extra map[string]any
	if err := json.Unmarshal(raw, &extra); err != nil {
		return nil, fmt.Errorf("provider options must be a JSON object: %w", err)
	}
	blocked := make(map[string]struct{}, len(protected))
	for _, key := range protected {
		blocked[key] = struct{}{}
	}
	for key, value := range extra {
		if _, exists := blocked[key]; exists {
			return nil, fmt.Errorf("provider option %q cannot override canonical request field", key)
		}
		target[key] = value
	}
	return json.Marshal(target)
}
