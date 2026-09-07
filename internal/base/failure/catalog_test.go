package failure

import "testing"

func TestCatalogCoversStableCodesWithoutDuplicateWireValues(t *testing.T) {
	seen := make(map[Code]struct{})
	for _, code := range Codes() {
		if code == "" {
			t.Fatal("empty stable failure code")
		}
		if _, duplicate := seen[code]; duplicate {
			t.Fatalf("duplicate stable failure code %q", code)
		}
		seen[code] = struct{}{}
		traits, ok := TraitsFor(code)
		if !ok {
			t.Fatalf("failure code %q missing traits", code)
		}
		if traits.Domain == "" {
			t.Fatalf("failure code %q missing domain", code)
		}
	}
	if len(seen) != len(catalog) {
		t.Fatalf("catalog entries = %d, declared codes = %d", len(catalog), len(seen))
	}
}

func TestCancellationTraitsRemainRetryable(t *testing.T) {
	for _, code := range []Code{CodeCanceled, CodeDeadlineExceeded} {
		traits, ok := TraitsFor(code)
		if !ok || !traits.Retryable || !traits.Temporary {
			t.Fatalf("traits for %q = %+v, %v", code, traits, ok)
		}
	}
}
