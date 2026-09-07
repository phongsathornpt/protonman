package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
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

type mutableManagedServer struct {
	mu      sync.Mutex
	name    string
	tools   []Tool
	listErr error
}

func (s *mutableManagedServer) Name() string { return s.name }
func (s *mutableManagedServer) ListTools(context.Context) ([]Tool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listErr != nil {
		return nil, s.listErr
	}
	return append([]Tool(nil), s.tools...), nil
}
func (s *mutableManagedServer) CallTool(context.Context, string, json.RawMessage) (Result, error) {
	return Result{}, nil
}
func (s *mutableManagedServer) Close() error { return nil }
func (s *mutableManagedServer) setTools(tools []Tool) {
	s.mu.Lock()
	s.tools = append([]Tool(nil), tools...)
	s.mu.Unlock()
}
func (s *mutableManagedServer) setError(err error) { s.mu.Lock(); s.listErr = err; s.mu.Unlock() }

func TestManagerRefreshReplacesCatalogAtomically(t *testing.T) {
	server := &mutableManagedServer{name: "db", tools: []Tool{{Name: "old"}}}
	manager, err := NewManager(server)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	registry, _ := builtin.NewRegistry()
	if err := manager.Bind(context.Background(), registry); err != nil {
		t.Fatal(err)
	}
	if manager.CatalogGeneration("db") != 1 {
		t.Fatalf("initial generation = %d", manager.CatalogGeneration("db"))
	}

	server.setTools([]Tool{{Name: "new"}, {Name: "second"}})
	if err := manager.Refresh(context.Background(), "db"); err != nil {
		t.Fatal(err)
	}
	if _, ok := registry.Lookup("mcp.db.old"); ok {
		t.Fatal("stale tool survived refresh")
	}
	if _, ok := registry.Lookup("mcp.db.new"); !ok {
		t.Fatal("new tool missing after refresh")
	}
	if _, ok := registry.Lookup("mcp.db.second"); !ok {
		t.Fatal("second tool missing after refresh")
	}
	if manager.CatalogGeneration("db") != 2 {
		t.Fatalf("generation = %d, want 2", manager.CatalogGeneration("db"))
	}

	server.setTools([]Tool{{Name: "bad tool"}})
	if err := manager.Refresh(context.Background(), "db"); err == nil {
		t.Fatal("invalid refresh error = nil")
	}
	if _, ok := registry.Lookup("mcp.db.new"); !ok {
		t.Fatal("failed refresh removed valid prior catalog")
	}
	if manager.CatalogGeneration("db") != 2 {
		t.Fatal("failed refresh advanced generation")
	}
}

func TestManagerRefreshFailureRetainsCatalog(t *testing.T) {
	server := &mutableManagedServer{name: "db", tools: []Tool{{Name: "query"}}}
	manager, _ := NewManager(server)
	defer manager.Close()
	registry, _ := builtin.NewRegistry()
	if err := manager.Bind(context.Background(), registry); err != nil {
		t.Fatal(err)
	}
	server.setError(errors.New("offline"))
	if err := manager.Refresh(context.Background(), "db"); err == nil {
		t.Fatal("refresh error = nil")
	}
	if _, ok := registry.Lookup("mcp.db.query"); !ok {
		t.Fatal("transport failure removed prior catalog")
	}
}
