package config

import (
	"testing"
	"time"
)

func TestParseIntOrDefault(t *testing.T) {
	tests := []struct {
		input    string
		def      int
		expected int
	}{
		{"5", 10, 5},
		{"100", 0, 100},
		{"-3", 10, -3},
		{"abc", 10, 10},       // invalid returns default
		{"", 10, 10},          // empty returns default
		{"3.14", 10, 3},       // parses integer prefix (3)
		{"7xyz", 10, 7},       // parses prefix
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseIntOrDefault(tt.input, tt.def)
			if result != tt.expected {
				t.Errorf("parseIntOrDefault(%q, %d) = %d; want %d", tt.input, tt.def, result, tt.expected)
			}
		})
	}
}

func TestParseDurationOrDefault(t *testing.T) {
	tests := []struct {
		input    string
		def      time.Duration
		expected time.Duration
	}{
		{"60m", 10 * time.Minute, 60 * time.Minute},
		{"2h", 10 * time.Minute, 2 * time.Hour},
		{"90s", 10 * time.Minute, 90 * time.Second},
		{"1h30m", 10 * time.Minute, 90 * time.Minute},
		{"invalid", 10 * time.Minute, 10 * time.Minute}, // invalid returns default
		{"", 10 * time.Minute, 10 * time.Minute},        // empty returns default
		{"500ms", time.Second, 500 * time.Millisecond},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseDurationOrDefault(tt.input, tt.def)
			if result != tt.expected {
				t.Errorf("parseDurationOrDefault(%q, %v) = %v; want %v", tt.input, tt.def, result, tt.expected)
			}
		})
	}
}
