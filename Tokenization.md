# Input Tokenization

At the end of our basic shell, we said we needed better tokenization
support. Recall, we're currently just calling `strings.Fields(cmd)` to
split the command into strings on whitespace.

This means that if we enter, for instance, `git commit -m 'I am a message'`,
it gets split into the slice `[]string{"git", "commit", "-m", "'I", "am", "a",
"message'"}`, when what we probably intended was `[]string{"git", "commit",
"-m", "I am a message"}`

Splitting on whitespace was easy, because there was no thinking or design to
do. More complex tokenization is.. more complex. Do we want a POSIX compliant
sh shell? Do we want something more similar to csh syntax? Do we want to go with
a syntax more similar to Plan 9's rc shell? Or do we want to come up with
something on our own?

Let's start with something simple. We'll just add the ability to use `'` to
declare a string literal until the next time we see a `'` (Unless there's a
`\` before the `'`, in which case we'll escape it and include a `'` in the
literal.) We'll also want to add pipeline support eventually, so if we see a `|`
outside of a string literal, we'll use that as a delimiter too.

We should be able to do all this fairly easily on a single pass of cmd.

## Tests

I hate parsing, and I'm never confident I'm doing it properly (I'm usually not),
so we'll start with some table driven tests of the basic use cases.

### tokenize_test.go
```go
package main

import (
	"testing"
	<<<other tokenize_test.go imports>>>
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
```

We also need to define Command.Tokenize(). We'll start with a little refactoring
for an implementation that just uses `strings.Fields` (and expect it to fail)
the tests that we just wrote.

### "HandleCmd Implementation"
```go
func (c Command) HandleCmd() error {
	parsed := c.Tokenize()
	if len(parsed) == 0 {
		// There was no command, it's not an error, the user just hit
		// enter.
		PrintPrompt()
		return nil
	}

	var args []string
	for _, val := range parsed[1:] {
		if val[0] == '$' {
			args = append(args, os.Getenv(val[1:]))
		} else {
			args = append(args, val)
		}
	}

	if parsed[0] == "cd" {
		if len(args) == 0 {
			return fmt.Errorf("Must provide an argument to cd")
		}
		return os.Chdir(args[0])

	}

	cmd := exec.Command(parsed[0], args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
```

(It looks like we did a lot there, but it's just the implementation
from the end of the README.md with `parsed := strings.Fields(string(c))`
replaced with `parsed := c.Tokenize()` on the first line.)

We'll start moving our new code into another file to keep it cleaner,
which is going to have a similar structure to main.go.

### tokenize.go
```go
package main

import (
	<<<tokenize.go imports>>>
)

<<<tokenize.go globals>>>
```

### "tokenize.go globals"
```
func (c Command) Tokenize() []string {
	<<<Tokenize Implementation>>>
}
```

Finally, the implementation.

### "Tokenize Implementation"
```go
return strings.Fields(string(c))
```

### "tokenize.go imports" +=
```go
"strings"
```

Some refactoring overhead: we'll have to remove "strings" from our 
main.go imports, otherwise Go will refuse to compile. Our list ends
up being:

### "main.go imports"
```go
"bufio"
"fmt"
"github.com/pkg/term"
"os"
"os/exec"
```

And now if we run our test...

```
> go test ./...
--- FAIL: TestTokenization (0.00s)
	tokenize_test.go:22: Mismatch for result length in test case 3. Got 6 want 4
FAIL
FAIL	github.com/driusan/gosh	0.006s
exit status 1
```

Hooray! It failed on the 4th test case as expected! (We just printed the index
in the error, which is zero indexed.)

Now that we've done our refactoring and written our tests, we can worry about
our implementation.

## Tokenization, for real.

So how are we going to actually implement this? We'll need to iterate
through our command string, and keep track of some things every time we see
a `'`. Are we in a string? If so, this is the start of one. If not, it's the
end of one unless the previous character is a `\`. If we're in a string, we 
don't care about anything other than where the end of the string is. If we're
not, we care about: is this a whitespace character? If so, we want to ignore it,
(or append the previous token to the parsed arguments if the previous character
wasn't whitespace.)

We can use `unicode.IsWhitespace()` for the whitespace checking, and a token
start int to keep track of where the start of the current token is so that
we can append to the parsed args slice when we get to the end of one.

### "Tokenize Implementation"
```go
var parsed []string
tokenStart := -1
inStringLiteral := false
for i, chr := range c {
	switch chr {
		case '\'':
			<<<Handle Quote>>>
		default:
			<<<Handle Nonquote>>>
	}
}
return parsed
```

We'll start with the handling of the character `'`. We need to know
if it's the start or end of our string literal. If it's the start,
mark it, and if it's the end, add it to the parsed tokens.

### "Handle Quote"
```go
if inStringLiteral {
	if i > 0 && c[i-1] == '\\' {
		// The quote was escaped, so ignore it.
		continue
	}
	inStringLiteral = false

	token := string(c[tokenStart:i])

	// Replace escaped quotes with just a single ' before appending
	token = strings.Replace(token, `\'`, "'", -1)
	parsed = append(parsed, token)

	// Now that we've finished, reset the tokenStart for the next token.
	tokenStart = -1
} else {
	// This is the quote, which means the literal starts at the next
	// character
	tokenStart = i+1
	inStringLiteral = true
}
```

Now, for handling non-quotation characters. If we're in the middle of a string
literal, we just want to ignore it and let it be taken care of above. Otherwise,
if it's a whitespace, we end the current token and add it it to the parsed
arguments. if it's a whitespace character, we've either reached the end of a 
token and should add it to the parsed arguments, or we're not in a token and
should just ignore it.

### "Handle Nonquote"
```go
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
```

### "tokenize.go imports" +=
```go
"unicode"
```

And when we run our tests... 

```
> go test ./... 
--- FAIL: TestTokenization (0.00s)
	tokenize_test.go:22: Mismatch for result length in test case 0. Got 0 want 1
