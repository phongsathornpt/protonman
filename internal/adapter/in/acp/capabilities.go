package acp

import "encoding/json"

// MarshalJSON keeps negotiated ACP capabilities aligned with the conformance
// manifest. A stable optional capability is not emitted until its end-to-end
// implementation is marked advertised.
func (c SessionCapabilities) MarshalJSON() ([]byte, error) {
	type wire struct {
		Meta                  Meta      `json:"_meta,omitempty"`
		Resume                *struct{} `json:"resume,omitempty"`
		Delete                *struct{} `json:"delete,omitempty"`
		Close                 *struct{} `json:"close,omitempty"`
		AdditionalDirectories *struct{} `json:"additionalDirectories,omitempty"`
	}

	out := wire{
		Meta:   c.Meta,
		Resume: c.Resume,
		Delete: c.Delete,
		Close:  c.Close,
	}
	if feature, ok := stableFeature("session/additional_directories"); ok && feature.Advertised {
		out.AdditionalDirectories = c.AdditionalDirectories
	}
	return json.Marshal(out)
}
