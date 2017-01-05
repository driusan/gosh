# Piping and Redirection

To be a useful UNIX shell, our shell needs to support piping and
redirection. Since while we were writing our tokenizer we had the
foresight of handling `|`, `<`, and `>` characters, we should only
need to update our HandleCmd implementation.

Recall, our most recent HandleCmd implementation looked like this:

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

Let's refactor the way we markup the sections a little before we go any further
to make it easier to maintain:

### "HandleCmd Implementation"
```go
func (c Command) HandleCmd() error {
	parsed := c.Tokenize()
	<<<Handle no tokens in command case>>>
	<<<Replace environment variables in command>>>
	<<<Handle cd command>>>
	<<<Execute command and return>>>
}
```

### "Handle no tokens in command case"
```go
if len(parsed) == 0 {
	// There was no command, it's not an error, the user just hit
	// enter.
	PrintPrompt()
	return nil
}
```

### "Replace environment variables in command"
```go
var args []string
for _, val := range parsed[1:] {
	if val[0] == '$' {
		args = append(args, os.Getenv(val[1:]))
	} else {
		args = append(args, val)
	}
}
```

### "Handle cd command"
```go
if parsed[0] == "cd" {
	if len(args) == 0 {
		return fmt.Errorf("Must provide an argument to cd")
	}
	return os.Chdir(args[0])
}
```

### "Execute command and return"
```go
cmd := exec.Command(parsed[0], args...)
cmd.Stdin = os.Stdin
cmd.Stdout = os.Stdout
cmd.Stderr = os.Stderr

return cmd.Run()
```

It's only the last part we need to be concerned about to implement pipelining
and redirection. Instead of starting one command, we need to start multiple
commands and hook up their stdin and stdout streams.

Let's start by going through the tokens looking for a `|` token. Each time we 
find one, we'll add all the elements since the last `|` to a slice of [][]string,
with each one representing a command in the pipeline. Then we'll go through
those and find the redirections, and parse those out, then we can create
a slice of []*exec.Cmd that we can create with exec.Command() with the name
and args..

Actually, this is getting too complicated. Let's create a new type instead.
Something like:

### "Parsed Command Type"
```go
type ParsedCommand struct{
	Args []string
	Stdin string
	Stdout string
}
```
### "main.go globals" +=
```go
<<<Parsed Command Type>>>
```

Then we can go through a single pass and created the []ParsedCommand, which
will make it easier to create the []*exec.Cmd without needing to make extra
passes to figure out which tokens are part of the command, which are arguments,
and which are special shell characters like redirection and pipe.

So our execute command becomes something like:

### "Execute command and return"
```go
// Keep track of the current command being built
var currentCmd ParsedCommand
// Keep array of all commands that have been built, so we can create the
// pipeline
var allCommands []ParsedCommand
// Keep track of where this command started in parsed, so that we can build
// currentCommand.Args when we find a special token.
var lastCommandStart = 0
// Keep track of if we've found a special token such as < or >, so that
// we know if currentCmd.Args has already been populated.
var foundSpecial bool
for i, t := parsed {
	if t == "<" || t == ">" || t == "|" {
		if foundSpecial == false {
			currrentCmd.Args = parsed[lastCommandStart] 
		}
		foundSpecial = true
	}
	if t == "|" {
		allCommands = append(allCommands, currentCmd)
		lastCommandStart = i
		foundSpecial = false
	}
}

<<<Build pipeline and execute>>>
```

Our tokens code is probably getting complicated enough that we should have a
Token type with methods like `IsSpecial() bool` and `IsPipe() bool` instead
of a []string to make it more readable, but we'll ignore that for now.

I'm not very confident in the above, either, so let's refactor it into a function
and build another table driven test.

### "Execute command and return"
```go
// Convert parsed from []string to []Token. We should refactor all the code
// to use tokens, but for now just do this instead of going back and changing
// all the references/declarations in every other section of code.
var parsedtokens []Token = []Token{Token(parsed[0])}
for _, t := range args {
	parsedtokens = append(parsedtokens, Token(t))	
}
commands := ParseCommands(parsedtokens)
<<<Build pipeline and execute>>>
```

