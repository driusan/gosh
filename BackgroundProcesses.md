# Background Processes and Signals

To be really useful as a shell, we need to at least handle the
basics of background processes. Let's start with starting a process in
the background.

This should be easy. All we need to do is check if the last token
in the command is an `&`, and if it is call `cmd.Start()` instead of
`cmd.Wait()` in our command handler. In fact, we should probably
start by ensuring we treat '&' as a special token when tokenizing.

Recall we had:

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

While we're at it, let's treat `<` and `>` specially to make our
redirection a little more robust. They all follow more or less
the same rule: be a delimiting token, unless it's inside a string.

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


Handle Pipe Chr looked like this:

### "Handle Pipe Chr"
```go
if inStringLiteral {
	continue
}
if tokenStart >= 0 {
	parsed = append(parsed, string(c[tokenStart:i]))
}
parsed = append(parsed, "|")
tokenStart = -1
```

If we change that from append "|" to append string(chr), we should
be able to handle them all in the same case that would look like this:

### "Handle Tokenize Chr"
```go
switch chr {
	case '\'':
		<<<Handle Quote>>>
	case '|', '<', '>', '&':
		<<<Handle Special Chr>>>
	default:
		<<<Handle Nonquote>>>
}
```

And then:

### "Handle Special Chr"
```go
if inStringLiteral {
	continue
}
if tokenStart >= 0 {
	parsed = append(parsed, string(c[tokenStart:i]))
}
parsed = append(parsed, string(chr))
tokenStart = -1
```

Now, we're ready to just check if the command ends in '&' and use
`Start` instead of `Wait` if so.

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

becomes

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

where "Handle background inputs" just needs to check if the
parsed tokens ends in a `&`, and if so set a flag for `Start Processes
and Wait` to skip the waiting bit.

### "Handle background inputs"
```go
var backgroundProcess bool
if parsed[len(parsed)-1] == "&" {
	parsed = parsed[:len(parsed)-1]
	backgroundProcess = true
}
```

### "Start Processes and Wait"
```go
for _, c := range cmds {
	err := c.Start()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
	}
}
if backgroundProcess {
	// We can't tell if a background process returns an error
	// or not, so we just claim it didn't.
	return nil
}
return cmds[len(cmds)-1].Wait()
```

Now when we try running a long-running command like `more BackgroundProcsses.md`
for testing, it doesn't quite work how we exepcted. *Both* the background
process and the shell are hooked up to STDIN.

While in `zsh` we get output like

```sh
$ more BackgroundProcsses.md&
[2] 1444
[2]  + 1444 suspended (tty output)  more BackgroundProcsses.md
```

in our shell we just launch more, which happily reads `STDIN` as if
it were a foreground process. We should probably only hook it up if it's
a foreground process, so that background processes don't read `STDIN`

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
		if !backgroundProcess {
			newCmd.Stdin = os.Stdin
		}
	}
}
```

Hm.. that didn't work. What's going on? If we look into it a little
more into how UNIX handles job control, according to [wikipedia](https://en.wikipedia.org/wiki/Job_control_(Unix)#Implementation)

> A background process that attempts to read from or write to its controlling 
> terminal is sent a SIGTTIN (for input) or SIGTTOU (for output) signal. These 
> signals stop the process by default, but they may also be handled in other 
> ways. Shells often override the default stop action of SIGTTOU so that
> background processes deliver their output to the controlling terminal by
> default.

So it looks like we'll have to get into signals sooner than we wanted.
`os.Process.Signal()` allows us to send arbitrary signals to programs. So
instead, let's create an io.Reader that, when read from, sends a signal to
a process. In fact, we'll make it a ReadWriter and allow different signals
for read and write, in case we want to implement the `SIGTTOU` behaviour too,
though for now we'll just do `SIGTTIN`.

### "Process Signaller"
```go
type ProcessSignaller struct{
	// The process to signal when Read from
	Proc *os.Process
	ReadSignal, WriteSignal os.Signal
}

func (p ProcessSignaller) Read([]byte) (n int, err error) {
	if p.Proc == nil {
		return 0, fmt.Errorf("Invalid process.")
	}
	if err := p.Proc.Signal(p.ReadSignal); err != nil {
		return 0, err
	}
	return 0, fmt.Errorf("Not an interactive terminal.")
}

