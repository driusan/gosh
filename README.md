# gosh Shell

This is an attempt to write a simple UNIX shell using literate programming. We'll
be using Go to write it, because I like Go.

(I intend to formalize the literate programming syntax I'm using with
markdown later, but it should be fairly straight forward. A header immediately
before a code block in quotation marks is a name for that code block. It can be
referenced in other code blocks as `<<<<name>>>>`. A `+=` at the end of the
header means append to the code block, don't replace it. A header without
quotation marks means the code block should be the contents of that filename.)

## What does a simple shell do?

A simple shell needs to do a few things.

1. Read user input.
2. Interpret the user input.
3. Execute the user's input.

And repeat, until the user inputs something like an `exit` command. A good
shell does much more (like tab completion, file globbing, etc) but for now
we'll stick to the absolute basics.

## Main Body

We'll start with the main loop. Most Go programs have a file that looks
something like this:

### main.go
```go
package main

import (
	<<<main.go imports>>>
)

<<<main.go globals>>>

<<<main.go funcs>>>
```

### "main.go funcs"
```go
func main() {
	<<<mainbody>>>
}
```
Our main body is going to be a loop that repeatedly reads from `os.Stdin`.
This means that we should probably start by adding `os` to the import list.

### "main.go imports"
```go
"os"
```

And we'll start with a loop that just repeatedly reads a rune from the user.
`*os.File` (`os.Stdin`'s type) doesn't support the ReadRune interface, but
fortunately the `bufio` package provides a wrapper which allows us to convert
any `io.Reader` into a RuneReader, so let's import that too.

### "main.go imports" +=
```go
"bufio"
```

Then the basis of our main body becomes a loop that initializes and repeatedly
reads a rune from `os.Stdio`. For now, we'll just print it and see how it goes:

### "mainbody"
```go
r := bufio.NewReader(os.Stdin)
for {
	c, _, err := r.ReadRune()
	if err != nil {
		panic(err)
	}
	print(c)
}
```

The problem with this, is that `os.Stdin` sends things to the underlying `io.Reader`
one line at a time, and doesn't provide a way to change this. It turns out
that there's no way in the standard library to force it to send one character
at a time. Luckily for us, the third party package `github.com/pkg/term` *does*
provide a way to manipulate the settings of POSIX ttys, which is all we need
to support. So instead, let's import that so that we can convert the terminal
to raw mode. (We'll still use bufio for the simplicity of ReadRune.)

(We'll actually use CbreakMode(), which is like raw mode in that it'll send
us 1 key at a time, but unlike raw mode in that special command sequences
will still be interpreted.)

### "main.go imports"
```go
"github.com/pkg/term"
"bufio"
```

### "mainbody"
```go
// Initialize the terminal
t, err := term.Open("/dev/tty")
if err != nil {
	panic(err)
}
// Restore the previous terminal settings at the end of the program
defer t.Restore()
t.SetCbreak()
r := bufio.NewReader(t)
for {
	c, _, err := r.ReadRune()
	if err != nil {
		panic(err)
	}
	println(c)
}
```

## The Command Loop

That's a good start, but we probably want to handle the rune that's read in a
way other than just printing it. Let's keep track of what the current command
read is in a string, and when a newline is pressed, call a function to handle
the current command and reset the string. We'll declare a type for commands,
and do a little refactoring, just to be proactive.

### "mainbody"
```go
<<<Initialize Terminal>>>
<<<Command Loop>>>
```

### "main.go globals"
```go
type Command string
```

### "Initialize Terminal"
```go
// Initialize the terminal
t, err := term.Open("/dev/tty")
if err != nil {
	panic(err)
}
// Restore the previous terminal settings at the end of the program
defer t.Restore()
t.SetCbreak()
```

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
			// The terminal doesn't echo in raw mode,
			// so print the newline itself to the terminal.
			fmt.Printf("\n")
			<<<Handle Command>>>
			cmd = ""
		default:
			fmt.Printf("%c", c)
			cmd += Command(c)
	}
}
```

Okay, but there's a problem. Since we're getting sent one character
at a time, when we get the error `exec: "ls\u007f": executable file not found in $PATH`

(0x7F is the ASCII code for "DEL". 0x08 is the code for "Backspace".)

Let's make both of those work as backspace.

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
			// The terminal doesn't echo in raw mode,
			// so print the newline itself to the terminal.
			fmt.Printf("\n")
			<<<Handle Command>>>
			cmd = ""
		case '\u007f', '\u0008':
			<<<Handle Backspace>>>
		default:
			fmt.Printf("%c", c)
			cmd += Command(c)
	}
}
```

How do we handle the backspace key? We want to cut the last
character off cmd, and erase it from the screen. Let's try 
printing '\u0008' and see if that erases the last character
in Cbreak mode:

### "Handle Backspace"
```go
if len(cmd) > 0 {
	cmd = cmd[:len(cmd)-1]
	fmt.Printf("\u0008")
}
```

It moves the cursor, but doesn't actually delete the character. There might be
a more appropriate character to print, but for now we'll just print backspace,
space to overwrite the character, and then backspace again.

### "Handle Backspace"
```go
if len(cmd) > 0 {
	cmd = cmd[:len(cmd)-1]
	fmt.Printf("\u0008 \u0008")
}
```

