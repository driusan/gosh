package main

import (
	"testing"
)

func TestTokenization(t *testing.T) {
	tests := []struct {
		cmd      Command
		expected []string
	}{
		{cmd: "ls", expected: []string{"ls"}},
		{"     ls    	", []string{"ls"}},
		{"ls -l", []string{"ls", "-l"}},
		{"git commit -m 'I am message'", []string{"git", "commit", "-m", "I am message"}},
		{"git commit -m 'I\\'m another message'", []string{"git", "commit", "-m", "I'm another message"}},
		{"ls|cat", []string{"ls", "|", "cat"}},
	}
	for i, tc := range tests {
		val := tc.cmd.Tokenize()
		if len(val) != len(tc.expected) {
			// The below loop might panic if the lengths aren't equal, so this is fatal instead of an error.
			t.Fatalf("Mismatch for result length in test case %d. Got '%v' want '%v'", i, len(val), len(tc.expected))
		}
		for j, token := range val {
			if token != tc.expected[j] {
				t.Errorf("Mismatch for index %d in test case %d. Got '%v' want '%v'", j, i, token, tc.expected[j])
			}
		}
	}
}
func TestParseCommands(t *testing.T) {
	tests := []struct {
		val      []Token
		expected []ParsedCommand
	}{
		{
			[]Token{"ls"},
			[]ParsedCommand{
				ParsedCommand{[]string{"ls"}, "", ""},
			},
		},
		{
			[]Token{"ls", "|", "cat"},
			[]ParsedCommand{
				ParsedCommand{[]string{"ls"}, "", ""},
				ParsedCommand{[]string{"cat"}, "", ""},
			},
		},
		{
			[]Token{"ls", ">", "cat"},
			[]ParsedCommand{
				ParsedCommand{[]string{"ls"}, "", "cat"},
			},
		},
		{
			[]Token{"ls", "<", "cat"},
			[]ParsedCommand{
				ParsedCommand{[]string{"ls"}, "cat", ""},
			},
		},
		{
			[]Token{"ls", ">", "foo", "<", "bar", "|", "cat", "hello", ">", "x", "|", "tee"},
			[]ParsedCommand{
				ParsedCommand{[]string{"ls"}, "bar", "foo"},
				ParsedCommand{[]string{"cat", "hello"}, "", "x"},
				ParsedCommand{[]string{"tee"}, "", ""},
			},
		},
	}

	for i, tc := range tests {
		val := ParseCommands(tc.val)
		if len(val) != len(tc.expected) {
			t.Fatalf("Unexpected number of ParsedCommands in test %d. Got %v want %v", i, val, tc.expected)
		}
		for j, _ := range val {
			if val[j].Stdin != tc.expected[j].Stdin {
				t.Fatalf("Mismatch for test %d Stdin. Got %v want %v", i, val[j].Stdin, tc.expected[j].Stdin)
			}
			if val[j].Stdout != tc.expected[j].Stdout {
				t.Fatalf("Mismatch for test %d Stdout. Got %v want %v", i, val[j].Stdout, tc.expected[j].Stdout)
			}
			for k, _ := range val[j].Args {
				if val[j].Args[k] != tc.expected[j].Args[k] {
					t.Fatalf("Mismatch for test %d. Got %v want %v", i, val[j].Args[k], tc.expected[j].Args[k])
				}
			}
		}
	}
}
