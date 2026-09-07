package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/projectTHORN/proton/internal/adapter/out/model"
	"github.com/projectTHORN/proton/internal/engine/toolcall"
	"github.com/projectTHORN/proton/internal/engine/turn"
)

type verificationRunner struct {
	result turn.Result
}

func (r verificationRunner) Run(context.Context, []model.Message, turn.Sink) (turn.Result, error) {
	return r.result, nil
}

func verificationCoordinator(result turn.Result) *Coordinator {
	return NewCoordinator(nil, emptyRegistry{}, nil, nil,
		WithRunnerFactory(func(Profile, *toolcall.Service) (turn.Runner, error) {
			return verificationRunner{result: result}, nil
		}),
	)
}
func TestDEXRejectsUnverifiedMutations(t *testing.T) {
	coord := verificationCoordinator(turn.Result{
		Message:      model.Message{Role: model.RoleAssistant, Content: "done"},
		Rounds:       2,
		Verification: turn.VerificationState{Mutated: true},
	})
	defer coord.Close()
	result, err := coord.execute(context.Background(), Request{ID: "dex-1", Profile: ProfileDEX, Task: "edit"})
	if !errors.Is(err, ErrUnverifiedChanges) {
		t.Fatalf("execute() error = %v, want ErrUnverifiedChanges", err)
	}
	if !result.Verification.Mutated || result.Verification.Verified {
		t.Fatalf("verification = %#v", result.Verification)
	}
}

func TestWorkerSurfacesUnverifiedMutationWarning(t *testing.T) {
	coord := verificationCoordinator(turn.Result{
		Message:      model.Message{Role: model.RoleAssistant, Content: "implemented"},
		Verification: turn.VerificationState{Mutated: true},
	})
	defer coord.Close()
	result, err := coord.execute(context.Background(), Request{ID: "worker-1", Profile: ProfilePOW, Task: "edit"})
	if err != nil {
		t.Fatalf("execute() error = %v", err)
	}
	if !strings.Contains(result.Summary, "not verified after the final mutation") {
		t.Fatalf("summary = %q", result.Summary)
	}
}
func TestDEXAcceptsVerifiedMutation(t *testing.T) {
	coord := verificationCoordinator(turn.Result{
		Message: model.Message{Role: model.RoleAssistant, Content: "done"},
		Verification: turn.VerificationState{
			Mutated: true, Verified: true, Verifier: "go test",
		},
	})
	defer coord.Close()
	result, err := coord.execute(context.Background(), Request{ID: "dex-2", Profile: ProfileDEX, Task: "edit"})
	if err != nil {
		t.Fatalf("execute() error = %v", err)
	}
	if !result.Verification.Verified || result.Verification.Verifier != "go test" {
		t.Fatalf("verification = %#v", result.Verification)
	}
}