## Handling the Command
Okay, so how do we handle the command? If it's the string "exit"
we probably should exit. Otherwise, we'll want to execute it using
the [`os.exec`](https://golang.org/pkg/os/exec/) package. We were
proactive about declaring cmd as a type instead of a string, so
we can just define some kind of HandleCmd() method on the type and
call that.

### "Handle Command"
```go
if cmd == "exit" || cmd == "quit" {
	os.Exit(0)
} else {
	cmd.HandleCmd();
}
```

### "main.go funcs" +=
```go
<<<HandleCmd Implementation>>>
```

### "HandleCmd Implementation"
```go
func (c Command) HandleCmd() error {
	cmd := exec.Command(string(c))
	return cmd.Run()
}
```

We'll need to add `os` and `os/exec` to our imports, while we're
at it.

### "main.go imports" +=
```go
"os"
"os/exec"
```

If we run it and try executing something, it doesn't seem to be working.
What's going on? Let's print the error if it happens to find out.

### "Handle Command"
```go
if cmd == "exit" || cmd == "quit" {
	os.Exit(0)
} else {
	err := cmd.HandleCmd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
	}
}
```

### "main.go imports" +=
```go
"fmt"
```

There's still no error (unless we just hit enter without entering anything,
in which case it complains about not being able to execute "". (We should
probably handle that as a special case, too.)) 

So what's going on with the lack of any output or error? It turns out if we look at the `os/exec.Command`
documentation we'll see:

```go
// Stdout and Stderr specify the process's standard output and error.
//
// If either is nil, Run connects the corresponding file descriptor
// to the null device (os.DevNull).
//
// If Stdout and Stderr are the same writer, at most one
// goroutine at a time will call Write.
Stdout io.Writer
Stderr io.Writer
```

`Stdin` makes a similar claim. So let's hook those up to os.Stdin, os.Stdout
and os.Stderr.

While we're at it, let's print a simple prompt.

So our new command handling code is:

### "Handle Command"
```go
if cmd == "exit" || cmd == "quit" {
	os.Exit(0)
} else if cmd == "" {
	PrintPrompt()
} else {
	err := cmd.HandleCmd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
	}
	PrintPrompt()
}
```

We need to define PrintPrompt() that we just used.

### "main.go funcs" +=
```go
func PrintPrompt() {
	<<<PrintPrompt Implementation>>>
}
```

We'll start with a simple implementation that just prints a ">" so that we don't 
get confused and think we're in a POSIX compliant sh prompt, then work on adding
a better prompt later.

# "PrintPrompt Implementation"
```go
fmt.Printf("\n> ")
```


And we'll want to print it on startup too:

### "Initialize Terminal" +=
```go
PrintPrompt()
```

And our new HandleCmd() implementation.

### "HandleCmd Implementation"
```go
func (c Command) HandleCmd() error {
	cmd := exec.Command(string(c))
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
```

Now we can finally run some commands from our shell!

But wait, when we run anything with arguments, we get an
error: `exec: "ls -l": executable file not found in $PATH`.
 exec is trying to run the command named "ls -l", not the
command "ls" with the parameter "-l". 


This is the signature from the GoDoc:

`func Command(name string, arg ...string) *Cmd`

*Not*

`func Command(command string) *Cmd`

We can use `Fields` function in the standard Go [`strings`](https://golang.org/pkg/strings)
package to split a string on any whitespace, which is basically what we want
right now. There will be some problems with that (notably we won't be able 
to enclose arguments in quotation marks if they contain whitespace), but at 
least we won't have to write our own tokenizer.

While we're at it, the "$PATH" in the error message reminds me. If there *are*
any arguments that start with a "$", we should probably expand that to the OS
environment variable.

### "HandleCmd Implementation"
```go
func (c Command) HandleCmd() error {
	parsed := strings.Fields(string(c))
	if len(parsed) == 0 {
		// There was no command, it's not an error, the user just hit
		// enter.
		PrintPrompt()
		return nil
	}

	var args []string
	for _, val := range parsed[1:] {
		if val[0] == '$' {
			args = append(args, os.Getenv(val[1:])
		} else {
			args = append(args, val)
		}
	}
	cmd := exec.Command(parsed[0], args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
```

### "main.go imports" +=
```go
"strings"
```

There's one other command that needs to be implemented internally: `cd`. Each
program in unix contains it's own working directory. When it's spawned, it
inherits its parent's working directory. If `cd` were implemented as an
external program, the new directory would never make it back to the
parent (our shell.) It's fairly easy to change the working directory,
we just add a check after we've parsed the args if the command is "cd", and
call `os.Chdir` instead of `exec.Command` as appropriate.

### "HandleCmd Implementation"
```go
func (c Command) HandleCmd() error {
	parsed := strings.Fields(string(c))
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

### Handling EOFs

There's still one minor noticable bug. If we hit `^D` on an empty line, it should
be treated as an EOF instead of adding the character `0x04`. (And
if it's not an empty line, we probably still shouldn't add it to
the command.)

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
```

And now.. hooray! We have a simple shell that works! We should add tab completion,
a smarter tokenizer, and a lot of other features if we were going to use this
every day, but at least we have a proof-of-concept, and maybe you learned something
by doing it.

If there's any other features you'd like to see added, feel free to either
create a pull request telling the story of how to you'd do it, or just create
an issue on GitHub and see if someone else does. Feel free to also file bug
reports in either the code or prose.

The Tokenization.md file builds on this and improves on the tokenization by
adding support for string literals (with spaces) in arguments.

TabCompletion.md builds on Tokenization.md to add rudimentary command and file
tab completion to the shell.

Piping.md adds support for stdin/stdout redirection and piping processes
together with `|`.

The final result of putting this all together after running `go fmt` is in the
accompanying `*.go` files in this repo, so it should be go gettable.
