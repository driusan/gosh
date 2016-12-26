# Environment Variables and Startup Scripts

In a UNIX system, when you execute a progress, the new process inherits
the environment of the parent who spawned it. This has worked for us well so
far, because we inherited the `$PATH` of the shell that spawned us, which let
the `os/exec` package search the path without us doing anything special.

Unfortunately, it also means we inherited some variables that don't make sense
any more (like `$SHELL`), and it means that we can't be used as a login shell,
because there's no way to set environment variables yet.

We'll implement a `set` builtin, which takes two parameters: the name, and the
value of an environment variable to set. We don't need a way to launch processes
with different environments, because the standard Unix command `env` already
provides that, but we can't depend on an external command for setting an
environment, because any changes it makes to the environment would only be valid
for the child process and end once the spawned program dies.

We also need to provide a way to read and execute startup file (to bootstrap
the user's environment), which needs to be run in our own process space for much
the same reason.

## Expanding on builtins

Recall that our most recent HandleCmd implementation was:

### "HandleCmd Implementation"
```go
func (c Command) HandleCmd() error {
	parsed := c.Tokenize()
	<<<Handle no tokens in command case>>>
	<<<Replace environment variables in command>>>
	<<<Handle background inputs>>>
	<<<Handle cd command>>>
	<<<Execute command and return>>>
}
```

Let's change that "Handle cd" to be a more generic "Handle builtins", because
we'll probably want to be adding more builtins eventually.

### "HandleCmd Implementation"
```go
func (c Command) HandleCmd() error {
	parsed := c.Tokenize()
	<<<Handle no tokens in command case>>>
	<<<Replace environment variables in command>>>
	<<<Handle background inputs>>>
	<<<Handle builtin commands>>>
	<<<Execute command and return>>>
}
```

The builtins can be handled with a fairly simple switch (we'll separate the
macros into a different macro so that we can easily add to it when we have
more builtins.)

### "Handle builtin commands"
```go
switch parsed[0] {
	<<<Builtin Commands>>>
}
```

### "Builtin Commands"
```go
case "cd":
	<<<Handle cd command>>>
```

The extra unnecessary if statement in Handle cd is going to drive me insane now
that it's a switch, so let's get rid of it

### "Handle cd command"
```go
if len(args) == 0 {
	return fmt.Errorf("Must provide an argument to cd")
}
return os.Chdir(args[0])
```

And we can now add our "set" builtin.

### "Builtin Commands" +=
```go
case "set":
if len(args) != 2 {
	return fmt.Errorf("Usage: set var value")
}
return os.Setenv(args[0], args[1])
```

That was pretty straight forward. Now, let's also set the `$SHELL`
variable on startup and source a startup script. We'll add it to
the mainbody that we defined at the start of this shell, after
initializing the terminal but before going into the command loop.

### "mainbody"
```go
<<<Initialize Terminal>>>
<<<Initialize Shell>>>
<<<Command Loop>>>
```

### "Initialize Shell"
```go
os.Setenv("SHELL", os.Args[0])
<<<Read startup script>>>
```

Before reading the startup script, maybe it would make sense to add a
"source" builtin, since the code if effectively the same. We'll define
the function:

### "main.go funcs" +=
```go
func SourceFile(filename string) error {
	<<<SourceFile implementation>>>
}
```

and then we just need to call it with the startup script name. What
is the script name? We'll call it `$HOME/.goshrc`, and we'll use the
`os/user` package to look up the `$HOME` directory, just in case there's
any OS specific idiosyncrasies.

### "main.go imports" +=
```go
"os/user"
```

### "Read startup script"
```go
if u, err := user.Current(); err == nil {
	SourceFile(u.HomeDir + "/.goshrc")
}
```

Let's define the builtin, too. We'll call the builtin `source`, and might
as well just make it source all of the arguments

### "Builtin Commands" +=
```go
case "source":
	<<<Source Builtin>>>
```

### "Source Builtin" 
```go
if len(args) < 1 {
	return fmt.Errorf("Usage: source file [...other files]")
}

for _, f := range args {
	SourceFile(f)
}
return nil
```

Okay, now how do we actually implement SourceFile?

We'll obviously need to start by opening the file, then we need
to go through it line by line, and then we'll need to execute
each line as if the user had typed it.
then 
### "SourceFile implementation"
```go
<<<Open sourced file>>>
<<<Iterate through sourced file>>>
```

### "Open sourced file"
```go
f, err := os.Open(filename)
if err != nil {
	return err
}
defer f.Close()
```

We can iterate through the file by using a bufio.Reader and
reading until there's a '\n'.

### "Iterate through sourced file"
```go
scanner := bufio.NewReader(f)
for {
	line, err := scanner.ReadString('\n')
	switch err {
		case io.EOF:
			return nil
		case nil:
			// Nothing special
		default:
			return err	
	}
	<<<Handle sourced file line>>>
}
```

Now, we can handle a line by treating it the same way we treat user entered
input: with the `HandleCmd()` method on a `Command`.

### "Handle sourced file line"
```go
c := Command(line)
if err := c.HandleCmd(); err != nil {
	return err
}
```
