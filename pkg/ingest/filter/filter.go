// Package filter provides attribute-based span filtering.
package filter

import (
	"fmt"
	"strings"

	"github.com/stefanpenner/otel-analyzer/pkg/utils"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Filter represents a set of conditions that spans must match.
type Filter struct {
	conditions []condition
}

type condition struct {
	key      string
	value    string
	negate   bool
}

// Parse parses filter expressions like "service.name=checkout" or "http.status_code=5*".
// Multiple conditions are separated by commas. Prefix with "!" to negate.
func Parse(expr string) (*Filter, error) {
	if expr == "" {
		return nil, nil
	}

	f := &Filter{}
	parts := strings.Split(expr, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		negate := false
		if strings.HasPrefix(part, "!") {
			negate = true
			part = part[1:]
		}

		eqIdx := strings.Index(part, "=")
		if eqIdx < 0 {
			// Bare key means "attribute exists"
			f.conditions = append(f.conditions, condition{key: part, value: "*", negate: negate})
			continue
		}

		key := part[:eqIdx]
		value := part[eqIdx+1:]
		if key == "" {
			return nil, fmt.Errorf("empty key in filter expression: %s", part)
		}

		f.conditions = append(f.conditions, condition{key: key, value: value, negate: negate})
	}

	return f, nil
}

// Apply filters a slice of spans, returning only those that match all conditions.
func (f *Filter) Apply(spans []sdktrace.ReadOnlySpan) []sdktrace.ReadOnlySpan {
	if f == nil || len(f.conditions) == 0 {
		return spans
	}

	var result []sdktrace.ReadOnlySpan
	for _, s := range spans {
		if f.matches(s) {
			result = append(result, s)
		}
	}
	return result
}

func (f *Filter) matches(s sdktrace.ReadOnlySpan) bool {
	// Build attribute map
	attrs := make(map[string]string)
	for _, a := range s.Attributes() {
		attrs[string(a.Key)] = a.Value.AsString()
	}

	// Also check resource attributes
	if s.Resource() != nil {
		for _, a := range s.Resource().Attributes() {
			attrs[string(a.Key)] = a.Value.AsString()
		}
	}

	// Check span-level properties
	attrs["otel.span_name"] = s.Name()
	attrs["otel.status_code"] = s.Status().Code.String()

	for _, cond := range f.conditions {
		val, exists := attrs[cond.key]
		matched := exists && utils.GlobMatch(cond.value, val)
		if cond.negate {
			if matched {
				return false
			}
		} else {
			if !matched {
				return false
			}
		}
	}
	return true
}

// ErrorsOnly returns a filter that only passes spans with ERROR status.
func ErrorsOnly() *Filter {
	return &Filter{
		conditions: []condition{
			{key: "otel.status_code", value: "ERROR"},
		},
	}
}

