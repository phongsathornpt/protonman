package diagnostic

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

var unreachableKinds = []Kind{
	KindInvalidPrompt,
}

var reachableKinds = []Kind{
	KindModelNotFound,
	KindContextOverflow,
	KindAuthentication,
	KindForbidden,
	KindRateLimit,
	KindQuotaExceeded,
	KindServerOverloaded,
	KindStreamTimeout,
	KindStreamIncomplete,
	KindEmptyResponse,
	KindRuntimeTimeout,
	KindMCPFailed,
	KindConfigInvalid,
	KindConfigTypo,
	KindToolFailed,
	KindToolDispatch,
	KindPermissionDenied,
	KindCancelled,
	KindGeneric,
}

func allKinds() []Kind {
	return append(append([]Kind(nil), reachableKinds...), unreachableKinds...)
}

func sortStrings(s []string) {
	slices.Sort(s)
}

// regenerateGolden lets a deliberate, reviewed behavior change refresh the frozen
// classification table. The refreshed file must always be read back in a diff: the
// golden is the contract that ordinary refactors are not allowed to rewrite.
var regenerateGolden = flag.Bool("regenerate-diagnostic-golden", false, "rewrite testdata/classify.golden from the current Classify implementation")

// TestClassifyGolden freezes the full presentation output for every corpus case.
// Classify is a precedence ladder over unstructured provider text, so byte-for-byte
// output comparison is the only trustworthy regression signal.
func TestClassifyGolden(t *testing.T) {
	const path = "testdata/classify.golden"
	corpus := buildClassifyCorpus()

	var got bytes.Buffer
	for _, testCase := range corpus {
		got.WriteString(renderClassifyCase(testCase))
	}
	rendered := got.Bytes()

	if *regenerateGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create golden directory: %v", err)
		}
		if err := os.WriteFile(path, rendered, 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("rewrote %s (%d cases, %d bytes)", path, len(corpus), len(rendered))
		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (refresh deliberately with -regenerate-diagnostic-golden only after reviewing the behavior change)", path, err)
	}
	if !bytes.Equal(want, rendered) {
		t.Fatalf("Classify output diverges from frozen golden %s:\n%s", path, describeGoldenDiff(string(want), string(rendered)))
	}
}

// TestClassifyRawDetailsInvariant keeps the golden small by excluding raw provider
// text, then pins the one rule that must always hold about it instead.
func TestClassifyRawDetailsInvariant(t *testing.T) {
	for _, testCase := range buildClassifyCorpus() {
		result := Classify(testCase.err, testCase.provider, testCase.model)
		want := ""
		if testCase.err != nil {
			want = testCase.err.Error()
		}
		if result.RawDetails != want {
			t.Errorf("%s: RawDetails = %q, want %q", testCase.name, result.RawDetails, want)
		}
	}
}

// TestClassifyReachableKinds freezes which presentation kinds the classifier can
// actually produce. A declared kind with no rule is dead vocabulary: this test
// fails when a rule is added or removed so the list stays honest.
func TestClassifyReachableKinds(t *testing.T) {
	produced := map[Kind]bool{}
	for _, testCase := range buildClassifyCorpus() {
		produced[Classify(testCase.err, testCase.provider, testCase.model).Kind] = true
	}
	for _, kind := range unreachableKinds {
		if produced[kind] {
			t.Errorf("kind %q is now reachable; move it to reachableKinds and refresh the golden", kind)
		}
	}
	for _, kind := range reachableKinds {
		if !produced[kind] {
			t.Errorf("corpus no longer produces kind %q; add a case so the precedence ladder stays pinned", kind)
		}
	}
	accounted := map[Kind]bool{}
	for _, kind := range reachableKinds {
		accounted[kind] = true
	}
	for _, kind := range unreachableKinds {
		accounted[kind] = true
	}
	for _, kind := range allKinds() {
		if !accounted[kind] {
			t.Errorf("kind %q is in neither reachableKinds nor unreachableKinds", kind)
		}
	}
}

// TestIsContextOverflowSamples pins the pattern table independently of Classify,
// including the exclusion precedence that keeps rate-limit text out of overflow.
func TestIsContextOverflowSamples(t *testing.T) {
	for index, sample := range overflowSamples {
		if !isContextOverflow(sample) {
			t.Errorf("overflowSamples[%d] %q no longer matches the overflow pattern table", index, sample)
		}
	}
	for index, sample := range overflowExclusionSamples {
		if isContextOverflow(sample) {
			t.Errorf("overflowExclusionSamples[%d] %q must stay excluded from overflow", index, sample)
		}
	}
}

