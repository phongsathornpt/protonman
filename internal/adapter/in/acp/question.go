package acp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"
	"sync"

	questiontool "github.com/phongsathornpt/protonman/internal/adapter/out/tool/question"
)

const (
	requestQuestionMethod   = "session/request_question"
	methodElicitationCreate = "elicitation/create"
)

type questionDecision struct {
	response questiontool.Response
	err      error
}

// RequestQuestionParams defines the parameters sent from ACP server to client.
type RequestQuestionParams struct {
	SessionID   string                      `json:"sessionId"`
	Question    string                      `json:"question,omitempty"`
	Options     []string                    `json:"options,omitempty"`
	Multiple    bool                        `json:"multiple,omitempty"`
	Recommended string                      `json:"recommended,omitempty"`
	Questions   []questiontool.QuestionItem `json:"questions,omitempty"`
}

// RequestQuestionResult defines the result expected from the client.
type RequestQuestionResult struct {
	Status          questiontool.Status       `json:"status"`
	Answer          string                    `json:"answer,omitempty"`
	SelectedOptions []string                  `json:"selectedOptions,omitempty"`
	Answers         []questiontool.AnswerItem `json:"answers,omitempty"`
}

type questionWaiter struct {
	ch  chan questionDecision
	req questiontool.Request
}

type questionBroker struct {
	server  *Server
	waiters map[uint64]questionWaiter
	mu      sync.Mutex
	closed  bool
}

func newQuestionBroker(server *Server) *questionBroker {
	return &questionBroker{
		server:  server,
		waiters: make(map[uint64]questionWaiter),
	}
}

type sessionQuestionPrompter struct {
	broker    *questionBroker
	sessionID string
}

func (p *sessionQuestionPrompter) PromptQuestion(ctx context.Context, req questiontool.Request) (questiontool.Response, error) {
	if p == nil || p.broker == nil {
		return questiontool.Response{
			Status: questiontool.StatusDeclined,
			Answer: "interactive questions unavailable",
		}, nil
	}
	return p.broker.request(ctx, p.sessionID, req)
}

func (b *questionBroker) prompter(sessionID string) questiontool.Prompter {
	return &sessionQuestionPrompter{
		broker:    b,
		sessionID: sessionID,
	}
}

func (b *questionBroker) request(ctx context.Context, sessionID string, req questiontool.Request) (questiontool.Response, error) {
	if err := ctx.Err(); err != nil {
		return questiontool.Response{
			Status: questiontool.StatusDeclined,
			Answer: "question request cancelled before it was sent",
		}, nil
	}
	if b == nil || b.server == nil {
		return questiontool.Response{
			Status: questiontool.StatusDeclined,
			Answer: "no ACP client is attached to this session",
		}, nil
	}

	method := requestQuestionMethod
	var encoded []byte
	var err error

	if b.server.supportsFormElicitation() {
		method = methodElicitationCreate
		elicitationReq := buildElicitationRequest(sessionID, req)
		encoded, err = json.Marshal(elicitationReq)
	} else {
		params := RequestQuestionParams{
			SessionID:   sessionID,
			Question:    req.Question,
			Options:     req.Options,
			Multiple:    req.Multiple,
			Recommended: req.Recommended,
			Questions:   req.Questions,
		}
		encoded, err = json.Marshal(params)
	}

	if err != nil {
		return questiontool.Response{
			Status: questiontool.StatusDeclined,
			Answer: "encode question request",
		}, nil
	}

	id, waiter, err := b.register(req)
	if err != nil {
		return questiontool.Response{
			Status: questiontool.StatusDeclined,
			Answer: "question prompting is unavailable",
		}, nil
	}
	defer b.unregister(id)

	b.server.mu.Lock()
	output := b.server.output
	b.server.mu.Unlock()
	if output == nil {
		return questiontool.Response{
			Status: questiontool.StatusDeclined,
			Answer: "ACP connection is closed",
		}, nil
	}

	if err := WriteJSON(output, &b.server.writeMu, RPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(strconv.FormatUint(id, 10)),
		Method:  method,
		Params:  encoded,
	}); err != nil {
		return questiontool.Response{
			Status: questiontool.StatusDeclined,
			Answer: "deliver question request",
		}, nil
	}

	select {
	case <-ctx.Done():
		b.server.cancelOutboundRequest(id)
		return questiontool.Response{
			Status: questiontool.StatusDeclined,
			Answer: "question request was cancelled",
		}, nil
	case decision := <-waiter:
		if decision.err != nil {
			return questiontool.Response{
				Status: questiontool.StatusDeclined,
				Answer: decision.err.Error(),
			}, nil
		}
		return decision.response, nil
	}
}

