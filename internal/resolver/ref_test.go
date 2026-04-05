package resolver

import "testing"

func TestResolveProviderSource(t *testing.T) {
	tests := []struct {
		name string
		ref  string
		want string
	}{
		{name: "short provider ref", ref: "sourceplane/lite-ci", want: "ghcr.io/sourceplane/lite-ci"},
		{name: "short provider ref with tag", ref: "sourceplane/lite-ci:v1", want: "ghcr.io/sourceplane/lite-ci:v1"},
		{name: "explicit registry ref", ref: "ghcr.io/sourceplane/lite-ci:v1", want: "ghcr.io/sourceplane/lite-ci:v1"},
		{name: "localhost registry ref", ref: "localhost/tinx/provider:v1", want: "localhost/tinx/provider:v1"},
		{name: "scheme ref", ref: "custom://acme/setup@v1", want: "custom://acme/setup@v1"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ResolveProviderSource(test.ref); got != test.want {
				t.Fatalf("ResolveProviderSource(%q) = %q, want %q", test.ref, got, test.want)
			}
		})
	}
}
