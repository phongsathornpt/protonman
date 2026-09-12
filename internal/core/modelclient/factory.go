// Package modelclient defines provider-neutral model client construction ports.
package modelclient

import (
	"time"

	"github.com/phongsathornpt/protonman/internal/core/modelcatalog"
	"github.com/phongsathornpt/protonman/internal/core/modelconfig"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

// Request contains provider-neutral inputs for one language model client.
type Request struct {
	ProviderName   string
	ProviderType   string
	BaseURL        string
	APIKey         string
	ModelID        string
	SessionID      string
	AgentProfile   string
	RequestTimeout time.Duration
	RemoteModel    *modelcatalog.RemoteModel
	LowConcurrency modelconfig.LowConcurrencySetting
}

// Factory constructs provider-specific language models behind a core port.
type Factory interface {
	Build(Request) sdk.LanguageModel
}