func buildElicitationRequest(sessionID string, req questiontool.Request) CreateElicitationRequest {
	items := req.NormalizedItems()
	message := req.Question
	if len(items) > 1 && strings.TrimSpace(message) == "" {
		message = "Please answer the following questions"
	}
	if strings.TrimSpace(message) == "" {
		message = "Input required"
	}

	properties := make(map[string]ElicitationPropertySchema, len(items))
	required := make([]string, 0, len(items))

	for i, item := range items {
		key := fmt.Sprintf("q%d", i+1)
		required = append(required, key)

		if item.Multiple {
			prop := ElicitationPropertySchema{
				Type:        "array",
				Title:       item.Question,
				Description: item.Question,
				Items: &MultiSelectItems{
					Type: "string",
					Enum: item.Options,
				},
			}
			if item.Recommended != "" {
				prop.Default = []string{item.Recommended}
			}
			properties[key] = prop
		} else {
			prop := ElicitationPropertySchema{
				Type:        "string",
				Title:       item.Question,
				Description: item.Question,
			}
			if len(item.Options) > 0 {
				prop.Enum = item.Options
			}
			if item.Recommended != "" {
				prop.Default = item.Recommended
			}
			properties[key] = prop
		}
	}

	return CreateElicitationRequest{
		Message:   message,
		Mode:      "form",
		SessionID: sessionID,
		RequestedSchema: ElicitationSchema{
			Type:       "object",
			Title:      message,
			Properties: properties,
			Required:   required,
		},
	}
}

func (b *questionBroker) register(req questiontool.Request) (uint64, chan questionDecision, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return 0, nil, errors.New("ACP question broker is closed")
	}
	id := b.nextQuestionID()
	waiter := make(chan questionDecision, 1)
	b.waiters[id] = questionWaiter{ch: waiter, req: req}
	return id, waiter, nil
}

func (b *questionBroker) unregister(id uint64) {
	b.mu.Lock()
	delete(b.waiters, id)
	b.mu.Unlock()
}

func (b *questionBroker) hasWaiter(id uint64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, ok := b.waiters[id]
	return ok
}

func (b *questionBroker) resolve(id uint64, decision questionDecision) {
	b.mu.Lock()
	waiter, ok := b.waiters[id]
	if ok {
		delete(b.waiters, id)
	}
	b.mu.Unlock()
	if !ok {
		return
	}
	waiter.ch <- decision
}

func (b *questionBroker) decodeOutcome(id uint64, encoded []byte) questionDecision {
	b.mu.Lock()
	waiter, ok := b.waiters[id]
	b.mu.Unlock()
	if !ok {
		return questionDecision{err: errors.New("no waiter for question request")}
	}
	req := waiter.req

	// Check if this is an elicitation response (has "action" field).
	var elicitationResp CreateElicitationResponse
	if err := json.Unmarshal(encoded, &elicitationResp); err == nil && elicitationResp.Action != "" {
		switch strings.ToLower(strings.TrimSpace(elicitationResp.Action)) {
		case "decline":
			return questionDecision{
				response: questiontool.Response{
					Status: questiontool.StatusDeclined,
					Answer: "user declined the elicitation",
				},
			}
		case "cancel":
			return questionDecision{
				response: questiontool.Response{
					Status: questiontool.StatusDeclined,
					Answer: "elicitation was cancelled",
				},
			}
		case "accept":
			return questionDecision{
				response: extractElicitationAnswers(req, elicitationResp.Content),
			}
		default:
			return questionDecision{
				err: fmt.Errorf("unknown elicitation action %q", elicitationResp.Action),
			}
		}
	}

	// Legacy RequestQuestionResult
	var result RequestQuestionResult
	if err := json.Unmarshal(encoded, &result); err != nil {
		return questionDecision{err: fmt.Errorf("decode question response: %w", err)}
	}
	status := result.Status
	if status == "" {
		status = questiontool.StatusAnswered
	}
	return questionDecision{
		response: questiontool.Response{
			Status:          status,
			Answer:          result.Answer,
			SelectedOptions: result.SelectedOptions,
			Answers:         result.Answers,
		},
	}
}

