// Package agentidentity defines the stable client identity sent to model gateways.
package agentidentity

// Type identifies the client/runtime issuing a model request.
type Type string

const (
	TypeProtonman Type = "protonman"
	TypeUnknown   Type = "unknown"
)

func (t Type) String() string { return string(t) }
