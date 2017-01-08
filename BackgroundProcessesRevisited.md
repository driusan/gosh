# Background Processes, Revisited

We hacked together a method of running processes in the background
with BackgroundProcesses.md, but unfortunately it isn't a very good
hack and we should probably do it right.

In reality, there's no need for the Process Signaller. The OS keeps
track of what the foreground process is, and sends signals the interrupt
signals as appropriate.

All we should be doing is using system calls to tell it when the foreground
process has switched. So let's start by getting rid of the Process Signaller,
and hooking up the first process to os.Stdin (unless redirected.) That way
we avoid the overhead that was causing our shell to be noticablely slow when
using our custom io.Reader for STDIN

### "Process Signaller"
```go
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

Now, when started a process in Go, exec can take a syscall.*SysProcAttr to
set system properties of the process that gets created. SysProcAttr has
a Foreground attribute, but if we used that, we wouldn't be able to create
our pipeline. As soon as the first process in the pipeline was created, we'd
be relegated to a background process ourselves.

What we need to do instead is set the process group ID (PGid) for each of our
processes, and at the end have the kernel switch that group to the foreground
process with a syscall.

syscall.SysProcAttr let's us set the pgid with a Setpgid flag. If we explicitly
set a Pgid it'll set it to that, otherwise it'll create a new one. We'll let
the first Pgid be set automatically, and then after starting the first process
get the Pgid for that process and set the rest of the processes in that group
to the new pgid.

We'll also want to keep a list of what process groups exist.

While we're at it, we should probably want to keep track of what the foreground
process is. We'll also need to keep track of the pgrp we're creating so that
we can set that to the foreground process.

# "main.go globals" +=
```go
var processGroups []uint32

