package main

import (
	"context"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/tool"
)

func TestBuildRuntimeRegistersAskQuestionInInteractiveMode(t *testing.T) {
	runtime, err := buildRuntime(context.Background(), cliOptions{})
	if err != nil {
		t.Fatalf("buildRuntime() error = %v", err)
	}
	defer runtime.Close()

	if runtime.questionBridge == nil {
		t.Fatal("interactive buildRuntime() did not create questionBridge")
	}

	found := false
	for _, def := range runtime.registry.Definitions() {
		if def.Name == tool.NameAskQuestion {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("interactive tool registry missing %q in definitions", tool.NameAskQuestion)
	}
	if _, ok := runtime.registry.Lookup(tool.NameAskQuestion); !ok {
		t.Fatalf("interactive tool registry cannot lookup %q", tool.NameAskQuestion)
	}
}

func TestBuildRuntimeOmitsAskQuestionInHeadlessMode(t *testing.T) {
	runtime, err := buildRuntime(context.Background(), cliOptions{headless: true})
	if err != nil {
		t.Fatalf("buildRuntime(headless) error = %v", err)
	}
	defer runtime.Close()

	if runtime.questionBridge != nil {
		t.Fatal("headless buildRuntime() unexpectedly created questionBridge")
	}

	for _, def := range runtime.registry.Definitions() {
		if def.Name == tool.NameAskQuestion {
			t.Fatalf("headless tool registry unexpectedly published %q", tool.NameAskQuestion)
		}
	}
	if _, ok := runtime.registry.Lookup(tool.NameAskQuestion); ok {
		t.Fatalf("headless tool registry unexpectedly looked up %q", tool.NameAskQuestion)
	}
}

func TestBuildRuntimeOmitsAskQuestionInACPMode(t *testing.T) {
	runtime, err := buildRuntime(context.Background(), cliOptions{acp: true})
	if err != nil {
		t.Fatalf("buildRuntime(acp) error = %v", err)
	}
	defer runtime.Close()

	if runtime.questionBridge != nil {
		t.Fatal("acp buildRuntime() unexpectedly created questionBridge")
	}

	for _, def := range runtime.registry.Definitions() {
		if def.Name == tool.NameAskQuestion {
			t.Fatalf("acp tool registry unexpectedly published %q", tool.NameAskQuestion)
		}
	}
}
