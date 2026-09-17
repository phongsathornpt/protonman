package model

import (
	"context"
	"strings"

	"github.com/phongsathornpt/protonman/proton-sdk/domain"
	"github.com/phongsathornpt/protonman/proton-sdk/port"
	"github.com/phongsathornpt/protonman/proton-sdk/usecase"
)

type requestMetadataModel struct {
	base            port.LanguageModel
	sessionID       string
	projectID       string
	parentSessionID string
}

func withRequestMetadata(base port.LanguageModel, sessionID, projectID, parentSessionID string) port.LanguageModel {
	if base == nil {
		return nil
	}
	return &requestMetadataModel{
		base:            base,
		sessionID:       strings.TrimSpace(sessionID),
		projectID:       strings.TrimSpace(projectID),
		parentSessionID: strings.TrimSpace(parentSessionID),
	}
}

func (m *requestMetadataModel) Provider() string                       { return m.base.Provider() }
func (m *requestMetadataModel) ModelID() string                        { return m.base.ModelID() }
func (m *requestMetadataModel) Capabilities() domain.ModelCapabilities { return m.base.Capabilities() }
func (m *requestMetadataModel) ContextWindow() int                     { return usecase.ModelContextWindow(m.base) }
func (m *requestMetadataModel) TokenLimits() domain.TokenLimits        { return usecase.ModelTokenLimits(m.base) }
func (m *requestMetadataModel) Stream(ctx context.Context, request domain.Request) (port.Stream, error) {
	if m.sessionID != "" {
		request.Metadata.SessionID = m.sessionID
	}
	if m.projectID != "" {
		request.Metadata.ProjectID = m.projectID
	}
	if m.parentSessionID != "" {
		request.Metadata.ParentSessionID = m.parentSessionID
	}
	return m.base.Stream(ctx, request)
}
