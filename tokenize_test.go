package main

import (
	"testing"
)

func TestTokenization(t *testing.T) {
	tests := []struct {
		cmd      Command
		expected []string
	}{
		{"ls", []string{"ls"}},
		{"     ls    	", []string{"ls"}},
		{"ls -l", []string{"ls", "-l"}},
		{"git commit -m 'I am message'", []string{"git", "commit", "-m", "I am message"}},
		{"ls|cat", []string{"ls", "|", "cat"}},
	}
	for i, tc := range tests {
		val := tc.cmd.Tokenize()
		if len(val) != len(tc.expected) {
			// The below loop might panic if the lengths aren't equal, so this is fatal instead of an error.
			t.Fatalf("Mismatch for result length in test case %d. Got '%v' want '%d'", i, len(val), len(tc.expected))
		}
		for j, token := range val {
			if token != tc.expected[j] {
				t.Errorf("Mismatch for index %d in test case %d. Got '%v' want '%v'", j, i, token, tc.expected[j])
			}
		}
	}
}
