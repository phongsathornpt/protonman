package questiontool

import (
	"context"
	"strings"
)

// Status represents whether the user responded or declined.
type Status string

const (
	// StatusAnswered indicates the user provided an answer.
	StatusAnswered Status = "answered"
	// StatusDeclined indicates the prompt was dismissed or declined.
	StatusDeclined Status = "declined"
)

// QuestionItem defines a single question in an interview round or question batch.
type QuestionItem struct {
	Question    string   `json:"question"`
	Options     []string `json:"options,omitempty"`
	Multiple    bool     `json:"multiple,omitempty"`
	Recommended string   `json:"recommended,omitempty"`
}

// Request defines the input parameters for asking a question or a batch of questions.
type Request struct {
	Question    string         `json:"question,omitempty"`
	Options     []string       `json:"options,omitempty"`
	Multiple    bool           `json:"multiple,omitempty"`
	Recommended string         `json:"recommended,omitempty"`
	Questions   []QuestionItem `json:"questions,omitempty"`
}

// NormalizedItems returns a slice of QuestionItem regardless of whether single question
// or questions array was supplied.
func (r Request) NormalizedItems() []QuestionItem {
	if len(r.Questions) > 0 {
		return r.Questions
	}
	if strings.TrimSpace(r.Question) != "" {
		return []QuestionItem{
			{
				Question:    r.Question,
				Options:     r.Options,
				Multiple:    r.Multiple,
				Recommended: r.Recommended,
			},
		}
	}
	return nil
}

// AnswerItem represents the answer to a single question in a multi-question round.
type AnswerItem struct {
	Question        string   `json:"question"`
	Answer          string   `json:"answer"`
	SelectedOptions []string `json:"selectedOptions,omitempty"`
}

// Response holds the outcome of asking the user a question or batch of questions.
type Response struct {
	Status          Status       `json:"status"`
	Answer          string       `json:"answer"`
	SelectedOptions []string     `json:"selectedOptions,omitempty"`
	Answers         []AnswerItem `json:"answers,omitempty"`
}

// Prompter delivers an interactive question request to the UI and waits for the response.
type Prompter interface {
	PromptQuestion(ctx context.Context, req Request) (Response, error)
}
