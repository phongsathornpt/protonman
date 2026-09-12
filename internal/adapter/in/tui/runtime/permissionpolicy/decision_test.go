package permissionpolicy

import (
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/permission"
)

func TestResolvePersistenceScopes(t *testing.T) {
	project := Resolve(AllowProject)
	if project.Persist != PersistProject || project.Resolution.Action != permission.ActionAllow {
		t.Fatalf("project decision = %+v", project)
	}
	deny := Resolve(Deny)
	if deny.Persist != PersistNone || deny.Resolution.Action != permission.ActionDeny {
		t.Fatalf("deny decision = %+v", deny)
	}
}
