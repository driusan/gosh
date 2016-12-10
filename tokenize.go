package main

import (
	"strings"
	"unicode"
)

func (c Command) Tokenize() []string {
	var parsed []string
	tokenStart := -1
	inStringLiteral := false
	for i, chr := range c {
		switch chr {
		case '\'':
			if inStringLiteral {
				if i > 0 && c[i-1] == '\\' {
					// The quote was escaped, so ignore it.
					continue
				}
				inStringLiteral = false

				// i is the `'`, which means the previous character was the end of the
				// token
				token := string(c[tokenStart:i])

				// Replace escaped quotes with just a single ' before appending
				token = strings.Replace(token, `\'`, "'", -1)
				parsed = append(parsed, token)

				// Now that we've finished, reset the tokenStart for the next token.
				tokenStart = -1
			} else {
				// This is the quote, which means the literal starts at the next
				// character
				tokenStart = i + 1
				inStringLiteral = true
			}
		case '|':
			if inStringLiteral {
				continue
			}
			if tokenStart >= 0 {
				parsed = append(parsed, string(c[tokenStart:i]))
			}
			parsed = append(parsed, "|")
			tokenStart = -1
		default:
			if inStringLiteral {
				continue
			}
			if unicode.IsSpace(chr) {
				if tokenStart == -1 {
					continue
				}
				parsed = append(parsed, string(c[tokenStart:i]))
				tokenStart = -1
			} else if tokenStart == -1 {
				tokenStart = i
			}
		}
	}

	if tokenStart >= 0 {
		if inStringLiteral {
			// Ignore the ' character
			tokenStart += 1
		}
		parsed = append(parsed, string(c[tokenStart:]))
	}

	return parsed
}
