package acp

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/engine/turn"
	"github.com/phongsathornpt/protonman/proton-sdk/domain"
)

type acpUsageRunner struct {
	used       int64
	size       int64
	generation uint64
	ok         bool
}

func (r *acpUsageRunner) Run(context.Context, []domain.Message, turn.Sink) (turn.Result, error) {
	return turn.Result{}, nil
}

func (r *acpUsageRunner) ContextUsage() (int64, int64, uint64, bool) {
	return r.used, r.size, r.generation, r.ok
}

func TestWriteSessionUsageNotificationEmitsStableUsageUpdateOncePerGeneration(t *testing.T) {
	runner := &acpUsageRunner{used: 53_000, size: 200_000, generation: 1, ok: true}
	sess := &Session{id: "session-usage", runner: runner}
	server := &Server{}
	var output bytes.Buffer

	if err := server.writeSessionUsageNotification(&output, sess); err != nil {
		t.Fatalf("writeSessionUsageNotification() error = %v", err)
	}
	got := output.String()
	for _, want := range []string{
		`"method":"session/update"`,
		`"sessionId":"session-usage"`,
		`"sessionUpdate":"usage_update"`,
		`"used":53000`,
		`"size":200000`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("usage notification missing %s: %s", want, got)
		}
	}

	before := output.Len()
	if err := server.writeSessionUsageNotification(&output, sess); err != nil {
		t.Fatalf("duplicate writeSessionUsageNotification() error = %v", err)
	}
	if output.Len() != before {
		t.Fatalf("duplicate generation emitted another notification: %s", output.String()[before:])
	}

	runner.used = 54_321
	runner.generation = 2
	if err := server.writeSessionUsageNotification(&output, sess); err != nil {
		t.Fatalf("next writeSessionUsageNotification() error = %v", err)
	}
	if !strings.Contains(output.String()[before:], `"used":54321`) {
		t.Fatalf("next generation missing updated usage: %s", output.String()[before:])
	}
}

func TestWriteSessionUsageNotificationSkipsUnavailableUsage(t *testing.T) {
	runner := &acpUsageRunner{used: 42, size: 0, generation: 1, ok: true}
	sess := &Session{id: "session-usage", runner: runner}
	server := &Server{}
	var output bytes.Buffer

	if err := server.writeSessionUsageNotification(&output, sess); err != nil {
		t.Fatalf("writeSessionUsageNotification() error = %v", err)
	}
	if output.Len() != 0 {
		t.Fatalf("usage notification emitted without a known context window: %s", output.String())
	}
}
