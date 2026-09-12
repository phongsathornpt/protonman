package permissionpolicy

import "github.com/phongsathornpt/protonman/internal/core/permission"

type PersistenceScope string

const (
	PersistNone    PersistenceScope = ""
	PersistProject PersistenceScope = "project"
	PersistGlobal  PersistenceScope = "global"
)

type Decision struct {
	Resolution permission.Resolution
	Persist    PersistenceScope
}

func Resolve(option Option) Decision {
	switch option {
	case AllowOnce:
		return Decision{Resolution: permission.Resolution{Action: permission.ActionAllow, Scope: permission.GrantScopeOnce, Reason: "user allowed one call"}}
	case AllowSession:
		return Decision{Resolution: permission.Resolution{Action: permission.ActionAllow, Scope: permission.GrantScopeSession, Reason: "user allowed this exact request for the session"}}
	case AllowProject:
		return Decision{Resolution: permission.Resolution{Action: permission.ActionAllow, Scope: permission.GrantScopeOnce, Reason: "user allowed call and saved rule to project"}, Persist: PersistProject}
	case AllowGlobal:
		return Decision{Resolution: permission.Resolution{Action: permission.ActionAllow, Scope: permission.GrantScopeOnce, Reason: "user allowed call and saved rule globally"}, Persist: PersistGlobal}
	default:
		return Decision{Resolution: permission.Resolution{Action: permission.ActionDeny, Reason: "user denied one call"}}
	}
}
