package architecture_test

import (
	"context"
	"testing"

	"github.com/phongsathornpt/protonman/pkg/proton-sdk/domain"
	"github.com/phongsathornpt/protonman/pkg/proton-sdk/port"
	"github.com/phongsathornpt/protonman/pkg/proton-sdk/usecase"
)

var (
	_ = domain.Request{}
	_ = domain.RequestRequirements{}
	_ = domain.Response{}
	_ = usecase.ResponseAccumulator{}
	_ = domain.Message{}
	_ = domain.Tool{}
	_ = domain.Event{}
	_ = domain.Usage{}
	_ = domain.ModelMetadata{}

	_ = usecase.Collect
	_ = usecase.AppendAssistantResponse

	// Compatibility surfaces remain intentionally available until a planned
	// breaking release removes them.
	_ = domain.StepResult{}
	_ = usecase.CollectStep
	_ = usecase.AppendAssistantStep
)

type architectureMetadataModel struct{}

func (architectureMetadataModel) Provider() string { return "architecture-test" }
func (architectureMetadataModel) ModelID() string  { return "architecture-test" }
func (architectureMetadataModel) Capabilities() domain.ModelCapabilities {
	return domain.ModelCapabilities{Streaming: true}
}
func (architectureMetadataModel) Metadata() domain.ModelMetadata { return domain.ModelMetadata{} }
func (architectureMetadataModel) Stream(context.Context, domain.Request) (port.Stream, error) {
	return nil, nil
}

func TestCanonicalSDKMetadataContract(t *testing.T) {
	var model port.LanguageModel = architectureMetadataModel{}
	metadataModel, ok := model.(port.MetadataModel)
	if !ok {
		t.Fatal("canonical SDK model must support MetadataModel in this contract fixture")
	}
	_ = metadataModel.Metadata()
}

func TestCanonicalSDKRequestRequirementContract(t *testing.T) {
	request := domain.Request{Messages: []domain.Message{{Role: domain.RoleUser, Content: "hello"}}}
	requirements := request.Requirements()
	if !requirements.Streaming {
		t.Fatal("Go SDK requests must require the streaming execution contract")
	}
	if !(domain.ModelCapabilities{Streaming: true}).Satisfies(requirements) {
		t.Fatal("streaming model should satisfy a text-only canonical request")
	}
}