### "tokenize.go globals" +=
```go
type Token string

func (t Token) IsPipe() bool {
	return t == "|"
}

func (t Token) IsSpecial() bool {
	return t == "<" || t == ">" || t == "|" 
}

func (t Token) IsStdinRedirect() bool {
	return t == "<"
}

func (t Token) IsStdoutRedirect() bool {
	return t == ">"
}
```

### "main.go funcs" +=
```go
func ParseCommands(tokens []Token) []ParsedCommand {
	<<<ParseCommands Implementation>>>
}
```

### "ParseCommands Implementation"
```go
// Keep track of the current command being built
var currentCmd ParsedCommand
// Keep array of all commands that have been built, so we can create the
// pipeline
var allCommands []ParsedCommand
// Keep track of where this command started in parsed, so that we can build
// currentCommand.Args when we find a special token.
var lastCommandStart = 0
// Keep track of if we've found a special token such as < or >, so that
// we know if currentCmd.Args has already been populated.
var foundSpecial bool
for i, t := range tokens {
	if t.IsSpecial() {
		if foundSpecial == false {
			// Convert from Token to string
			var slice []Token
			if i == len(tokens)-1 {
				slice = tokens[lastCommandStart:]
			} else {
				slice = tokens[lastCommandStart:i]
			}

			for _, t := range slice {
				currentCmd.Args = append(currentCmd.Args, string(t))
			}
		}
		foundSpecial = true
	}
	if t.IsPipe() {
		allCommands = append(allCommands, currentCmd)
		lastCommandStart = i
		foundSpecial = false
	}
}
allCommands = append(allCommands, currentCmd)
return allCommands
```

We'll add the tests to the existing tokenize_test, since it's still related
to parsing the command.

### tokenize_test.go +=
```go
func TestParseCommands(t *testing.T) {
	tests := []struct{
		val []Token
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
		<<<Other ParseCommands Test Cases>>>
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
```

Run our tests and..

```sh
--- FAIL: TestParseCommands (0.00s)
	tokenize_test.go:55: Unexpected number of ParsedCommands in test 1. Got [{[]  }] want [{[ls]  } {[cat]  }]
FAIL
FAIL	github.com/driusan/gosh	0.005s

```

It's a good thing we wrote those tests, since it fails on the most basic
case. It turns out we only set command.Args in the IsSpecial loop, which
isn't getting triggered for the last iteration. Let's add an or to check if
it's the last element, and do the same for the pipe statement while we're at it

We also forgot to reset currentCmd after a pipe, so let's take care of that

### "ParseCommands Implementation"
```go
// Keep track of the current command being built
var currentCmd ParsedCommand
// Keep array of all commands that have been built, so we can create the
// pipeline
var allCommands []ParsedCommand
// Keep track of where this command started in parsed, so that we can build
// currentCommand.Args when we find a special token.
var lastCommandStart = 0
// Keep track of if we've found a special token such as < or >, so that
// we know if currentCmd.Args has already been populated.
var foundSpecial bool
for i, t := range tokens {
	if t.IsSpecial() || i == len(tokens)-1 {
		if foundSpecial == false {
			// Convert from Token to string. If it's a pipe, we want
			// to strip the '|' token, if it's the last token, we
			// don't want to strip anything.
			var slice []Token
			if i == len(tokens)-1 {
				slice = tokens[lastCommandStart:]
			} else {
				slice = tokens[lastCommandStart:i]
			}
			for _, t := range slice {
				currentCmd.Args = append(currentCmd.Args, string(t))
			}
		}
		foundSpecial = true
	}
	if t.IsPipe() || i == len(tokens)-1 {
		allCommands = append(allCommands, currentCmd)
		lastCommandStart = i+1
		foundSpecial = false
		currentCmd = ParsedCommand{}
	}
}
return allCommands
```

and now our tests pass. Let's take care of `<` and `>` while we're
in this code. We'll just do it by adding some extra booleans to
track if the next token is for redirecting Stdin or Stdout.

