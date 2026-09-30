//go:build desktop || desktop_gio

package gioui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/phongsathornpt/protonman/internal/adapter/out/acpclient"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

const requestQuestionMethod = "session/request_question"

type questionParams struct {
	SessionID   string                           `json:"sessionId"`
	Question    string                           `json:"question,omitempty"`
	Options     []string                         `json:"options,omitempty"`
	Multiple    bool                             `json:"multiple,omitempty"`
	Recommended string                           `json:"recommended,omitempty"`
	Questions   []desktopstate.QuestionItemState `json:"questions,omitempty"`
}

func (c *controller) handleQuestionRequestForAgent(agentID string, client *acpclient.Client, ctx context.Context, request acpclient.Request) (any, error) {
	return c.handleQuestionRequestFromAgent(agentID, client, ctx, request)
}

func (c *controller) handleQuestionRequestFromAgent(agentID string, source *acpclient.Client, ctx context.Context, request acpclient.Request) (any, error) {
	if request.Method != requestQuestionMethod {
		return nil, fmt.Errorf("%w: %s", acpclient.ErrMethodNotHandled, request.Method)
	}
	var params questionParams
	if err := json.Unmarshal(request.Params, &params); err != nil {
		return nil, fmt.Errorf("decode question request: %w", err)
	}
	if strings.TrimSpace(params.SessionID) == "" {
		return nil, errors.New("question request must include a session ID")
	}

	questions := params.Questions
	if len(questions) == 0 && strings.TrimSpace(params.Question) != "" {
		questions = []desktopstate.QuestionItemState{
			{
				Question:    params.Question,
				Options:     params.Options,
				Multiple:    params.Multiple,
				Recommended: params.Recommended,
			},
		}
	}
	if len(questions) == 0 {
		return nil, errors.New("question request must contain at least one question")
	}

	item := desktopstate.QuestionRequest{
		RequestID: string(request.ID),
		SessionID: params.SessionID,
		Questions: questions,
	}
	if item.RequestID == "" {
		return nil, errors.New("question request must include an ID")
	}

	waiter := make(chan desktopstate.QuestionResponse, 1)
	c.mu.Lock()
	if source != nil {
		currentAgentID := c.agentIDForClientLocked(source)
		if currentAgentID == "" || agentID != "" && currentAgentID != agentID {
			c.mu.Unlock()
			return nil, errors.New("ACP client changed before question request")
		}
		agentID = currentAgentID
	}
	item.AgentID = agentID
	item.RequestID = sessionRefStorageKey(desktopstate.SessionRef{AgentID: agentID, SessionID: item.RequestID})
	session, sessionExists := desktopSessionByID(c.state, params.SessionID, agentID)
	if !sessionExists {
		c.mu.Unlock()
		return nil, errors.New("question request references an unknown session")
	}
	if agentID != "" && session.AgentID != agentID {
		c.mu.Unlock()
		return nil, errors.New("question request agent does not own the session")
	}
	if c.questionWait == nil {
		c.questionWait = make(map[string]chan desktopstate.QuestionResponse)
	}
	if _, exists := c.questionWait[item.RequestID]; exists {
		c.mu.Unlock()
		return nil, errors.New("duplicate question request ID")
	}
	c.questionWait[item.RequestID] = waiter
	desktopstate.Apply(&c.state, desktopstate.Event{
		Kind:      desktopstate.EventQuestionRequested,
		AgentID:   agentID,
		SessionID: params.SessionID,
		Question:  item,
	})
	statusAgentID := agentID
	if statusAgentID == "" {
		statusAgentID = c.activeAgentID
	}
	c.statuses[statusAgentID] = "Question · " + sessionTitle(c.state, params.SessionID)
	c.revision++
	c.mu.Unlock()
	c.notify()

	select {
	case <-ctx.Done():
		c.finishQuestion(item.RequestID, params.SessionID, waiter, desktopstate.QuestionResponse{Status: "declined", Answer: "cancelled"}, item)
		return desktopstate.QuestionResponse{Status: "declined", Answer: "cancelled"}, nil
	case resp := <-waiter:
		c.finishQuestion(item.RequestID, params.SessionID, waiter, resp, item)
		return resp, nil
	}
}

func (c *controller) resolveQuestion(requestID string, response desktopstate.QuestionResponse) {
	c.mu.Lock()
	waiter := c.questionWait[requestID]
	if waiter == nil {
		c.mu.Unlock()
		return
	}
	c.mu.Unlock()
	select {
	case waiter <- response:
	default:
	}
}

func (c *controller) finishQuestion(requestID, sessionID string, waiter chan desktopstate.QuestionResponse, resp desktopstate.QuestionResponse, question desktopstate.QuestionRequest) {
	c.mu.Lock()
	if c.questionWait[requestID] != waiter {
		c.mu.Unlock()
		return
	}
	delete(c.questionWait, requestID)
	desktopstate.Apply(&c.state, desktopstate.Event{
		Kind:      desktopstate.EventQuestionResolved,
		AgentID:   question.AgentID,
		SessionID: sessionID,
		RequestID: requestID,
	})

	var answerSummary string
	if resp.Status == "answered" && strings.TrimSpace(resp.Answer) != "" {
		answerSummary = "Answered: " + resp.Answer
	} else {
		answerSummary = "Declined question"
	}
	qTitle := "Question"
	if len(question.Questions) > 0 && question.Questions[0].Question != "" {
		qTitle = question.Questions[0].Question
	}
	auditItem := desktopstate.TimelineItem{
		ID:     fmt.Sprintf("question-audit-%s", requestID),
		Kind:   desktopstate.TimelinePermission,
		Title:  answerSummary,
		Text:   qTitle,
		Status: resp.Status,
	}
	desktopstate.Apply(&c.state, desktopstate.Event{
		Kind:      desktopstate.EventTimelineAppended,
		AgentID:   question.AgentID,
		SessionID: sessionID,
		Item:      auditItem,
	})

	agentID := ""
	if session, ok := desktopSessionByID(c.state, sessionID, question.AgentID); ok {
		agentID = session.AgentID
	}
	if agentID == "" {
		agentID = c.activeAgentID
	}
	if c.statuses[agentID] == "Question · "+sessionTitle(c.state, sessionID) {
		c.statuses[agentID] = "Connected"
	}
	c.revision++
	c.mu.Unlock()
	c.notify()
}
