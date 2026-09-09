package glob

import "testing"

func TestMatch(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		value   string
		want    bool
	}{
		{name: "empty empty", pattern: "", value: "", want: true},
		{name: "empty pattern with value", pattern: "", value: "foo", want: false},
		{name: "star matches all", pattern: "*", value: "anything", want: true},
		{name: "star matches empty", pattern: "*", value: "", want: true},
		{name: "exact match", pattern: "abc", value: "abc", want: true},
		{name: "exact mismatch", pattern: "abc", value: "def", want: false},
		{name: "prefix match", pattern: "rm *", value: "rm -rf /tmp", want: true},
		{name: "prefix mismatch", pattern: "rm *", value: "cp file", want: false},
		{name: "suffix match", pattern: "*.go", value: "main.go", want: true},
		{name: "suffix nested", pattern: "*.go", value: "pkg/sub/file.go", want: true},
		{name: "question mark", pattern: "file?.txt", value: "file1.txt", want: true},
		{name: "question mark mismatch", pattern: "file?.txt", value: "file12.txt", want: false},
		{name: "multiple wildcards", pattern: "*perm*.*go", value: "internal/permission/permission.go", want: true},
		{name: "unicode matching", pattern: "*โลก*", value: "สวัสดีชาวโลกทุกคน", want: true},
		{name: "unicode question", pattern: "สวัส?ี", value: "สวัสดี", want: true},
		{name: "double star prefix", pattern: "**/*.pem", value: "certs/server.pem", want: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Match(tc.pattern, tc.value)
			if got != tc.want {
				t.Errorf("Match(%q, %q) = %v, want %v", tc.pattern, tc.value, got, tc.want)
			}
		})
	}
}

func BenchmarkGlobMatchWildcard(b *testing.B) {
	pattern := "*"
	value := "some/long/nested/path/to/a/workspace/file.go"
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		_ = Match(pattern, value)
	}
}

func BenchmarkGlobMatchPrefix(b *testing.B) {
	pattern := "rm *"
	value := "rm -rf /tmp/test-dir/created-file.txt"
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		_ = Match(pattern, value)
	}
}

func BenchmarkGlobMatchSuffix(b *testing.B) {
	pattern := "*.go"
	value := "internal/permission/permission.go"
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		_ = Match(pattern, value)
	}
}

func BenchmarkGlobMatchComplex(b *testing.B) {
	pattern := "*perm*.*go"
	value := "internal/permission/permission.go"
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		_ = Match(pattern, value)
	}
}
