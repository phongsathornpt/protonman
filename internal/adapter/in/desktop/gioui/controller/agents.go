//go:build desktop || desktop_gio

package controller

import (
	"context"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/phongsathornpt/protonman/internal/adapter/out/acpclient"
)

// ACPAgentProfile is the single supervised Protonman ACP process definition.
// The desktop drives exactly one agent: `protonman --acp`.
type ACPAgentProfile struct {
	ID          string
	DisplayName string
	Command     string
	Args        []string
	Env         []string
}

func DefaultACPAgentProfile() ACPAgentProfile {
	return ACPAgentProfile{
		ID:          ProtonmanAgentID,
		DisplayName: "Protonman",
		Command:     resolveACPBinary(),
		Args:        []string{"--acp"},
	}
}

func CloneACPAgentProfile(profile ACPAgentProfile) ACPAgentProfile {
	profile.Args = slices.Clone(profile.Args)
	profile.Env = slices.Clone(profile.Env)
	return profile
}

func CommandSpecForACPAgent(profile ACPAgentProfile) acpclient.CommandSpec {
	spec := acpclient.CommandSpec{
		Path: strings.TrimSpace(profile.Command),
		Args: slices.Clone(profile.Args),
	}
	for _, key := range profile.Env {
		if strings.ContainsRune(key, '=') {
			spec.Env = append(spec.Env, key)
			continue
		}
		if value, ok := os.LookupEnv(key); ok {
			spec.Env = append(spec.Env, key+"="+value)
		}
	}
	return spec
}

type agentSessionLock struct {
	token chan struct{}
	refs  int
}

type agentSessionLockRegistry struct {
	mu     sync.Mutex
	locked *agentSessionLock
}

func (r *agentSessionLockRegistry) lock() func() {
	unlock, _ := r.lockContext(context.Background())
	return unlock
}

func (r *agentSessionLockRegistry) lockContext(ctx context.Context) (func(), bool) {
	if ctx == nil {
		ctx = context.Background()
	}
	r.mu.Lock()
	lock := r.locked
	if lock == nil {
		lock = &agentSessionLock{token: make(chan struct{}, 1)}
		lock.token <- struct{}{}
		r.locked = lock
	}
	lock.refs++
	r.mu.Unlock()

	select {
	case <-ctx.Done():
		r.release(lock)
		return nil, false
	case <-lock.token:
	}
	if ctx.Err() != nil {
		lock.token <- struct{}{}
		r.release(lock)
		return nil, false
	}
	return func() {
		lock.token <- struct{}{}
		r.release(lock)
	}, true
}

func (r *agentSessionLockRegistry) release(lock *agentSessionLock) {
	r.mu.Lock()
	lock.refs--
	if lock.refs == 0 && r.locked == lock {
		r.locked = nil
	}
	r.mu.Unlock()
}

func (c *controller) lockAgentSession() func() {
	return c.sessionLocks.lock()
}

func (c *controller) lockAgentSessionContext(ctx context.Context) (func(), bool) {
	if ctx == nil {
		ctx = c.context()
	}
	return c.sessionLocks.lockContext(ctx)
}
