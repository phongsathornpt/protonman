package mcp

import (
	"encoding/json"
	"fmt"
)

// Limits bounds untrusted MCP discovery and transport metadata.
type Limits struct {
	MaxServers             int
	MaxToolsPerServer      int
	MaxTotalTools          int
	MaxDescriptionBytes    int
	MaxSchemaBytes         int
	MaxSchemaDepth         int
	MaxMessageBytes        int
	MaxArgumentsBytes      int
	MaxTextOutputBytes     int
	MaxStructuredBytes     int
	MaxStderrBytes         int
	MaxConcurrentDiscovery int
}

func DefaultLimits() Limits {
	return Limits{
		MaxServers: 32, MaxToolsPerServer: 256, MaxTotalTools: 512,
		MaxDescriptionBytes: 16 * 1024, MaxSchemaBytes: 256 * 1024, MaxSchemaDepth: 64,
		MaxMessageBytes: 4 * 1024 * 1024, MaxArgumentsBytes: 1024 * 1024,
		MaxTextOutputBytes: 1024 * 1024, MaxStructuredBytes: 2 * 1024 * 1024, MaxStderrBytes: 256 * 1024,
		MaxConcurrentDiscovery: 8,
	}
}

func (l Limits) validate() error {
	if l.MaxServers <= 0 || l.MaxToolsPerServer <= 0 || l.MaxTotalTools <= 0 ||
		l.MaxDescriptionBytes <= 0 || l.MaxSchemaBytes <= 0 || l.MaxSchemaDepth <= 0 ||
		l.MaxMessageBytes <= 0 || l.MaxArgumentsBytes <= 0 || l.MaxTextOutputBytes <= 0 ||
		l.MaxStructuredBytes <= 0 || l.MaxStderrBytes <= 0 || l.MaxConcurrentDiscovery <= 0 {
		return fmt.Errorf("MCP limits must all be positive")
	}
	return nil
}

func validateManifestLimits(serverName string, manifest Tool, limits Limits) error {
	if len(manifest.Description) > limits.MaxDescriptionBytes {
		return fmt.Errorf("MCP tool %s.%s description is %d bytes; limit is %d", serverName, manifest.Name, len(manifest.Description), limits.MaxDescriptionBytes)
	}
	for label, schema := range map[string]map[string]any{"input": manifest.InputSchema, "output": manifest.OutputSchema} {
		if schema == nil {
			continue
		}
		data, err := json.Marshal(schema)
		if err != nil {
			return fmt.Errorf("MCP tool %s.%s %s schema is not JSON-compatible: %w", serverName, manifest.Name, label, err)
		}
		if len(data) > limits.MaxSchemaBytes {
			return fmt.Errorf("MCP tool %s.%s %s schema is %d bytes; limit is %d", serverName, manifest.Name, label, len(data), limits.MaxSchemaBytes)
		}
		if depth := jsonValueDepth(schema); depth > limits.MaxSchemaDepth {
			return fmt.Errorf("MCP tool %s.%s %s schema depth is %d; limit is %d", serverName, manifest.Name, label, depth, limits.MaxSchemaDepth)
		}
	}
	return nil
}

func jsonValueDepth(value any) int {
	switch typed := value.(type) {
	case map[string]any:
		max := 1
		for _, child := range typed {
			if depth := 1 + jsonValueDepth(child); depth > max {
				max = depth
			}
		}
		return max
	case []any:
		max := 1
		for _, child := range typed {
			if depth := 1 + jsonValueDepth(child); depth > max {
				max = depth
			}
		}
		return max
	default:
		return 1
	}
}
