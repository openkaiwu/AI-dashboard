package main

import "testing"

func TestNormalizedCollectionInterval(t *testing.T) {
	tests := []struct {
		name     string
		input    int
		expected int
	}{
		{name: "unset uses one minute", input: 0, expected: 60},
		{name: "old five minute default migrates to one minute", input: 300, expected: 60},
		{name: "one minute is accepted", input: 60, expected: 60},
		{name: "short interval is clamped", input: 30, expected: 60},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := normalizedCollectionInterval(test.input); got != test.expected {
				t.Fatalf("normalizedCollectionInterval(%d) = %d, want %d", test.input, got, test.expected)
			}
		})
	}
}