func renderClassifyCase(testCase classifyCase) string {
	var out strings.Builder
	result := Classify(testCase.err, testCase.provider, testCase.model)
	fmt.Fprintf(&out, "### %s\n", testCase.name)
	fmt.Fprintf(&out, "kind=%q title=%q badge=%q\n", result.Kind, result.Title, result.Badge)
	fmt.Fprintf(&out, "message=%q\n", result.Message)
	fmt.Fprintf(&out, "code=%q retryable=%t usercode=%q\n", result.Code, result.Retryable, UserCode(result.Kind))
	out.WriteString("suggestions=")
	if len(result.Suggestions) == 0 {
		out.WriteString("<none>\n\n")
		return out.String()
	}
	for index, suggestion := range result.Suggestions {
		if index > 0 {
			out.WriteString(" | ")
		}
		fmt.Fprintf(&out, "%q", suggestion)
	}
	out.WriteString("\n\n")
	return out.String()
}

// describeGoldenDiff reports per-case differences instead of dumping two large
// blobs, so a precedence mistake is identifiable at a glance.
func describeGoldenDiff(want, got string) string {
	wanted, gotBlocks := indexGoldenBlocks(want), indexGoldenBlocks(got)
	names := make([]string, 0, len(wanted)+len(gotBlocks))
	seen := map[string]bool{}
	for _, block := range []map[string]string{wanted, gotBlocks} {
		for name := range block {
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}
	sortStrings(names)

	var out strings.Builder
	for _, name := range names {
		oldBlock, inWant := wanted[name]
		newBlock, inGot := gotBlocks[name]
		switch {
		case !inGot:
			fmt.Fprintf(&out, "  - case %q disappeared\n", name)
		case !inWant:
			fmt.Fprintf(&out, "  + case %q is new\n", name)
		case oldBlock != newBlock:
			fmt.Fprintf(&out, "  ~ case %q\n", name)
			out.WriteString(indentBlocks(oldBlock, newBlock))
		}
	}
	return out.String()
}

func indexGoldenBlocks(text string) map[string]string {
	out := map[string]string{}
	for _, block := range strings.Split(text, "\n### ") {
		if name, _, ok := strings.Cut(block, "\n"); ok && name != "" {
			out[name] = strings.TrimRight(block, "\n")
		}
	}
	return out
}

func indentBlocks(want, got string) string {
	wantLines, gotLines := strings.Split(want, "\n"), strings.Split(got, "\n")
	count := max(len(wantLines), len(gotLines))
	var out strings.Builder
	for i := 0; i < count; i++ {
		w, g := lineAt(wantLines, i), lineAt(gotLines, i)
		if w == g {
			continue
		}
		fmt.Fprintf(&out, "      want: %s\n      got:  %s\n", w, g)
	}
	return out.String()
}

func lineAt(lines []string, index int) string {
	if index < len(lines) {
		return lines[index]
	}
	return "<missing>"
}

func BenchmarkClassifyKnownKinds(b *testing.B) {
	cases := []error{
		errors.New("provider request failed: status 401 unauthorized"),
		errors.New("status 429 too many requests"),
		errors.New("model nemotron-3.5-lightning-free is not supported"),
		errors.New("read tcp 10.0.0.1:443: connect: operation timed out"),
		errors.New("MCP server \"github\" failed to initialize"),
		errors.New("turn failed: something completely unexpected"),
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		for _, err := range cases {
			Classify(err, "opencode", "nemotron-3.5-lightning-free")
		}
	}
}

func BenchmarkClassifyContextOverflow(b *testing.B) {
	// Worst case for the pattern table: no exclusion hits and the matching
	// overflow pattern sits at the end of the list.
	err := errors.New(overflowSamples[len(overflowSamples)-1])
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		Classify(err, "opencode", "muse-spark")
	}
}

func BenchmarkClassifyLongUnmatchedPayload(b *testing.B) {
	// Guards against quadratic rescanning of large provider bodies: a 20 KB
	// message that only matches near the very end of the pattern table.
	err := buildLongOverflowSample()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		Classify(err, "opencode", "muse-spark")
	}
}

func BenchmarkIsContextOverflowMiss(b *testing.B) {
	text := "an unrelated provider message with no overflow wording at all whatsoever"
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		isContextOverflow(text)
	}
}
