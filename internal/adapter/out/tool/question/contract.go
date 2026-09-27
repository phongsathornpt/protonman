package questiontool

import "context"

// Status represents whether the user responded or declined.
type Status string

const (
	// StatusAnswered indicates the user provided an answer.
	StatusAnswered Status = "answered"
	// StatusDeclined indicates the prompt was dismissed or declined.
	StatusDeclined Status = "declined"
)

// Request defines the input parameters for asking a question.
type Request struct {
	Question string   `json:"question"`
	Options  []string `json:"options,omitempty"`
	Multiple bool     `json:"multiple,omitempty"`
}

// Response holds the outcome of asking the user a question.
type Response struct {
	Status          Status   `json:"status"`
	Answer          string   `json:"answer"`
	SelectedOptions []string `json:"selected_options,omitempty"`
}

// Prompter delivers an interactive question request to the UI and waits for the response.
type Prompter interface {
	PromptQuestion(ctx context.Context, req Request) (Response, error)
}
