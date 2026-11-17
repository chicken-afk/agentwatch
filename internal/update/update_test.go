package update

import "testing"

func TestDescribe(t *testing.T) {
	tests := []struct {
		rev      string
		modified bool
		want     string
	}{
		{"", false, "unknown (built without git info)"},
		{"e058f17a98bcae6df486c863536c0e6a77faf925", false, "e058f17"},
		{"e058f17a98bcae6df486c863536c0e6a77faf925", true, "e058f17 + local changes"},
		{"abc", false, "abc"},
	}
	for _, tt := range tests {
		if got := describe(tt.rev, tt.modified); got != tt.want {
			t.Errorf("describe(%q, %v) = %q, want %q", tt.rev, tt.modified, got, tt.want)
		}
	}
}

func TestSourceDirOverride(t *testing.T) {
	t.Setenv("AGENTWATCH_SRC", "/tmp/x")
	if d, err := SourceDir(); err != nil || d != "/tmp/x" {
		t.Errorf("SourceDir() = %q, %v", d, err)
	}
}
