package enrichment

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RuleEnricher applies user-defined rules to classify spans.
// Rules are loaded from a YAML/JSON config file.
type RuleEnricher struct {
	Rules []Rule
}

// Rule defines a pattern-based enrichment rule.
type Rule struct {
	Name  string            `json:"name"`
	Match RuleMatch         `json:"match"`
	Hints RuleHints         `json:"hints"`
}

// RuleMatch specifies conditions for a rule to apply.
type RuleMatch struct {
	// Attributes maps attribute keys to glob-like patterns.
	// A rule matches if ALL specified attributes match.
	// Patterns: "*" matches anything, "prefix*" matches prefix, "*suffix" matches suffix.
	Attributes map[string]string `json:"attributes"`
	// SpanName is an optional span name glob pattern.
	SpanName string `json:"span_name"`
}

// RuleHints specifies the SpanHints to apply when a rule matches.
type RuleHints struct {
	Category string `json:"category"`
	Icon     string `json:"icon"`
	Color    string `json:"color"`
	BarChar  string `json:"bar_char"`
	Outcome  string `json:"outcome"`
	IsRoot   bool   `json:"is_root"`
	IsLeaf   bool   `json:"is_leaf"`
	IsMarker bool   `json:"is_marker"`
}

// LoadRules loads enrichment rules from a JSON file.
func LoadRules(path string) (*RuleEnricher, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading rules file: %w", err)
	}

	ext := strings.ToLower(filepath.Ext(path))

	var config struct {
		Enrichers []Rule `json:"enrichers"`
	}

	switch ext {
	case ".json":
		if err := json.Unmarshal(data, &config); err != nil {
			return nil, fmt.Errorf("parsing rules JSON: %w", err)
		}
	default:
		// Try JSON first for any extension
		if err := json.Unmarshal(data, &config); err != nil {
			return nil, fmt.Errorf("parsing rules file (only JSON supported): %w", err)
		}
	}

	return &RuleEnricher{Rules: config.Enrichers}, nil
}

// Enrich applies rules in order; first matching rule wins.
func (e *RuleEnricher) Enrich(name string, attrs map[string]string, isZeroDuration bool) SpanHints {
	for _, rule := range e.Rules {
		if matchesRule(rule.Match, name, attrs) {
			h := SpanHints{
				Category: rule.Hints.Category,
				Icon:     rule.Hints.Icon,
				Color:    rule.Hints.Color,
				BarChar:  rule.Hints.BarChar,
				Outcome:  rule.Hints.Outcome,
				IsRoot:   rule.Hints.IsRoot,
				IsLeaf:   rule.Hints.IsLeaf,
				IsMarker: rule.Hints.IsMarker,
			}
			if h.BarChar == "" {
				h.BarChar = "█"
			}
			if h.Icon == "" {
				h.Icon = "● "
			}
			return h
		}
	}
	return SpanHints{}
}

// matchesRule checks if a span matches a rule's conditions.
func matchesRule(match RuleMatch, name string, attrs map[string]string) bool {
	// Check span name pattern
	if match.SpanName != "" && !globMatch(match.SpanName, name) {
		return false
	}

	// Check all attribute patterns
	for key, pattern := range match.Attributes {
		val, ok := attrs[key]
		if !ok {
			// Attribute not present — "*" matches absence as "present with any value"
			if pattern == "*" {
				return false
			}
			return false
		}
		if !globMatch(pattern, val) {
			return false
		}
	}

	// At least one condition must be specified
	return match.SpanName != "" || len(match.Attributes) > 0
}

// globMatch performs simple glob matching: "*" matches everything,
// "prefix*" matches prefix, "*suffix" matches suffix, exact match otherwise.
func globMatch(pattern, value string) bool {
	if pattern == "*" {
		return true
	}
	if strings.HasPrefix(pattern, "*") && strings.HasSuffix(pattern, "*") {
		return strings.Contains(value, pattern[1:len(pattern)-1])
	}
	if strings.HasPrefix(pattern, "*") {
		return strings.HasSuffix(value, pattern[1:])
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(value, pattern[:len(pattern)-1])
	}
	return pattern == value
}
