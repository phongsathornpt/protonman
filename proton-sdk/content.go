package protonsdk

type ContentPartType string

const (
	ContentPartText  ContentPartType = "text"
	ContentPartImage ContentPartType = "image"
)

type ContentPart struct {
	Type     ContentPartType `json:"type"`
	Text     string          `json:"text,omitempty"`
	MIMEType string          `json:"mime_type,omitempty"`
	Data     string          `json:"data,omitempty"`
}
