# Prompts

We've went quite far with our trusty ">" prompt, but now we've done enough that
we might want to start using this as our login shell.

We'd like customizable prompts. We can start with just using $PROMPT as our
prompt if it exists, and falling back on our trusty old ">". While we're changing
things, we should probably print our prompt to `os.Stderr`, which is more
appropriate than Stdout for status-y type things like prompts or progress.

# "PrintPrompt Implementation"
```go
if p := os.Getenv("PROMPT"); p != "" {
	fmt.Fprintf(os.Stderr, "\n%s", p)
} else {
	fmt.Fprintf(os.Stderr, "\n> ")
}
```

And now if we wanted we could type `set PROMPT $` to make our shell look like
sh.. except we can't, because the `$` gets treated as an environment variable
by our parser before it gets to "set".

We can start by using the standard Go `os.ExpandEnv` function in our replacement
instead of our naive loop that just replaced any tokens that start with '$' with
the environment variable. This will also have the benefit of making our parser a
little more standard, and allowing us to use environment variables inside of
tokens, such as `$GOPATH/bin` too.

### "Replace environment variables in command"
```go
args := make([]string, 0, len(parsed))
for _, val := range parsed[1:] {
	args = append(args, os.ExpandEnv(val))
}
```

But that doesn't get us very far, either, because we still can't do anything
dynamic. We can try setting a `$` environment variable that evaluates to the
string "$" as a way to try escaping "$" in the shell and then something like
"set PROMPT $$PWD>" would theoretically set the prompt variable to the string
"$PWD>", but we'd have to hope that the ExpandEnv interprets $$PWD as ($$)PWD
and not $($PWD). Let's give it a shot anyways, but keeping in mind that if it
works we're depending on undocumented behaviour that may change in a future
version of Go.

### "Initialize Terminal" +=
```go
os.Setenv("$", "$")
```

It *seems* to work, so let's improve our PrintPrint implementation to dynamically
expand variables that were set in the PROMPT variable too

# "PrintPrompt Implementation"
```go
if p := os.Getenv("PROMPT"); p != "" {
	fmt.Fprintf(os.Stderr, "\n%s", os.ExpandEnv(p))
} else {
	fmt.Fprintf(os.Stderr, "\n> ")
}
```

We'd now be able to do something like `set PROMPT '$$PWD>'` to get the current
working directory in our prompt, except that our cd implementation doesn't
set PWD. Let's fix it to set both 

### "Handle cd command"
```go
if len(args) == 0 {
	return fmt.Errorf("Must provide an argument to cd")
}
old, _ := os.Getwd()
err := os.Chdir(args[0])
if err == nil {
	new, _ := os.Getwd()
	os.Setenv("PWD", new)
	os.Setenv("OLDPWD", old)
}
return err
```

There, now we can even do something like set PROMPT '$$PWD:$$?> ' to get the
last return code too.

What would be great though, was if we could use a '!' prefix to signify running
a command to run in order to get the prompt, similarly to how our tab completion
works.

It shouldn't be too hard to add a similar check to run the command with its
standard out being directed to stderr if our PROMPT variable starts with a '!'.
We could even expand the environment variables in the same way we're already
doing.

# "PrintPrompt Implementation"
```go
if p := os.Getenv("PROMPT"); p != "" {
	if len(p) > 1 && p[0] == '!' {
		<<<Run command for prompt>>>
	} else {
		fmt.Fprintf(os.Stderr, "\n%s", os.ExpandEnv(p))
	}
} else {
	fmt.Fprintf(os.Stderr, "\n> ")
}
```

### "Run command for prompt"
```go
input := os.ExpandEnv(p[1:])
split := strings.Fields(input)
cmd := exec.Command(split[0], split[1:]...)
cmd.Stdout = os.Stderr
if err := cmd.Run(); err != nil {
	// Fall back on our standard prompt, with a warning.
	fmt.Fprintf(os.Stderr, "\nInvalid prompt command\n> ")
}
```

But `Run` will return an err if the command exits with a non-zero error status,
which may have nothing to do with our prompt. If it fails to run, the error is
of type *ExitError according to the Run() documentation. So let's only print
our warning if the error returned is of that type.

### "Run command for prompt"
```go
input := os.ExpandEnv(p[1:])
split := strings.Fields(input)
cmd := exec.Command(split[0], split[1:]...)
cmd.Stdout = os.Stderr
if err := cmd.Run(); err != nil {
	if _, ok := err.(*exec.ExitError); !ok {
		// Fall back on our standard prompt, with a warning.
		fmt.Fprintf(os.Stderr, "\nInvalid prompt command\n> ")
	}
}
```

Now that we're customizing prompts, we might notice that if we set a prompt in
our startup script, the first prompt gets printed before our `~/.goshrc` script
is sourced. Let's add it to Initialize Shell

### "Initialize Shell" += 
```go
PrintPrompt()
```

and take it out of Initialize Terminal. (We'll have to do a little refactoring
of our blocks that we probably should have done upfront.)

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
terminal = t

<<<Create SIGCHLD chan>>>
<<<Ignore certain signal types>>>
os.Setenv("$", "$")
```

Now, we can create write prompts in any language of our choosing, as long as
we can print to standard out in our language.
