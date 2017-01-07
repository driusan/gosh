package main

import ()

func LongestPrefix(strs []string) string {
	if len(strs) == 0 {
		return ""
	}

	prefix := strs[0]
	for _, cmp := range strs[1:] {
		for i := range prefix {
			if i > len(cmp) || prefix[i] != cmp[i] {
				prefix = cmp[:i]
				break
			}
		}
	}
	return prefix
}