var ForegroundPid uint32
```

### "Start Processes and Wait"
```go
<<<Create SysProcAttr with appropriate attributes>>>
<<<Start processes with proper Pgid>>>
<<<Change foreground process>>>
```

Let's try the simplest SysProcAttr:

### "Create SysProcAttr with appropriate attributes"
```
var pgrp uint32
sysProcAttr := &syscall.SysProcAttr{
	Setpgid: true,
}
```

And we'll range through cmds. After the first process is created,
we'll get the Pgid of that process to explicitly set it for further
processes in the pipeline.

### "Start processes with proper Pgid"
```go
for _, c := range cmds {
	c.SysProcAttr = sysProcAttr
	if err := c.Start(); err != nil {
		return err
	}
	if sysProcAttr.Pgid == 0 {
		sysProcAttr.Pgid, _ = syscall.Getpgid(c.Process.Pid)
		pgrp = uint32(sysProcAttr.Pgid)
		processGroups = append(processGroups, uint32(c.Process.Pid))
	}
}
```

Now, how do we handle changing the process group? If we dig into how Go
does it in the test framework, we'll see that is uses a raw syscall to `syscall.SYS_IOCTL`.
It'd be nice if there was something higher level (or if standard syscalls had
better names than `TIOCSPGRP`, but as far as I can tell there isn't.

So let's just return if this was a background process otherwise, and otherwise
make the syscall to set the foreground process to the process group we just created.
We should also set our global ForegroundPid variable while we're at it.

### "Change foreground process"
```go
<<<Return if backgroundProcess>>>
<<<Set ForegroundPid to pgrp>>>
<<<Set Foreground Process to pgrp>>>
```

### "Return if backgroundProcess"
```go
if backgroundProcess {
	// We can't tell if a background process returns an error
	// or not, so we just claim it didn't.
	return nil
}
```

### "Set ForegroundPid to pgrp"
```go
ForegroundPid = pgrp
```

### "Set Foreground Process to pgrp"
```go
_, _, err1 := syscall.RawSyscall(
	syscall.SYS_IOCTL,
	uintptr(0),
	uintptr(syscall.TIOCSPGRP),
	uintptr(unsafe.Pointer(&pgrp)),
)
// RawSyscall returns an int for the error, we need to compare
// to syscall.Errno(0) instead of nil
if err1 != syscall.Errno(0) {
	return err1
}
return ForegroundProcess
```

Note that we returned a ForegroundProcess error instead of a nil after setting
the foreground process group. This is to indicate to the caller that the process
spawned was a foreground process, so it should block. (Being a background process
doesn't mean we stop running, it just means any I/O will generate a signal telling
us to stop.)

We should probably declare the sentinal error that we're using, too.

# "main.go globals" +=
```go
var ForegroundProcess error = errors.New("Process is a foreground process")
```

One problem with this is that the process we're invoking likely expects the
terminal to be in the normal line mode, not CBreak mode, so we should probably
restore the tty to its original state before changing the active process, and
then set it to CBreak mode again once we get control back. (Ideally, when we
got control we'd check the status of the the terminal and keep it associated
with the process group in case the process was background/foregrounded, but for
now we'll just make the flawed assumption that we're the only one who will
ever change the tty mode, since the term package we're using doesn't seem
to have an easy way to get the current mode.)

### "Set Foreground Process to pgrp"
```go
terminal.Restore()
_, _, err1 := syscall.RawSyscall(
	syscall.SYS_IOCTL,
	uintptr(0),
	uintptr(syscall.TIOCSPGRP),
	uintptr(unsafe.Pointer(&pgrp)),
)
// RawSyscall returns an int for the error, we need to compare
// to syscall.Errno(0) instead of nil
if err1 != syscall.Errno(0) {
	return err1
}
return ForegroundProcess
```

Now, another problem is that once we launch a foreground process, we don't magically
become the foreground process again once it exits. We need to do that ourselves once
a foreground process terminates. But how do we know a foreground process terminated?

Whenever a child process dies or changes foreground state, the parent (that's us!)
gets a SIGCHLD signal from unix. The default behaviour for SIGCHLD in Go is to ignore it.
Instead, we can listen for it with the `os/signal` package. `os/signal` will send us
a message on a channel that we specify, and we'll have to wait for something to come in
on that channel if the process launched was a foreground process (that's where our
sentinal error comes in.)

We'll create the channel and ask to be notified when we're initializing the terminal.

## "Initialize Terminal" +=
```go
<<<Create SIGCHLD chan>>>
```

## "Create SIGCHLD chan"
```go
child := make(chan os.Signal)
signal.Notify(child, syscall.SIGCHLD)
```

Since we're importing `os/signal`, it may be a good idea to ignore some signals, like
the SIGTTOU we get if we try and print something while not the foreground process.
We probably want to ignore SIGINT (the user pressed ctrl-C) while we're at it.

## "Initialize Terminal" +=
```go
<<<Ignored certain signal types>>>
```

## "Ignore certain signal types"
```go
signal.Ignore(
	<<<Ignored signal types>>>
)
```

### "Ignored signal types"
```go
syscall.SIGTTOU,
syscall.SIGINT,
```

(We've used a new few imports that we should probably add, too:

# "main.go imports" +=
```go
"unsafe"
"errors"
"os/signal"
```
)

Now, our command handler needs to wait for the foreground process status to change if
we returned our ForegroundProcess sentinal.

### "Handle Command"
```go
if cmd == "exit" || cmd == "quit" {
	t.Restore()
	os.Exit(0)
} else if cmd == "" {
	PrintPrompt()
} else {
	err := cmd.HandleCmd()
	if err == ForegroundProcess {
		Wait(child)
	} else if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
	}
	PrintPrompt()
}
```

Let's start by defining the Wait function that we just used:

### "main.go funcs" +=
```go
func Wait(ch chan os.Signal) {
	<<<Wait Implementation>>>
}
```

How are we going to implement wait? Well, we'll have to start with waiting
for something to come in on our channel.

### "Wait Channel Select"
```go
select {
case <-ch:
	<<<SIGCHLD Received handler>>>
}
```

And then what? All we know is that we got a signal that a child status changed.
We don't even know who sent it. For all we know, a background process died or
finished. So let's just go through all of our processGroups and update our
processGroup list, resuming the foreground status of the shell as appropriate.

In fact, since we're deleting from processGroups, let's just make a new slice
and only add the still valid processes, since Go doesn't make it very easy to
delete from a slice while iterating through it.

### "SIGCHLD Received handler"
```go
newPg := make([]uint32, 0, len(processGroups))
for _, pg := range processGroups {
	<<<SIGCHLD update processGroup status>>>
}
processGroups = newPg
```

We also want to make sure we don't return from Wait if something else caused
a SIGCHLD, so let's wrap it in an infinite loop and only return when appropriate.

### "Wait Implementation"
```go
for {
	<<<Wait Channel Select>>>

	if ForegroundPid == 0 {
		return
	}
}
```


We want our update status handler to be something like:

### "SIGCHLD update processGroup status"
```go
<<<SIGCHLD Get pg status>>>
switch {
<<<SIGCHLD Handle pg statuses>>>
}
```

Okay, now how do we get the exited status of the process before it's done? `syscall.Wait4`
returns a `syscall.WaitStatus`, but not until the child exited, and we're in a loop, so we
don't want to block on every process group that we're trying to get the status of.

Luckily, there's some options. `WNOHANG` will cause it to not block but just return the status,
which is exactly what we want. (`WUNTRACED` and `WCONTINUED` just affect which information is
available from the WaitStatus.)

### "SIGCHLD Get pg status"
```go
var status syscall.WaitStatus
pid1, err := syscall.Wait4(int(pg), &status, syscall.WNOHANG|syscall.WUNTRACED|syscall.WCONTINUED, nil)
if pid1 == 0 && err == nil {
	// We don't want to accidentally remove things from processGroups if there was an error
	// from wait.
	newPg = append(newPg, pg)
	continue
}
```

We'll add cases for each of the booleans that WaitStatus provides methods to extract.

### "SIGCHLD Handle pg statuses"
```go
case status.Continued(): 
	<<<SIGCHLD Handle Continued>>>