### "ParseCommands Implementation"
```go
// Keep track of the current command being built
var currentCmd ParsedCommand
// Keep array of all commands that have been built, so we can create the
// pipeline
var allCommands []ParsedCommand
// Keep track of where this command started in parsed, so that we can build
// currentCommand.Args when we find a special token.
var lastCommandStart = 0
// Keep track of if we've found a special token such as < or >, so that
// we know if currentCmd.Args has already been populated.
var foundSpecial bool
var nextStdin, nextStdout bool
for i, t := range tokens {
	if nextStdin {
		currentCmd.Stdin = string(t)
		nextStdin = false
	}
	if nextStdout {
		currentCmd.Stdout = string(t)
		nextStdout = false
	}
	if t.IsSpecial() || i == len(tokens)-1 {
		if foundSpecial == false {
			// Convert from Token to string
			var slice []Token
			if i == len(tokens)-1 {
				slice = tokens[lastCommandStart:]
			} else {
				slice = tokens[lastCommandStart:i]
			}

			for _, t := range slice {
				currentCmd.Args = append(currentCmd.Args, string(t))
			}
		}
		foundSpecial = true
	}
	if t.IsStdinRedirect() {
		nextStdin = true
	}
	if t.IsStdoutRedirect() {
		nextStdout = true
	}
	if t.IsPipe() || i == len(tokens)-1 {
		allCommands = append(allCommands, currentCmd)
		lastCommandStart = i+1
		foundSpecial = false
		currentCmd = ParsedCommand{}
	}
}
return allCommands
```

We'll add some test cases to be sure:

### "Other ParseCommands Test Cases"
```go
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
```

And the tests still pass, so we seem to be alright.

That leaves us with building the commands, connecting their
stdin and stdout pipes, and then running the whole thing.

### Building the pipeline

We used to do this:

```go
cmd := exec.Command(parsed[0], args...)
cmd.Stdin = os.Stdin
cmd.Stdout = os.Stdout
cmd.Stderr = os.Stderr

return cmd.Run()
```

which was nice and simple for a single command. Now we need to,
at a minimum, range over all the commands and create different processes
for each one, then set up their stdin and stdout pipes.

### "Build pipeline and execute"
```go
var cmds []*exec.Cmd
for i, c := range commands {
	if len(c.Args) == 0 {
		// This should have never happened, there is
		// no command, but let's avoid panicing.
		continue
	}
	newCmd := exec.Command(c.Args[0], c.Args[1:]...)
	newCmd.Stderr = os.Stderr
	cmds = append(cmds, newCmd)

	<<<Hookup stdin and stdout pipes>>>
}

<<<Start Processes and Wait>>>
```

How do we hookup the pipes? If there was a `<` or `>` redirect, it's easy, we
just open the file and replace the newCmd.Stdin/Stdout, overwriting the os.Stdin
that we just set it to. Otherwise, we can use os variant we just set it up to.

### "Hookup stdin and stdout pipes"
```go
<<<Hookup STDIN>>>
<<<Hookup STDOUT>>>
```

### "Hookup STDIN"
```go
// If there was an Stdin specified, use it.
if c.Stdin != "" {
	// Open the file to convert it to an io.Reader
	if f, err := os.Open(c.Stdin); err == nil {
		newCmd.Stdin = f
		defer f.Close()
	}
} else {
	// There was no Stdin specified, so 
	// connect it to the previous process in the
	// pipeline if there is one, the first process
	// still uses os.Stdin
	if i > 0 {
		pipe, err := cmds[i-1].StdoutPipe()
		if err != nil {
			continue
		}
		newCmd.Stdin = pipe
	} else {
		newCmd.Stdin = os.Stdin
	}
}
```

### "Hookup STDOUT"
```go
// If there was a Stdout specified, use it.
if c.Stdout != "" {
	// Create the file to convert it to an io.Reader
	if f, err := os.Create(c.Stdout); err == nil {
		newCmd.Stdout = f
		defer f.Close()
	}
} else {
	// There was no Stdout specified, so 
	// connect it to the previous process in the
	// unless it's the last command in the pipeline,
	// which still uses os.Stdout
	if i == len(commands)-1  {
		newCmd.Stdout = os.Stdout
	}
}
```

### "Start Processes and Wait"
```go
for _, c := range cmds {
	c.Start()
}
return cmds[len(cmds)-1].Wait()
```
