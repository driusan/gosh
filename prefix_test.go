package main

import (
	"testing"
)

func TestLongestPrefix(t *testing.T) {
	cases := []struct {
		Val      []string
		Expected string
	}{
		// Empty case or nil slice
		{nil, ""},
		{[]string{}, ""},

		// Prefix of 1 element is itself
		{[]string{"a"}, "a"},

		// 2 elements with no prefix
		{[]string{"a", "b"}, ""},

		// 2 elements with a common prefix
		{[]string{"aa", "ab"}, "a"},

		// multiple elements
		{[]string{"aaaa", "aabb", "aaac"}, "aa"},
	}
	for i, tc := range cases {
		if got := LongestPrefix(tc.Val); got != tc.Expected {
			t.Errorf("Unexpected prefix for case %d: got %v want %v", i, got, tc.Expected)
		}
	}
}
