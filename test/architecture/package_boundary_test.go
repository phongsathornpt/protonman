package architecture_test

import "testing"

func TestPackageWithinMatchesPackageBoundaries(t *testing.T) {
	tests := []struct {
		name       string
		importPath string
		prefix     string
		want       bool
	}{
		{name: "exact package", importPath: "example/internal/app", prefix: "example/internal/app", want: true},
		{name: "subpackage", importPath: "example/internal/app/appdirs", prefix: "example/internal/app", want: true},
		{name: "trailing slash prefix", importPath: "example/internal/app/appdirs", prefix: "example/internal/app/", want: true},
		{name: "adjacent package name", importPath: "example/internal/application", prefix: "example/internal/app", want: false},
		{name: "longer sibling prefix", importPath: "example/internal/app2", prefix: "example/internal/app", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := packageWithin(test.importPath, test.prefix); got != test.want {
				t.Fatalf("packageWithin(%q, %q) = %v, want %v", test.importPath, test.prefix, got, test.want)
			}
		})
	}
}
