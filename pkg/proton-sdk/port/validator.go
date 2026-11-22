package port

import (
	"encoding/json"
)

// SchemaValidator defines the contract for JSON schema validation.
type SchemaValidator interface {
	Validate(payload json.RawMessage) error
}
