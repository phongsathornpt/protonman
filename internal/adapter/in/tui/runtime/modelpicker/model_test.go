package modelpicker

import (
	"testing"

	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
)

func TestDisplayNamePrefersName(t *testing.T) {
	got := DisplayName(model.RemoteModel{ID: "ignored-id", Name: "DeepSeek V4"})
	if got != "DeepSeek V4" {
		t.Fatalf("DisplayName() = %q, want %q", got, "DeepSeek V4")
	}
}

func TestDisplayNameNormalizesFreeSuffix(t *testing.T) {
	got := DisplayName(model.RemoteModel{ID: "nemotron-3.5-lightning-free"})
	if got != "Nemotron 3.5 Lightning" {
		t.Fatalf("DisplayName() = %q, want %q", got, "Nemotron 3.5 Lightning")
	}
}

func TestActiveModelIndex(t *testing.T) {
	models := []model.RemoteModel{{ID: "one"}, {ID: "two"}}
	if got := ActiveModelIndex(models, "TWO"); got != 1 {
		t.Fatalf("ActiveModelIndex() = %d, want 1", got)
	}
	if got := ActiveModelIndex(models, "missing"); got != 0 {
		t.Fatalf("ActiveModelIndex() missing = %d, want 0", got)
	}
}