case status.Stopped():
	<<<SIGCHLD Handle Stopped>>>
case status.Signaled():
	<<<SIGCHLD Handle Signaled>>>
case status.Exited():
	<<<SIGCHLD Handle Exited>>>
default:
	<<<SIGCHLD Default Handler>>>
```

Going through them one by one:

If the process is resuming because of a SIGCONT, we'll keep the process in our newPg list (it's
still alive), and make it the foreground process if there's no other foreground process.
### "SIGCHLD Handle Continued"
```go
newPg = append(newPg, pg)

if ForegroundPid == 0 {
	<<<Make pg foreground>>>
}
```

If the child was stopped and is the foreground process, we should resume our shell as the foreground
process, keep a reference to the process group, and print a message.

### "SIGCHLD Handle Stopped"
```go
newPg = append(newPg, pg)
if pg == ForegroundPid && ForegroundPid != 0 {
	<<<Resume Shell Foreground>>>
}
fmt.Fprintf(os.Stderr, "%v is stopped\n", pid1)
```

Signaled means the process died from a signal (as opposed to by calling exit.) In this case we
*don't* want to add it to the newPg list, but *do* want to resume the shell if it was the foreground
process. (We should also tell the user.)

### "SIGCHLD Handle Signaled"
```go
if pg == ForegroundPid && ForegroundPid != 0 {
	<<<Resume Shell Foreground>>>
}

