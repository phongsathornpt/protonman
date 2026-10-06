//go:build desktop || desktop_gio

package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/phongsathornpt/protonman/internal/adapter/out/acpclient"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

const (
	requestQuestionMethod   = "session/request_question"
	methodElicitationCreate = "elicitation/create"
)

type questionParams struct {
	SessionID   string                           `json:"sessionId"`
	Question    string                           `json:"question,omitempty"`
	Options     []string                         `json:"options,omitempty"`
	Multiple    bool                             `json:"multiple,omitempty"`
	Recommended string                           `json:"recommended,omitempty"`
	Questions   []desktopstate.QuestionItemState `json:"questions,omitempty"`
}

type elicitationParams struct {
	Message         string `json:"message"`
	Mode            string `json:"mode"`
	SessionID       string `json:"sessionId"`
	RequestedSchema struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Properties  map[string]struct {
			Type        string   `json:"type"`
			Title       string   `json:"title"`
			Description string   `json:"description"`
			Default     any      `json:"default"`
			Enum        []string `json:"enum"`
			Items       *struct {
				Type string   `json:"type"`
				Enum []string `json:"enum"`
			} `json:"items"`
		} `json:"properties"`
		Required []string `json:"required"`
	} `json:"requestedSchema"`
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

	return c.promptQuestionsInternal(agentID, source, ctx, string(request.ID), params.SessionID, questions)
}

func (c *controller) handleElicitationRequestForAgent(agentID string, client *acpclient.Client, ctx context.Context, request acpclient.Request) (any, error) {
	return c.handleElicitationRequestFromAgent(agentID, client, ctx, request)
}

func (c *controller) handleElicitationRequestFromAgent(agentID string, source *acpclient.Client, ctx context.Context, request acpclient.Request) (any, error) {
	if request.Method != methodElicitationCreate {
		return nil, fmt.Errorf("%w: %s", acpclient.ErrMethodNotHandled, request.Method)
	}
	var params elicitationParams
	if err := json.Unmarshal(request.Params, &params); err != nil {
		return nil, fmt.Errorf("decode elicitation request: %w", err)
	}
	if strings.TrimSpace(params.SessionID) == "" {
		return nil, errors.New("elicitation request must include a session ID")
	}

	keys := make([]string, 0, len(params.RequestedSchema.Properties))
	for _, reqKey := range params.RequestedSchema.Required {
		if _, ok := params.RequestedSchema.Properties[reqKey]; ok {
			keys = append(keys, reqKey)
		}
	}
	var remaining []string
	for k := range params.RequestedSchema.Properties {
		found := false
		for _, reqKey := range keys {
			if reqKey == k {
				found = true
				break
			}
		}
		if !found {
			remaining = append(remaining, k)
		}
	}
	sort.Strings(remaining)
	keys = append(keys, remaining...)

	if len(keys) == 0 {
		return nil, errors.New("elicitation requestedSchema must define at least one property")
	}

	questions := make([]desktopstate.QuestionItemState, 0, len(keys))
	for _, key := range keys {
		prop := params.RequestedSchema.Properties[key]
		title := strings.TrimSpace(prop.Title)
		if title == "" {
			title = strings.TrimSpace(prop.Description)
		}
		if title == "" {
			title = key
		}
		isMulti := prop.Type == "array"
		opts := prop.Enum
		if isMulti && prop.Items != nil && len(prop.Items.Enum) > 0 {
			opts = prop.Items.Enum
		}
		recommended := ""
		if prop.Default != nil {
			switch d := prop.Default.(type) {
			case string:
				recommended = d
			case []any:
				if len(d) > 0 {
					recommended = fmt.Sprint(d[0])
				}
			}
		}
		questions = append(questions, desktopstate.QuestionItemState{
			Question:    title,
			Options:     opts,
			Multiple:    isMulti,
			Recommended: recommended,
		})
	}

	resp, err := c.promptQuestionsInternal(agentID, source, ctx, string(request.ID), params.SessionID, questions)
	if err != nil {
		return nil, err
	}
	if resp.Status != "answered" {
		return map[string]any{"action": "decline"}, nil
	}

	content := make(map[string]any, len(keys))
	if len(keys) == 1 {
		key := keys[0]
		if questions[0].Multiple {
			if len(resp.SelectedOptions) > 0 {
				content[key] = resp.SelectedOptions
			} else if resp.Answer != "" {
				content[key] = []string{resp.Answer}
			} else {
				content[key] = []string{}
			}
		} else {
			content[key] = resp.Answer
		}
	} else {
		for i, key := range keys {
			q := questions[i]
			var ans *desktopstate.QuestionAnswerItem
			for j := range resp.Answers {
				if resp.Answers[j].Question == q.Question || j == i {
					ans = &resp.Answers[j]
					break
				}
			}
			if q.Multiple {
				if ans != nil && len(ans.SelectedOptions) > 0 {
					content[key] = ans.SelectedOptions
				} else if ans != nil && ans.Answer != "" {
					content[key] = []string{ans.Answer}
				} else {
					content[key] = []string{}
				}
			} else {
				if ans != nil {
					content[key] = ans.Answer
				} else {
					content[key] = ""
				}
			}
		}
	}

	return map[string]any{
		"action":  "accept",
		"content": content,
	}, nil
}

