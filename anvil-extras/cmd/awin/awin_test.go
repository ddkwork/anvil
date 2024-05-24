package main

import "testing"

func TestPromptOrLastFullLine(t *testing.T) {
	type test struct {
		input, output string
	}

	tests := []test{
		{"", ""},
		{"a", "a"},
		{"abc", "abc"},
		{"abc\n", "abc\n"},
		{"abc\nx", "x"},
		{"abc\nxyz", "xyz"},
		{"abc\nxyz\n", "xyz\n"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			output := promptOrLastFullLine(tc.input)
			if output != tc.output {
				t.Fatalf("For '%s', expected '%s' does not match actual '%s'", tc.input, tc.output, output)
			}
		})
	}
}
