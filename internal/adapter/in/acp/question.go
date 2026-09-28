package acp

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand/v2"
	"strconv"
	"sync"

	questiontool "github.com/phongsathornpt/protonman/internal/adapter/out/tool/question"
)

const requestQuestionMethod = "session/request_question"

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

type questionBroker struct {
	server  *Server
	waiters map[uint64]chan questionDecision
	mu      sync.Mutex
	closed  bool
}

func newQuestionBroker(server *Server) *questionBroker {
	return &questionBroker{
		server:  server,
		waiters: make(map[uint64]chan questionDecision),
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

	params := RequestQuestionParams{
		SessionID:   sessionID,
		Question:    req.Question,
		Options:     req.Options,
		Multiple:    req.Multiple,
		Recommended: req.Recommended,
		Questions:   req.Questions,
	}
	encoded, err := json.Marshal(params)
	if err != nil {
		return questiontool.Response{
			Status: questiontool.StatusDeclined,
			Answer: "encode question request",
		}, nil
	}

	id, waiter, err := b.register()
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
		Method:  requestQuestionMethod,
		Params:  encoded,
	}); err != nil {
		return questiontool.Response{
			Status: questiontool.StatusDeclined,
			Answer: "deliver question request",
		}, nil
	}

	select {
	case <-ctx.Done():
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

func (b *questionBroker) register() (uint64, chan questionDecision, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return 0, nil, errors.New("ACP question broker is closed")
	}
	id := b.nextQuestionID()
	waiter := make(chan questionDecision, 1)
	b.waiters[id] = waiter
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
	waiter <- decision
}

func (b *questionBroker) close() {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.closed = true
	waiters := b.waiters
	b.waiters = make(map[uint64]chan questionDecision)
	b.mu.Unlock()
	for _, waiter := range waiters {
		waiter <- questionDecision{
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
