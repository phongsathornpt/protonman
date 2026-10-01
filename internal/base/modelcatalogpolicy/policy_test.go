package modelcatalogpolicy

import "testing"

func TestIsFreeModel(t *testing.T) {
	cases := []struct {
		name    string
		modelID string
		want    bool
	}{
		{"routed openrouter free", "cline/openrouter/free", true},
		{"routed openrouter free bare", "openrouter/free", true},
		{"routed colon free", "cline/google/gemma-4-26b-a4b-it:free", true},
		{"routed contributor basename", "cline/meta/muse-spark-1.3-contributor", true},
		{"routed flash basename", "cline/deepseek/deepseek-v4.1-flash", true},
		{"routed mimo basename", "cline/xiaomi/mimo-v2.6-flash", true},
		{"cline-free route", "cline-free/mimo-v2.6-flash", true},
		{"parenthesized free", "MiMo-V2.6-Flash (free)", true},
		{"stealth canary", "stealth/pixel-canary", true},
		{"stealth bunny", "stealth/space-bunny-alpha", true},
		{"free basename steath route", "stealth/deepseek-v4.1-flash", true},
		{"kat coder pro routed", "stealth/kat-coder-pro", true},
		{"exact big pickle", "big-pickle", true},
		{"exact free", "free", true},
		{"suffix dash free", "some-model-free", true},
		{"case insensitive", "Big-Pickle", true},
		{"whitespace trimmed", "  big-pickle  ", true},

		{"proton route is not free", "proton/glm-5.3-flash", false},
		{"protonman route is not free", "protonman/deepseek-v4.1-flash", false},
		{"proton basename does not inherit free", "proton/muse-spark-1.3-contributor", false},
		{"proton flash is metered", "proton/deepseek-v4.1-flash", false},
		{"paid claude", "claude-3-7-sonnet", false},
		{"paid coder", "deepseek-coder", false},
		{"empty", "", false},
		{"whitespace only", "   ", false},
		{"unrouted free basename is not free", "muse-spark-1.3-contributor", false},
		{"unknown provider route is not free", "openrouter/deepseek-v4.1-flash", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsFreeModel(tc.modelID); got != tc.want {
				t.Errorf("IsFreeModel(%q) = %v, want %v", tc.modelID, got, tc.want)
			}
		})
	}
}

func TestFallbackModelsForProvider(t *testing.T) {
	for _, provider := range []string{"protonman", "openai", "anthropic", "ollama", "custom"} {
		models := FallbackModelsForProvider(provider)
		if len(models) == 0 {
			t.Fatalf("expected fallback models for provider %q", provider)
		}
		if HasAnyFreeModel(models) {
			t.Errorf("provider %q fallback unexpectedly contains a free model: %v", provider, models)
		}
	}

	// opencode is the one curated provider whose default route is free.
	if !HasAnyFreeModel(FallbackModelsForProvider("opencode")) {
		t.Error("expected opencode fallback to advertise a free model")
	}
}

func TestFallbackModelsForProviderIsCaseInsensitive(t *testing.T) {
	if got := FallbackModelsForProvider("  ProtonMan "); len(got) != len(FallbackModelsForProvider("protonman")) {
		t.Errorf("provider lookup is not case/space insensitive: %v", got)
	}
}

func TestFallbackModelsForProviderReturnsCopy(t *testing.T) {
	first := FallbackModelsForProvider("openai")
	first[0] = "mutated"
	if second := FallbackModelsForProvider("openai"); second[0] == "mutated" {
		t.Error("FallbackModelsForProvider returned a slice aliasing the curated table")
	}
}

func TestPartitionModelsPreservesOrder(t *testing.T) {
	input := []string{
		"cline/anthropic/claude-3-5-sonnet",
		"cline/google/gemma-4-26b-a4b-it:free",
		"cline/deepseek/deepseek-chat",
		"cline/qwen/qwen3.8-27b:free",
	}
	free, other := PartitionModels(input)
	if len(free) != 2 || free[0] != input[1] || free[1] != input[3] {
		t.Errorf("unexpected free partition: %v", free)
	}
	if len(other) != 2 || other[0] != input[0] || other[1] != input[2] {
		t.Errorf("unexpected other partition: %v", other)
	}
}

func TestHasAnyFreeModel(t *testing.T) {
	if !HasAnyFreeModel([]string{"gpt-4o", "big-pickle"}) {
		t.Error("expected HasAnyFreeModel to find a free model")
	}
	if HasAnyFreeModel([]string{"gpt-4o", "o3-mini"}) {
		t.Error("expected HasAnyFreeModel to be false for paid models")
	}
	if HasAnyFreeModel(nil) {
		t.Error("expected HasAnyFreeModel to be false for an empty list")
	}
}

func TestLabelFallsBackToIdentifier(t *testing.T) {
	if got := Label("gpt-4o"); got != "GPT-4o" {
		t.Errorf("Label(gpt-4o) = %q, want GPT-4o", got)
	}
	if got := Label("some-unlisted-model"); got != "some-unlisted-model" {
		t.Errorf("expected unlisted model to label as itself, got %q", got)
	}
}

func TestLabeledModelsCoversEveryProvider(t *testing.T) {
	for _, provider := range []string{"protonman", "openai", "anthropic", "opencode", "opencode-zen", "opencode-go", "ollama", "unknown-provider"} {
		labeled := LabeledModels(provider)
		if len(labeled) == 0 {
			t.Fatalf("expected labeled models for provider %q", provider)
		}
		want := FallbackModelsForProvider(provider)
		if len(labeled) != len(want) {
			t.Fatalf("provider %q: labeled count %d != fallback count %d", provider, len(labeled), len(want))
		}
		for i, preset := range labeled {
			if preset.Model != want[i] {
				t.Errorf("provider %q: model[%d] = %q, want %q", provider, i, preset.Model, want[i])
			}
			if preset.Name == "" {
				t.Errorf("provider %q: model %q has no label", provider, preset.Model)
			}
		}
	}
}
