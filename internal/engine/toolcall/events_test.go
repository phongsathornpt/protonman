package toolcall

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/tool"
)

func TestServiceEmitsRedactedLifecycleEvents(t *testing.T) {
	tests := []struct {
		name         string
		policy       permission.Config
		wantKinds    []EventKind
		wantDecision permission.Action
		wantCode     tool.ErrorCode
		wantCalls    int
	}{
		{
			name: "allowed",
			policy: permission.Config{
				Rules: []permission.Rule{{
					Action: permission.ActionAllow,
					Tool:   permission.ToolBash,
				}},
			},
			wantKinds:    []EventKind{EventCallStarted, EventPermissionResolved, EventCallCompleted},
			wantDecision: permission.ActionAllow,
			wantCalls:    1,
		},
		{
			name: "denied",
			policy: permission.Config{
				Rules: []permission.Rule{{
					Action:  permission.ActionDeny,
					Tool:    permission.ToolBash,
					Pattern: "printf *",
				}},
			},
			wantKinds:    []EventKind{EventCallStarted, EventPermissionResolved, EventCallFailed},
			wantDecision: permission.ActionDeny,
			wantCode:     tool.ErrorCodePermissionDenied,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := &fakeHandler{
				definition: tool.Definition{
					Name:                "bash",
					Description:         "fake shell",
					Kind:                tool.KindBash,
					PermissionDetailKey: "command",
				},
			}
			observer := &recordingObserver{}
			service := newTestService(t, handler, test.policy, WithObserver(observer))
			call, err := tool.NewCall(
				"call-redacted",
				"bash",
				[]byte(`{"command":"printf super-secret"}`),
			)
			if err != nil {
				t.Fatalf("NewCall() error = %v", err)
			}

			_, callErr := service.Call(context.Background(), call)
			if test.wantCode == "" && callErr != nil {
				t.Fatalf("Call() error = %v, want nil", callErr)
			}
			if test.wantCode != "" && !errors.Is(callErr, ErrPermissionDenied) {
				t.Fatalf("Call() error = %v, want permission denied", callErr)
			}
			if got, want := handler.calls, test.wantCalls; got != want {
				t.Fatalf("handler calls = %d, want %d", got, want)
			}

			events := observer.Events()
			if got, want := eventKinds(events), test.wantKinds; !equalEventKinds(got, want) {
				t.Fatalf("event kinds = %#v, want %#v", got, want)
			}
			if got := events[1].Decision; got != test.wantDecision {
				t.Fatalf("permission decision = %q, want %q", got, test.wantDecision)
			}
			if events[len(events)-1].ErrorCode != test.wantCode {
				t.Fatalf("terminal error code = %q, want %q", events[len(events)-1].ErrorCode, test.wantCode)
			}
			for _, event := range events {
				if event.Time.IsZero() {
					t.Fatal("event time is zero")
				}
			}

			encoded, err := json.Marshal(events)
			if err != nil {
				t.Fatalf("Marshal(events) error = %v", err)
			}
			if strings.Contains(string(encoded), "super-secret") {
				t.Fatalf("telemetry contains private argument: %s", encoded)
			}
		})
	}
}

func TestWithObserverRequiresObserver(t *testing.T) {
	handler := &fakeHandler{
		definition: tool.Definition{
			Name:        "bash",
			Description: "fake shell",
			Kind:        tool.KindBash,
		},
	}
	policy, err := permission.NewPolicy(permission.Config{})
	if err != nil {
		t.Fatalf("NewPolicy() error = %v", err)
	}
	_, err = NewService(
		&fakeRegistry{handler: handler},
		policy,
		WithObserver(nil),
	)
	if err == nil {
		t.Fatal("NewService() error = nil, want invalid observer")
	}
	if !errors.Is(err, ErrInvalidService) {
		t.Fatalf("NewService() error = %v, want invalid service", err)
	}
}

type recordingObserver struct {
	mu     sync.Mutex
	events []Event
}

func (o *recordingObserver) Observe(_ context.Context, event Event) {
	o.mu.Lock()
	o.events = append(o.events, event)
	o.mu.Unlock()
}

func (o *recordingObserver) Events() []Event {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]Event{}, o.events...)
}

func eventKinds(events []Event) []EventKind {
	kinds := make([]EventKind, 0, len(events))
	for _, event := range events {
		kinds = append(kinds, event.Kind)
	}
	return kinds
}

func equalEventKinds(left []EventKind, right []EventKind) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
