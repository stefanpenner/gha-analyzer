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
		{"=value", true, 0},            // empty key
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

func TestParse_Negation(t *testing.T) {
	f, err := Parse("!service.name=internal")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(f.conditions))
	}
	if !f.conditions[0].negate {
		t.Error("expected negate=true")
	}
	if f.conditions[0].key != "service.name" {
		t.Errorf("expected key 'service.name', got %q", f.conditions[0].key)
	}
	if f.conditions[0].value != "internal" {
		t.Errorf("expected value 'internal', got %q", f.conditions[0].value)
	}
}

func TestParse_BareKey(t *testing.T) {
	f, err := Parse("http.status_code")
	if err != nil {
		t.Fatal(err)
	}
	if f.conditions[0].value != "*" {
		t.Errorf("bare key should have wildcard value, got %q", f.conditions[0].value)
	}
}

func TestErrorsOnly(t *testing.T) {
	f := ErrorsOnly()
	if len(f.conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(f.conditions))
	}
	if f.conditions[0].key != "otel.status_code" {
		t.Errorf("expected key 'otel.status_code', got %q", f.conditions[0].key)
	}
	if f.conditions[0].value != "ERROR" {
		t.Errorf("expected value 'ERROR', got %q", f.conditions[0].value)
	}
}

func TestApply_NilFilter(t *testing.T) {
	var f *Filter
	spans := f.Apply(nil)
	if spans != nil {
		t.Error("expected nil from nil filter")
	}
}

func TestApply_EmptyFilter(t *testing.T) {
	f := &Filter{}
	// Empty filter should pass everything through
	result := f.Apply(nil)
	if result != nil {
		t.Error("expected nil from empty filter on nil input")
	}
}