fmt.Fprintf(os.Stderr, "%v terminated by signal %v\n", pg, status.StopSignal())
```

Exited means it exited normally. If it was the foreground process, we want to resume the shell.
If it was a background process, we want to tell the user. Either way, we should set the `$?`
environment variable, so that it's available from our shell.

### "SIGCHLD Handle Exited"
```go
if pg == ForegroundPid && ForegroundPid != 0 {
	<<<Resume Shell Foreground>>>
} else {
	fmt.Fprintf(os.Stderr, "%v exited (exit status: %v)\n", pid1, status.ExitStatus())
}
os.Setenv("?", strconv.Itoa(status.ExitStatus()))
```

### "main.go imports" +=
```go
"strconv"
```

Finally, for the default case, we'll make sure we don't accidentally lose a process from our list
and print a message just so we know that it's happening (even though it probably means we missed
a case statement.) 

### "SIGCHLD Default Handler"
```go
newPg = append(newPg, pg)
fmt.Fprintf(os.Stderr, "Still running: %v: %v\n", pid1, status)
```

We already worked out the syscall for Make pg foreground and Resume Shell Foreground in our HandleCmd,
but our variable names are different.

### "Make pg foreground"
```go
terminal.Restore()
var pid uint32 = pg
_, _, err3 := syscall.RawSyscall(
	syscall.SYS_IOCTL,
	uintptr(0),
	uintptr(syscall.TIOCSPGRP),
	uintptr(unsafe.Pointer(&pid)),
)
if err3 != syscall.Errno(0) {
	panic(fmt.Sprintf("Err: %v", err3))
}
ForegroundPid = pid
```

Resuming the shell as the foreground is similar, except the process group is our own PID, and we want
to make sure we set the ForegroundPid to 0.

### "Resume Shell Foreground"
```go
terminal.SetCbreak()
var mypid uint32 = uint32(syscall.Getpid())
_, _, err3 := syscall.RawSyscall(
	syscall.SYS_IOCTL,
	uintptr(0),
	uintptr(syscall.TIOCSPGRP),
	uintptr(unsafe.Pointer(&mypid)),
)
if err3 != syscall.Errno(0) {
	panic(fmt.Sprintf("Err: %v", err3))
}
ForegroundPid = 0
```

## Builtins

We've finally got support for background processes! Almost. Ctrl-Z works, but we don't
have any way to trigger it being resumed.

We don't have a way to resume it, even if our code handles the case. We'll add a "jobs"
builtin to print the processGroups, and a bg and fg command to send them a signal to
stop or resume.

### "Builtin Commands" +=
```go
case "jobs":
	<<<Handle jobs>>>
case "bg":
	<<<Handle bg>>>
case "fg":
	<<<Handle fg>>>

```

The jobs case is easy:

### "Handle jobs"
```go
fmt.Printf("Job listing:\n\n")
for i, leader := range processGroups {
	fmt.Printf("Job %d (%d)\n", i, leader)
}
return nil
```

The bg case shouldn't be very difficult either. We just need to parse
the first argument, convert it to an int, and then get send a `SIGCONT`
signal to the process to tell it to continue.


### "Handle bg"
```go
if len(args) < 1 {
	return fmt.Errorf("Must specify job to background.")
}
i, err := strconv.Atoi(args[0])
if err != nil {
	return err
}

if i >= len(processGroups) || i < 0 {
	return fmt.Errorf("Invalid job id %d", i)
}
p, err := os.FindProcess(int(processGroups[i]))
if err != nil {
	return err
}
if err := p.Signal(syscall.SIGCONT); err != nil {
	return err
}
return nil
```

fg is similar, except we also want to set ForegroundPid and 
send our TIOCSPGRP syscall to the Pid and return the ForegroundProcess
sentinal.

### "Handle fg"
```go
if len(args) < 1 {
	return fmt.Errorf("Must specify job to foreground.")
}
i, err := strconv.Atoi(args[0])
if err != nil {
	return err
}

if i >= len(processGroups) || i < 0 {
	return fmt.Errorf("Invalid job id %d", i)
}
p, err := os.FindProcess(int(processGroups[i]))
if err != nil {
	return err
}
if err := p.Signal(syscall.SIGCONT); err != nil {
	return err
}
terminal.Restore()
var pid uint32 = processGroups[i]
_, _, err3 := syscall.RawSyscall(
	syscall.SYS_IOCTL,
	uintptr(0),
	uintptr(syscall.TIOCSPGRP),
	uintptr(unsafe.Pointer(&pid)),
)
if err3 != syscall.Errno(0) {
	panic(fmt.Sprintf("Err: %v", err3))
} else {
	ForegroundPid = pid
	return ForegroundProcess
}
```

And *now* we can use our shell for real with job control.
