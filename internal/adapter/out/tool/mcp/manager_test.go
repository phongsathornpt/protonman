package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/projectTHORN/proton/internal/adapter/out/tool/builtin"
)

type managedFakeServer struct {
	name       string
	tools      []Tool
	listErr    error
	closeOrder *[]string
	closed     int
}

func (s *managedFakeServer) Name() string { return s.name }
func (s *managedFakeServer) ListTools(context.Context) ([]Tool, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return append([]Tool(nil), s.tools...), nil
}
func (s *managedFakeServer) CallTool(context.Context, string, json.RawMessage) (Result, error) {
	return Result{}, nil
}
func (s *managedFakeServer) Close() error {
	s.closed++
	if s.closeOrder != nil {
		*s.closeOrder = append(*s.closeOrder, s.name)
	}
	return nil
}

func TestManagerBindAndCloseReverseOrder(t *testing.T) {
	order := []string{}
	first := &managedFakeServer{name: "first", tools: []Tool{{Name: "one"}}, closeOrder: &order}
	second := &managedFakeServer{name: "second", tools: []Tool{{Name: "two"}}, closeOrder: &order}
	manager, err := NewManager(first, second)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := builtin.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Bind(context.Background(), registry); err != nil {
		t.Fatal(err)
	}
	if len(registry.Definitions()) != 2 {
		t.Fatalf("definitions = %d", len(registry.Definitions()))
	}
	if err := manager.Close(); err != nil {
		t.Fatal(err)
	}
	if err := manager.Close(); err != nil {
		t.Fatal(err)
	}
	if got := len(order); got != 2 || order[0] != "second" || order[1] != "first" {
		t.Fatalf("close order = %#v", order)
	}
	if first.closed != 1 || second.closed != 1 {
		t.Fatalf("close counts = %d, %d", first.closed, second.closed)
	}
}

func TestManagerBindFailureCleansUpServers(t *testing.T) {
	boom := errors.New("boom")
	first := &managedFakeServer{name: "first"}
	second := &managedFakeServer{name: "second", listErr: boom}
	manager, err := NewManager(first, second)
	if err != nil {
		t.Fatal(err)
	}
	registry, _ := builtin.NewRegistry()
	if err := manager.Bind(context.Background(), registry); !errors.Is(err, boom) {
		t.Fatalf("Bind error = %v", err)
	}
	if first.closed != 1 || second.closed != 1 {
		t.Fatalf("close counts = %d, %d", first.closed, second.closed)
	}
}
