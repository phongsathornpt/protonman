package runtime

import (
	"context"
	"errors"
	"fmt"
	"sync"

	tea "charm.land/bubbletea/v2"

	"github.com/phongsathornpt/protonman/internal/core/permission"
)

type permissionBridge struct {
	requests chan permissionRequest
	done     chan struct{}
	once     sync.Once
}

type permissionRequest struct {
	request  permission.Request
	response chan permissionResponse
}

type permissionResponse struct {
	resolution permission.Resolution
	err        error
}

func newPermissionBridge() *permissionBridge {
	return &permissionBridge{
		requests: make(chan permissionRequest),
		done:     make(chan struct{}),
	}
}

func (b *permissionBridge) Prompt(ctx context.Context, request permission.Request) (permission.Resolution, error) {
	response := make(chan permissionResponse, 1)
	pending := permissionRequest{request: request, response: response}
	select {
	case b.requests <- pending:
	case <-ctx.Done():
		return permission.Resolution{}, fmt.Errorf("permission prompt canceled: %w", ctx.Err())
	case <-b.done:
		return permission.Resolution{}, errors.New("permission prompt closed")
	}
	select {
	case result := <-response:
		return result.resolution, result.err
	case <-ctx.Done():
		return permission.Resolution{}, fmt.Errorf("permission prompt canceled: %w", ctx.Err())
	case <-b.done:
		return permission.Resolution{}, errors.New("permission prompt closed")
	}
}

func (b *permissionBridge) Next() tea.Cmd {
	return func() tea.Msg {
		select {
		case request := <-b.requests:
			return permissionRequestMsg{request: request}
		case <-b.done:
			return permissionBridgeClosedMsg{}
		}
	}
}

func (b *permissionBridge) Close() { b.once.Do(func() { close(b.done) }) }
