//go:build desktop || desktop_gio

package controller

import (
	"context"
	"testing"
	"time"
)

func TestAgentSessionLockRegistryReleasesUnusedKeys(t *testing.T) {
	var registry agentSessionLockRegistry
	for range 1024 {
		unlock := registry.lock()
		unlock()
	}
	if registry.locked != nil {
		t.Fatal("retained idle session lock")
	}
}

func TestAgentSessionLockRegistryKeepsWaitersOnSameLock(t *testing.T) {
	var registry agentSessionLockRegistry
	firstUnlock := registry.lock()
	acquired := make(chan func(), 1)
	finished := make(chan struct{})
	go func() {
		unlock := registry.lock()
		acquired <- unlock
		close(finished)
	}()

	select {
	case <-acquired:
		t.Fatal("second operation acquired the session lock concurrently")
	case <-time.After(10 * time.Millisecond):
	}
	firstUnlock()

	var secondUnlock func()
	select {
	case secondUnlock = <-acquired:
	case <-time.After(time.Second):
		t.Fatal("waiting operation did not acquire the session lock")
	}
	if registry.locked == nil {
		t.Fatal("session lock missing while held")
	}
	secondUnlock()
	<-finished
	if registry.locked != nil {
		t.Fatal("retained released session lock")
	}
}

func TestAgentSessionLockRegistryCancelsWaitingAcquisition(t *testing.T) {
	var registry agentSessionLockRegistry
	firstUnlock := registry.lock()
	ctx, cancel := context.WithCancel(context.Background())
	acquired := make(chan bool, 1)
	go func() {
		unlock, ok := registry.lockContext(ctx)
		if ok {
			unlock()
		}
		acquired <- ok
	}()
	cancel()

	select {
	case ok := <-acquired:
		if ok {
			t.Fatal("cancelled waiter acquired the session lock")
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled waiter remained blocked")
	}
	if registry.locked == nil || registry.locked.refs != 1 {
		t.Fatal("retained waiter references, want only the holder")
	}
	firstUnlock()
	if registry.locked != nil {
		t.Fatal("retained released session lock")
	}
}
