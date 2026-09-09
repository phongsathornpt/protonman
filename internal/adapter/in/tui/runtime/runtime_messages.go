package runtime

import (
	"github.com/phongsathornpt/protonman/internal/core/tool"
)

type toolResultMsg struct {
	call   tool.Call
	result tool.Result
	err    error
}