func (p ProcessSignaller) Write([]byte) (n int, err error) {
	if p.Proc == nil {
		return 0, fmt.Errorf("Invalid process.")
	}
	if err := p.Proc.Signal(p.WriteSignal); err != nil {
		return 0, err
	}
	return 0, fmt.Errorf("Not an interactive terminal.")
}
```

For now, we'll add this to main.go, though it might eventually deserve
its own file.

### main.go +=
```go
<<<Process Signaller>>>
```

Now, we'll create the io.ReadWriter while checking if it's a background
process, and hook stdin up to that instead.
### "Handle background inputs"
```go
var backgroundProcess bool
var stdin io.Reader
if parsed[len(parsed)-1] == "&" {
	parsed = parsed[:len(parsed)-1]
	backgroundProcess = true
	stdin = &ProcessSignaller{
		// ????
	}
} else {
	stdin = os.Stdin
}
```

The `????` is because we don't yet have the `os.Process`, so how can
we instantiate the `ProcessSignaller`?. We'll have to stick with
our original handle background inputs.

### "Handle background inputs"
```go
var backgroundProcess bool
if parsed[len(parsed)-1] == "&" {
	// Strip off the &, it's not part of the command.
	parsed = parsed[:len(parsed)-1]
	backgroundProcess = true
}
```

Then wait until we have the `exec.Cmd` and are hooking up the `STDIN`
to instantiate our `ProcessSignaller`.

In fact, it's worse than that, we *also* don't have the `os.Process`
populated in the `exec.Cmd` instance while hooking up STDIN, because
Cmd.Process isn't populated until the process is started.

have two options: change our ProcessSignaller to take an `exec.Cmd`
instead of an `os.Process`, or add a special case after starting.

Since the only command hooked up to `STDIN` is the first one, it's
probably easier to do the latter. 

### "Start Processes and Wait"
```go
for i, c := range cmds {
	c.Start()
	if i == 0 && backgroundProcess {
		c.Stdin = &ProcessSignaller{
			c.Process,
			syscall.SIGTTIN,
			syscall.SIGTTOU,
		}
	}
}
if backgroundProcess {
	// We can't tell if a background process returns an error
	// or not, so we just claim it didn't.
	return nil
}
return cmds[len(cmds)-1].Wait()
```

### "main.go imports" +=
```go
"syscall"
```

Is this going to work? Can we change `c.Stdin` *after* the process has started?
It compiles and runs, but it turns out the answer is "no", because the behaviour
is the same as before. Let's make our ProcessSignaller a little smarter, then.
Let's have it include a `IsBackground` flag which will cause it to send the
signal, and if `IsBackground` is false, our reader will relay to `os.Stdin`.

This should also make it easier to handle `SIGTSTP` (ie. the user pressing 
`^Z` to background a running process) later.

### "Process Signaller"
```go
type ProcessSignaller struct{
	// The process to signal when Read from
	Proc *os.Process
	ReadSignal, WriteSignal os.Signal
	IsBackground bool
}

func (p ProcessSignaller) Read(b []byte) (n int, err error) {
	if p.IsBackground == false {
		return os.Stdin.Read(b)
	}
	if p.Proc == nil {
		return 0, fmt.Errorf("Invalid process.")
	}
	if err := p.Proc.Signal(p.ReadSignal); err != nil {
		return 0, err
	}
	return 0, fmt.Errorf("Not an interactive terminal.")
}

func (p ProcessSignaller) Write(b []byte) (n int, err error) {
	if p.IsBackground == false {
		return os.Stdout.Write(b)
	}

	if p.Proc == nil {
		return 0, fmt.Errorf("Invalid process.")
	}
	if err := p.Proc.Signal(p.WriteSignal); err != nil {
		return 0, err
	}
	return 0, fmt.Errorf("Not an interactive terminal.")
}
```

Then we always hook up our new reader to STDIN.

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
 		newCmd.Stdin = &ProcessSignaller{
			newCmd.Process,
			syscall.SIGTTIN,
			syscall.SIGTTOU,
			backgroundProcess,
		}
	}
}
```

