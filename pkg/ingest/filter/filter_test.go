package filter

import (
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		expr    string
		wantErr bool
		count   int
	}{
		{"", false, 0},
		{"service.name=checkout", false, 1},
		{"service.name=checkout,http.status_code=5*", false, 2},
		{"!service.name=internal", false, 1},
		{"http.status_code", false, 1}, // bare key = exists check
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			f, err := Parse(tt.expr)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.expr == "" {
				if f != nil {
					t.Error("expected nil filter for empty expr")
				}
				return
			}
			if len(f.conditions) != tt.count {
				t.Errorf("expected %d conditions, got %d", tt.count, len(f.conditions))
			}
		})
	}
}

func TestGlobMatch(t *testing.T) {
	tests := []struct {
		pattern string
		value   string
		want    bool
	}{
		{"*", "anything", true},
		{"exact", "exact", true},
		{"exact", "other", false},
		{"prefix*", "prefix-foo", true},
		{"prefix*", "other", false},
		{"*suffix", "foo-suffix", true},
		{"*suffix", "other", false},
		{"5*", "503", true},
		{"5*", "200", false},
	}

	for _, tt := range tests {
		got := globMatch(tt.pattern, tt.value)
		if got != tt.want {
			t.Errorf("globMatch(%q, %q) = %v, want %v", tt.pattern, tt.value, got, tt.want)
		}
	}
}