FAIL
FAIL	github.com/driusan/gosh	0.005s

```

It now fails on the first test. I told you I always got parsing wrong, so it's
a good thing we wrote those tets. It got 0 results and expected 1, suggesting
we're doing something wrong with the last (or first) token.

In fact, there's no whitespace at the end of the last token, and we forgot to
take care of that. So after the loop, let's check if tokenStart is >= 0 and
add the final token if so.

### "Tokenize Implementation"
```go
var parsed []string
tokenStart := -1
inStringLiteral := false
for i, chr := range c {
	switch chr {
		case '\'':
			<<<Handle Quote>>>
		default:
			<<<Handle Nonquote>>>
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
```

Now when we run `go test ./...` we get to the last `ls|cat` test, which isn't
surprising since we didn't implement '|' as a delimiter.

```
> go test            
--- FAIL: TestTokenization (0.00s)
	tokenize_test.go:22: Mismatch for result length in test case 4. Got '1' want '3'
FAIL
exit status 1
FAIL	github.com/driusan/gosh	0.005s
```

Let's add it to our switch statement, and do a little refactoring of our
of our Tokenize implementation into smaller semantic chunks while we're at it. 

### "Tokenize Implementation"
```go
<<<Tokenize Globals>>>
for i, chr := range c {
	<<<Handle Tokenize Chr>>>
}
<<<Add Last Token>>>
```

### "Tokenize Globals"
```go
var parsed []string
tokenStart := -1
inStringLiteral := false
```

### "Handle Tokenize Chr"
```go
switch chr {
	case '\'':
		<<<Handle Quote>>>
	case '|':
		<<<Handle Pipe Chr>>>
	default:
		<<<Handle Nonquote>>>
}
```

### "Add Last Token"
```go
if tokenStart >= 0 {
	if inStringLiteral {
		// Ignore the ' character
		tokenStart += 1
	}
	parsed = append(parsed, string(c[tokenStart:]))
}
return parsed
```

The logic of Handle Pipe Chr is going to be pretty straight forward. If we're
in a string literal, don't. Otherwise, add any existing token and add it
as well as a `|` token.

### "Handle Pipe Chr"
```go
if inStringLiteral {
	continue
}

if tokenStart >= 0 {
	parsed = append(parsed, string(c[tokenStart:i]))
} else {
	parsed = append(parsed, string(c[:i]))
}
parsed = append(parsed, "|")
tokenStart = -1
```

And now...

```
> go test
PASS
ok  	github.com/driusan/gosh	0.005s
```

We can pass arguments to programs with strings!

There's probably better ways to do this (like using the
[`text/scanner`](https://golang.org/pkg/text/scanner/) package and
we may want to revisit later, but for now this works.

We also never used the `other tokenize_test.go imports` macro, so let's define
an empty one to avoid compilation errors:

### "other tokenize_test.go imports"
```go
```

