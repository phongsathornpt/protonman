//go:build desktop || desktop_gio

package controller

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestAgentSessionLockRegistryReleasesUnusedKeys(t *testing.T) {
	var registry agentSessionLockRegistry
	for index := range 1024 {
		unlock := registry.lock(fmt.Sprintf("agent-%d", index))
		unlock()
	}
	if count := len(registry.locks); count != 0 {
		t.Fatalf("retained %d idle agent locks", count)
	}
}

func TestAgentSessionLockRegistryKeepsWaitersOnSameLock(t *testing.T) {
	var registry agentSessionLockRegistry
	firstUnlock := registry.lock("reviewer")
	acquired := make(chan func(), 1)
	finished := make(chan struct{})
	go func() {
		unlock := registry.lock("reviewer")
		acquired <- unlock
		close(finished)
	}()

	select {
	case <-acquired:
		t.Fatal("second operation acquired the agent lock concurrently")
	case <-time.After(10 * time.Millisecond):
	}
	firstUnlock()

	var secondUnlock func()
	select {
	case secondUnlock = <-acquired:
	case <-time.After(time.Second):
		t.Fatal("waiting operation did not acquire the agent lock")
	}
	if count := len(registry.locks); count != 1 {
		t.Fatalf("lock entries while held = %d, want 1", count)
	}
	secondUnlock()
	<-finished
	if count := len(registry.locks); count != 0 {
		t.Fatalf("retained %d released agent locks", count)
	}
}

func TestAgentSessionLockRegistryCancelsWaitingAcquisition(t *testing.T) {
	var registry agentSessionLockRegistry
	firstUnlock := registry.lock("reviewer")
	ctx, cancel := context.WithCancel(context.Background())
	acquired := make(chan bool, 1)
	go func() {
		unlock, ok := registry.lockContext(ctx, "reviewer")
		if ok {
			unlock()
		}
		acquired <- ok
	}()
	cancel()

	select {
	case ok := <-acquired:
		if ok {
			t.Fatal("cancelled waiter acquired the agent lock")
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled waiter remained blocked")
	}
	if count := registry.locks["reviewer"].refs; count != 1 {
		t.Fatalf("retained waiter references = %d, want only the holder", count)
	}
	firstUnlock()
	if count := len(registry.locks); count != 0 {
		t.Fatalf("retained %d released agent locks", count)
	}
}