func extractElicitationAnswers(req questiontool.Request, content map[string]any) questiontool.Response {
	items := req.NormalizedItems()
	if len(items) == 0 {
		return questiontool.Response{Status: questiontool.StatusAnswered}
	}

	if len(items) == 1 {
		item := items[0]
		val, ok := content["q1"]
		if !ok {
			val, ok = content["question"]
		}
		if !ok {
			val, ok = content["answer"]
		}
		if !ok && len(content) == 1 {
			for _, v := range content {
				val = v
				ok = true
				break
			}
		}

		var answer string
		var selected []string

		switch v := val.(type) {
		case string:
			answer = strings.TrimSpace(v)
			if len(item.Options) > 0 {
				for _, opt := range item.Options {
					if opt == answer {
						selected = []string{answer}
						break
					}
				}
			}
		case []any:
			for _, elem := range v {
				selected = append(selected, fmt.Sprint(elem))
			}
			answer = strings.Join(selected, ", ")
		case []string:
			selected = v
			answer = strings.Join(selected, ", ")
		default:
			if v != nil {
				answer = fmt.Sprint(v)
			}
		}

		return questiontool.Response{
			Status:          questiontool.StatusAnswered,
			Answer:          answer,
			SelectedOptions: selected,
			Answers: []questiontool.AnswerItem{
				{Question: item.Question, Answer: answer, SelectedOptions: selected},
			},
		}
	}

	answers := make([]questiontool.AnswerItem, 0, len(items))
	for i, item := range items {
		key := fmt.Sprintf("q%d", i+1)
		val, ok := content[key]
		if !ok {
			val, ok = content[fmt.Sprintf("question_%d", i)]
		}
		if !ok {
			val = content[item.Question]
		}

		var ansStr string
		var selected []string

		switch v := val.(type) {
		case string:
			ansStr = strings.TrimSpace(v)
			if len(item.Options) > 0 {
				for _, opt := range item.Options {
					if opt == ansStr {
						selected = []string{ansStr}
						break
					}
				}
			}
		case []any:
			for _, elem := range v {
				selected = append(selected, fmt.Sprint(elem))
			}
			ansStr = strings.Join(selected, ", ")
		case []string:
			selected = v
			ansStr = strings.Join(selected, ", ")
		default:
			if v != nil {
				ansStr = fmt.Sprint(v)
			}
		}

		answers = append(answers, questiontool.AnswerItem{
			Question:        item.Question,
			Answer:          ansStr,
			SelectedOptions: selected,
		})
	}

	topAnswer := ""
	if len(answers) > 0 {
		topAnswer = answers[0].Answer
	}

	return questiontool.Response{
		Status:  questiontool.StatusAnswered,
		Answer:  topAnswer,
		Answers: answers,
	}
}

func (b *questionBroker) close() {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.closed = true
	waiters := b.waiters
	b.waiters = make(map[uint64]questionWaiter)
	b.mu.Unlock()
	for _, waiter := range waiters {
		waiter.ch <- questionDecision{
			err: errors.New("ACP connection closed before the question request was answered"),
		}
	}
}

func (b *questionBroker) nextQuestionID() uint64 {
	for {
		id := rand.Uint64()
		if id == 0 {
			continue
		}
		if _, exists := b.waiters[id]; !exists {
			return id
		}
	}
}
