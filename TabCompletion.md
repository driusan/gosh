# Tab Completion

We can't have a modern shell without tab completion. Tab completion
is what separates us from the dumb terminals. A shell with good
tab completion adds some form of context-sensitive, easy, and 
customizable tab completion.

We'll want, at a minimum, to support autocompletion of commands
(for the first command) and filenames (for any other command.) Maybe
in addition to that, we can add support for regular expressions
that allow the user to add to the suggestions.

Let's start with the first two.

# Command Completion

We need to start actually adding support for the '\t' character in
our command loop. Recall that by the end of our basic implementation,
it looked like this: 

### "Command Loop"
r := bufio.NewReader(t)
var cmd Command
for {
	c, _, err := r.ReadRune()
	if err != nil {
		panic(err)
	}
	switch c {
		case '\n':
			// The terminal doesn't echo in raw mode,
			// so print the newline itself to the terminal.
			fmt.Printf("\n")
			<<<Handle Command>>>
			cmd = ""
		case '\u0004':
			if len(cmd) == 0 {
				os.Exit(0)
			}
		case '\u007f', '\u0008':
			<<<Handle Backspace>>>
		default:
			fmt.Printf("%c", c)
			cmd += Command(c)
	}
}

We'll start by adding a '\t' case, and for now just assume that
we'll implement a method called "Complete()" on the `Command` class.
If it returns an error, we should print it. (In fact, let's do it
in the non-zero case of '^D' too.

### "Command Loop"
```go
r := bufio.NewReader(t)
var cmd Command
for {
	c, _, err := r.ReadRune()
	if err != nil {
		panic(err)
	}
	switch c {
		case '\n':
			<<<Handle Command>>>
			cmd = ""
		case '\u0004':
			if len(cmd) == 0 {
				os.Exit(0)
			}
			err := cmd.Complete()
			if err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
			}

		case '\u007f', '\u0008':
			<<<Handle Backspace>>>
		case '\t':
			err := cmd.Complete()
			if err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
			}
		default:
			cmd += Command(c)
	}
}
```

We'll need to define Command.Complete() to make sure we don't have a compile
error, too. We'll put it in a new file.

### completion.go
```go
package main

import (
	<<<completion.go imports>>>
)

func (c *Command) Complete() error {
	<<<AutoCompletion Implementation>>>
}

<<<other completion.go globals>>>
```

For now, we'll start with an implementation that just does nothing but return
no error.

### "AutoCompletion Implementation"
```go
return nil
```

Okay, now that it compiles, what do we *actually* need to do for tab
completion?

We should probably start by tokenizing the command, so we know if we're
completing a command or a file.

### "AutoCompletion Implementation"
```go
tokens := c.Tokenize()
var suggestions []string
var base string
switch len(tokens) {
case 0:
	base = ""
	suggestions = CommandSuggestions(base)
case 1:
	base = tokens[0]
	suggestions = CommandSuggestions(tokens[0])
default:
	base = tokens[len(tokens)-1]
	suggestions = FileSuggestions(base)
}
```

What we do with the suggestions that we get back (don't worry,
we'll define the functions we just called in a bit) is going
to depend on the number of suggestions. If there's none, we do
nothing (except maybe print a `BEL` character as a warning).
If there's one, we'll use it, and if there's more than one, we'll
print them.

(We also still need to return something.)
### "AutoCompletion Implementation" +=
```go
switch len(suggestions) {
case 0:
	fmt.Printf("\u0007")
case 1:
	<<<Complete Suggestion>>>
default:
	<<<Display Suggestions>>>
}
return nil
```

So, we said we'd define the functions we just used:

### "other completion.go globals"
```go
func CommandSuggestions(base string) []string {
	<<<Command Suggestions Implementation>>>
}

func FileSuggestions(base string) []string {
	<<<File Suggestions Implementation>>>
}
```

## Command Suggestions

Let's start with the command suggestions. We'll need to get the $PATH
variable, parse it, get a list of the contents of each directory, and
then go through them and see if any have "base" as a prefix. Ideally
we would cache the results, but for now we'll just redo it every time
and see if the performance is unusable.

### "Command Suggestions Implementation"
```go
paths := strings.Split(os.Getenv("PATH"), ":")
var matches []string
for _, path := range paths {
	<<<Check For Command Completion in path>>>
}
return matches
```

How are we going to look into the paths? We can use `io/ioutil.ReadDir` to get
a list of files, so let's add that (and the things we just used) to the import
list.

### "completion.go imports" +=
"os"
"strings"
"io/ioutil"
```

### "Check For Command Completion in path"
```go
// We don't care if there's an invalid path in $PATH, so ignore
// the error.
files, _ := ioutil.ReadDir(path)
for _, file := range files {
	if name := file.Name(); strings.HasPrefix(name, base) {
		matches = append(matches, name)
	}
}
```

For now, we'll just leave a stub for filename suggestions, so that things
compile.
### "File Suggestions Implementation"
```go
return nil
```

But before we test it, we'll at least want to declare Display Suggestions
and Complete Suggestions

Display is easy.

### "Display Suggestions"
```go
fmt.Printf("%v", suggestions)
```

For completion, we need to delete the last token, and then add the new
suggestion. Since the Command is a type of string, not a type of []string,
we need to mimick the tokenization. We can strip the trailing whitespace,
then strip the last token as a suffix, then add the new autocompleted command.

### "Complete Suggestion"
```go
suggest := suggestions[0]
*c = Command(strings.TrimSpace(string(*c)))
*c = Command(strings.TrimSuffix(string(*c), base))
*c += Command(suggest)
```

We should also print the remaining part of the token that we just
completed. Since we don't know if the autocomplete suggestions screwed up
the cursor, we'll just print a new prompt with the completed command for now.
We should make this smarter later.

### "Complete Suggestion" +=
```go
PrintPrompt()
fmt.Printf("%s", *c)
```

Finally, we should implement the file name completion we said we were going
to do for other parameters.

To do that, we'll use the standard `path/filepath` package.

### "completion.go imports" +=
```go
"path/filepath
```

We'll try to just call filepath.Dir() on the token, and then use the same
ReadDir() method we used above to try and find any files that match
filepath.Base()

### "File Suggestions Implementation"
filedir := filepath.Dir(base)
fileprefix := filepath.Base(base)
files, err := ioutil.ReadDir(filedir)
if err != nil {
	return nil
}
var matches []string
for _, file := range files {
	if name := file.Name(); strings.HasPrefix(name, fileprefix) {
		matches = append(matches, filedir + "/" + name)
	}
}
return matches


