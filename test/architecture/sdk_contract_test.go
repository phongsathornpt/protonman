package architecture_test

import (
	"context"
	"testing"

	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

var (
	_ = sdk.Request{}
	_ = sdk.Response{}
	_ = sdk.Message{}
	_ = sdk.Tool{}
	_ = sdk.Event{}
	_ = sdk.Usage{}
	_ = sdk.ModelMetadata{}

	_ = sdk.Collect
	_ = sdk.AppendAssistantResponse

	// Compatibility surfaces remain intentionally available until a planned
	// breaking release removes them.
	_ = sdk.StepResult{}
	_ = sdk.CollectStep
	_ = sdk.AppendAssistantStep
)

type architectureMetadataModel struct{}

func (architectureMetadataModel) Provider() string { return "architecture-test" }
func (architectureMetadataModel) ModelID() string  { return "architecture-test" }
func (architectureMetadataModel) Capabilities() sdk.ModelCapabilities {
	return sdk.ModelCapabilities{Streaming: true}
}
func (architectureMetadataModel) Metadata() sdk.ModelMetadata { return sdk.ModelMetadata{} }
func (architectureMetadataModel) Stream(context.Context, sdk.Request) (sdk.Stream, error) {
	return nil, nil
}

func TestCanonicalSDKMetadataContract(t *testing.T) {
	var model sdk.LanguageModel = architectureMetadataModel{}
	metadataModel, ok := model.(sdk.MetadataModel)
	if !ok {
		t.Fatal("canonical SDK model must support MetadataModel in this contract fixture")
	}
	_ = metadataModel.Metadata()
}
