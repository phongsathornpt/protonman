package acp

import "encoding/json"

// SchemaArtifactVersion pins the ACP v1 schema artifact used as Protonman's
// conformance baseline. This is intentionally separate from ProtocolVersion:
// multiple schema artifact releases can describe the same wire protocol.
const SchemaArtifactVersion = "1.7.0"

// Meta is ACP's reserved extensibility bag. Values are kept as raw JSON so
// Protonman can preserve extension metadata without making assumptions about
// keys owned by clients, agents, or future ACP revisions.
type Meta map[string]json.RawMessage

// MetaCarrier can be embedded in protocol DTOs that expose ACP's reserved
// `_meta` property. New wire types should use this rather than vendor-specific
// top-level fields whenever the ACP extensibility model is sufficient.
type MetaCarrier struct {
	Meta Meta `json:"_meta,omitempty"`
}

func cloneMeta(meta Meta) Meta {
	if len(meta) == 0 {
		return nil
	}
	out := make(Meta, len(meta))
	for key, value := range meta {
		out[key] = append(json.RawMessage(nil), value...)
	}
	return out
}

// StableFeature describes one stable ACP surface tracked by the conformance
// suite. Supported means Protonman implements the feature; Advertised means it
// is exposed through negotiated capabilities where ACP defines one.
type StableFeature struct {
	Name       string
	Supported  bool
	Advertised bool
}

// StableV1Coverage is the single source of truth for the staged ACP v1
// conformance migration. False entries are deliberate implementation gaps, not
// claims of support. Tests and capability serialization consume this list rather
// than duplicating capability assumptions.
var StableV1Coverage = []StableFeature{
	{Name: "initialize", Supported: true, Advertised: true},
	{Name: "session/new", Supported: true, Advertised: true},
	{Name: "session/prompt", Supported: true, Advertised: true},
	{Name: "session/cancel", Supported: true, Advertised: true},
	{Name: "session/load", Supported: true, Advertised: true},
	{Name: "session/resume", Supported: true, Advertised: true},
	{Name: "session/list", Supported: true, Advertised: true},
	{Name: "session/delete", Supported: true, Advertised: true},
	{Name: "session/close", Supported: true, Advertised: true},
	{Name: "session/additional_directories", Supported: true, Advertised: true},
	{Name: "session/configuration", Supported: false, Advertised: false},
	{Name: "session/usage", Supported: false, Advertised: false},
	{Name: "session/info_update", Supported: false, Advertised: false},
	{Name: "elicitation", Supported: false, Advertised: false},
	{Name: "authentication", Supported: false, Advertised: false},
	{Name: "terminal_authentication", Supported: false, Advertised: false},
	{Name: "request_cancellation", Supported: false, Advertised: false},
}

func stableFeature(name string) (StableFeature, bool) {
	for _, feature := range StableV1Coverage {
		if feature.Name == name {
			return feature, true
		}
	}
	return StableFeature{}, false
}