func (c *controller) promptQuestionsInternal(agentID string, source *acpclient.Client, ctx context.Context, requestID, sessionID string, questions []desktopstate.QuestionItemState) (desktopstate.QuestionResponse, error) {
	if strings.TrimSpace(requestID) == "" {
		return desktopstate.QuestionResponse{}, errors.New("question request must include an ID")
	}

	item := desktopstate.QuestionRequest{
		RequestID: requestID,
		SessionID: sessionID,
		Questions: questions,
	}

	waiter := make(chan desktopstate.QuestionResponse, 1)
	c.mu.Lock()
	if source != nil {
		currentAgentID := c.agentIDForClientLocked(source)
		if currentAgentID == "" || agentID != "" && currentAgentID != agentID {
			c.mu.Unlock()
			return desktopstate.QuestionResponse{}, errors.New("ACP client changed before question request")
		}
		agentID = currentAgentID
	}
	item.AgentID = agentID
	item.RequestID = SessionRefStorageKey(desktopstate.SessionRef{AgentID: agentID, SessionID: item.RequestID})
	session, sessionExists := SessionByID(c.state, sessionID, agentID)
	if !sessionExists {
		c.mu.Unlock()
		return desktopstate.QuestionResponse{}, errors.New("question request references an unknown session")
	}
	if agentID != "" && session.AgentID != agentID {
		c.mu.Unlock()
		return desktopstate.QuestionResponse{}, errors.New("question request agent does not own the session")
	}
	if c.questionWait == nil {
		c.questionWait = make(map[string]chan desktopstate.QuestionResponse)
	}
	if _, exists := c.questionWait[item.RequestID]; exists {
		c.mu.Unlock()
		return desktopstate.QuestionResponse{}, errors.New("duplicate question request ID")
	}
	c.questionWait[item.RequestID] = waiter
	desktopstate.Apply(&c.state, desktopstate.Event{
		Kind:      desktopstate.EventQuestionRequested,
		AgentID:   agentID,
		SessionID: sessionID,
		Question:  item,
	})
	statusAgentID := agentID
	if statusAgentID == "" {
		statusAgentID = c.activeAgentID
	}
	c.statuses[statusAgentID] = "Question · " + SessionTitle(c.state, sessionID)
	c.revision++
	c.mu.Unlock()
	c.notify()

	select {
	case <-ctx.Done():
		c.finishQuestion(item.RequestID, sessionID, waiter, desktopstate.QuestionResponse{Status: "declined", Answer: "cancelled"}, item)
		return desktopstate.QuestionResponse{Status: "declined", Answer: "cancelled"}, nil
	case resp := <-waiter:
		c.finishQuestion(item.RequestID, sessionID, waiter, resp, item)
		return resp, nil
	}
}

func (c *controller) ResolveQuestion(requestID string, response desktopstate.QuestionResponse) {
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
	if session, ok := SessionByID(c.state, sessionID, question.AgentID); ok {
		agentID = session.AgentID
	}
	if agentID == "" {
		agentID = c.activeAgentID
	}
	if c.statuses[agentID] == "Question · "+SessionTitle(c.state, sessionID) {
		c.statuses[agentID] = "Connected"
	}
	c.revision++
	c.mu.Unlock()
	c.notify()
}