But then after starting the process, we'll have to set the *os.Process
pointer if Stdin is a ProcessSignaller. (I suppose we could have done this
in the first place too, but I didn't think of it until now.)

### "Start Processes and Wait"
```go
for _, c := range cmds {
	c.Start()
	if ps, ok := c.Stdin.(*ProcessSignaller); ok {
		ps.Proc = c.Process
	}
}
if backgroundProcess {
	// We can't tell if a background process returns an error
	// or not, so we just claim it didn't.
	return nil
}
return cmds[len(cmds)-1].Wait()
```

That's.. sort of working, except that user input now seems to be extra slow
when running in the foregroundand there's no indication from more about it
 running in the background or way to resume it.

At least it's easy to print an error message before sending a signal.

### "Process Signaller"
```go
type ProcessSignaller struct{
	// The process to signal when Read from
	Proc *os.Process
	ReadSignal, WriteSignal os.Signal
	IsBackground bool
}

func (p *ProcessSignaller) Read(b []byte) (n int, err error) {
	if !p.IsBackground {
		return os.Stdin.Read(b)
	}
	if p.Proc == nil {
		return 0, fmt.Errorf("Invalid process.")
	}
	fmt.Fprintf(os.Stderr, "%d suspended (tty input from background)\n", p.Proc.Pid)
	if err := p.Proc.Signal(p.ReadSignal); err != nil {
		return 0, err
	}
	return 0, fmt.Errorf("Not an interactive terminal.")
}

func (p *ProcessSignaller) Write(b []byte) (n int, err error) {
	if !p.IsBackground {
		return os.Stdout.Write(b)
	}

	if p.Proc == nil {
		return 0, fmt.Errorf("Invalid process.")
	}
	fmt.Fprintf(os.Stderr, "%d suspended (tty output from background)\n", p.Proc.Pid)
	if err := p.Proc.Signal(p.WriteSignal); err != nil {
		return 0, err
	}
	return 0, fmt.Errorf("Not an interactive terminal.")
}
```

Now, what about that speed issue? In fact, it's blocking in the Read() call,
while os.Stdin didn't when set to the processes's stdin directly. I'm not really
certain why that's the case, but we can check if there's data available to return
and send an error instead of blocking in Read when there's no data available.

To do that, we'll have to make the terminal that was initialized available
in main() available globally.


### "main.go globals" +=
```go
var terminal *term.Term
```

### "Initialize Terminal" +=
```go
terminal = t
```

And then use that to add a check before calling os.Stdin.Read(). We'll send an
io.EOF error along with it, because in non-Go languages the definition of "EOF"
is "a read that returns 0 bytes" in C (where functions can only return 1 value)
so that's how the program that was invoked will be interpreting the 0 byte read
anyways.

### "main.go imports" +=
```go
"io"
```
### "Process Signaller"
```go
type ProcessSignaller struct{
	// The process to signal when Read from
	Proc *os.Process
	ReadSignal, WriteSignal os.Signal
	IsBackground bool
}

func (p *ProcessSignaller) Read(b []byte) (n int, err error) {
	if !p.IsBackground {
		// If there's no data available from os.Stdin,
		// don't block.
		if n, err := terminal.Available(); n <= 0 {
			if err != nil {
				return n, err
			}
			return n, io.EOF
		}
		return os.Stdin.Read(b)
	}
	if p.Proc == nil {
		return 0, fmt.Errorf("Invalid process.")
	}
	fmt.Fprintf(os.Stderr, "%d suspended (tty input from background)\n", p.Proc.Pid)
	if err := p.Proc.Signal(p.ReadSignal); err != nil {
		return 0, err
	}
	return 0, fmt.Errorf("Not an interactive terminal.")
}

func (p *ProcessSignaller) Write(b []byte) (n int, err error) {
	if !p.IsBackground {
		return os.Stdout.Write(b)
	}

	if p.Proc == nil {
		return 0, fmt.Errorf("Invalid process.")
	}
	fmt.Fprintf(os.Stderr, "%d suspended (tty output from background)\n", p.Proc.Pid)
	if err := p.Proc.Signal(p.WriteSignal); err != nil {
		return 0, err
	}
	return 0, fmt.Errorf("Not an interactive terminal.")
}
```

We can now *start* a process in the background, without any significant
performance loss, but have no way of sending existing processes to the background,
or resuming them. This exploration of adding a feature to the shell has
already lasted long enough, so let's leave job control for another day.
