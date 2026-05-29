package registry

import "testing"

func TestParseRef(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    Ref
		wantErr bool
	}{
		{
			name: "ghcr with namespace and tag",
			in:   "ghcr.io/surajssd/dotfiles/openclaw:latest",
			want: Ref{Registry: "ghcr.io", Repository: "surajssd/dotfiles/openclaw", Tag: "latest"},
		},
		{
			name: "ghcr without tag defaults to latest",
			in:   "ghcr.io/foo/bar",
			want: Ref{Registry: "ghcr.io", Repository: "foo/bar", Tag: "latest"},
		},
		{
			name: "docker hub short form",
			in:   "alpine:3.20",
			want: Ref{Registry: "docker.io", Repository: "alpine", Tag: "3.20"},
		},
		{
			name: "docker hub user/image without tag",
			in:   "library/alpine",
			want: Ref{Registry: "docker.io", Repository: "library/alpine", Tag: "latest"},
		},
		{
			name: "registry with port",
			in:   "registry.local:5000/team/img:v1",
			want: Ref{Registry: "registry.local:5000", Repository: "team/img", Tag: "v1"},
		},
		{
			name: "localhost registry",
			in:   "localhost/foo:bar",
			want: Ref{Registry: "localhost", Repository: "foo", Tag: "bar"},
		},
		{
			name:    "empty",
			in:      "",
			wantErr: true,
		},
		{
			name:    "digest reference rejected",
			in:      "ghcr.io/foo/bar@sha256:abc",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseRef(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %#v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestExtractJSONField(t *testing.T) {
	body := `{"token":"abc123","access_token":"xyz","expires_in":300}`
	if got := extractJSONField(body, "token"); got != "abc123" {
		t.Fatalf("token: got %q want %q", got, "abc123")
	}
	if got := extractJSONField(body, "access_token"); got != "xyz" {
		t.Fatalf("access_token: got %q want %q", got, "xyz")
	}
	if got := extractJSONField(body, "missing"); got != "" {
		t.Fatalf("missing: got %q want empty", got)
	}
}
