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
processes, and at the end have the kernel switch that group.

syscall.SysProcAttr let's us set the pgid with a Setpgid flag. If we explicitly
set a Pgid it'll set it to that, otherwise it'll create a new one. We'll let
the first Pgid be set automatically, and then after starting the first process
get the Pgid for that process and set the rest of the processes in that group
to the new pgid.

We'll also want to keep a list of what process groups exist.

### "Start Processes and Wait"
```go
var pgrp uint32
sysProcAttr := &syscall.SysProcAttr{
	Setpgid: true,
}

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
if backgroundProcess {
	// We can't tell if a background process returns an error
	// or not, so we just claim it didn't.
	return nil
}
ForegroundPid = pgrp

_, _, err1 := syscall.RawSyscall(
	syscall.SYS_IOCTL,
	uintptr(0),
	uintptr(syscall.TIOCSPGRP),
	uintptr(unsafe.Pointer(&pgrp)),
)
if err1 != syscall.Errno(0) {
	return err1
}
return ForegroundProcess
```

# "main.go globals" +=
```go
var processGroups []uint32

var ForegroundProcess error = errors.New("Process is a foreground process")
var ForegroundPid uint32
```

# "main.go imports" +=
```go
"unsafe"
"errors"
"os/signal"
```

## "Initialize Terminal" +=
```go
signal.Ignore(
//	syscall.SIGTTIN,
	syscall.SIGTTOU,
	syscall.SIGINT,
)
child := make(chan os.Signal)
signal.Notify(child, syscall.SIGCHLD)
```

### "Handle Command"
```go
if cmd == "exit" || cmd == "quit" {
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
## "Resume Foreground"
```go
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
fmt.Fprintf(os.Stderr, "Resuming shell group...\n")
ForegroundPid = 0
```
## "main.go funcs" +=
```go
func Wait(ch chan os.Signal) {
	for {
		select {
		case <-ch:
			var newPg []uint32
			for _, pg := range processGroups {
				var status syscall.WaitStatus
				pid1, err := syscall.Wait4(int(pg), &status, syscall.WNOHANG|syscall.WUNTRACED|syscall.WCONTINUED, nil)
				if pid1 == 0 && err == nil {
					// We don't want to accidentally remove things from processGroups
					newPg = append(newPg, pg)
					continue
				}
				switch {
				case status.Continued(): 
					newPg = append(newPg, pg)
					fmt.Fprintf(os.Stderr, "Resuming %v...? \n", pg)

					if ForegroundPid == 0 {
						fmt.Fprintf(os.Stderr, "Resuming %v... !\n", pg)
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
					}
				case status.Stopped():
					newPg = append(newPg, pg)
					if status.Continued() {
						fmt.Fprintf(os.Stderr, "Resuming %v...? \n", pg)
						if ForegroundPid == 0 {
							fmt.Fprintf(os.Stderr, "Resuming %v... !\n", pg)
							var pid uint32 = pg
							_, _, err3 := syscall.RawSyscall(
								syscall.SYS_IOCTL,
								uintptr(0),
								uintptr(syscall.TIOCSPGRP),
								uintptr(unsafe.Pointer(&pid)),
							)
							if err3 !=syscall.Errno(0) {
								panic(fmt.Sprintf("Err: %v", err3))
							}

						} else {
							fmt.Fprintf(os.Stderr, "%v continued in background\n", pid1)
						}
					} else {
						if pg == ForegroundPid && ForegroundPid != 0 {
							<<<Resume Foreground>>>
						}
						fmt.Fprintf(os.Stderr, "%v is stopped\n", pid1)
					}
				case status.Signaled():
					if pg == ForegroundPid && ForegroundPid != 0 {
						<<<Resume Foreground>>>
					}

					fmt.Fprintf(os.Stderr, "%v terminated by signal %v\n", pg, status.StopSignal())
				case status.Exited():
					if pg == ForegroundPid && ForegroundPid != 0 {
						<<<Resume Foreground>>>
					} else {
						fmt.Fprintf(os.Stderr, "%v exited (exit status: %v)\n", pid1, status.ExitStatus())
					}
				default:
					newPg = append(newPg, pg)
					fmt.Fprintf(os.Stderr, "Still running: %v: %v\n", pid1, status)
				}

			}
			processGroups = newPg

			if ForegroundPid == 0 {
				return
			}
		}
	}
}
```


### "Builtin Commands" +=
```go
case "jobs":
	fmt.Printf("Job listing:\n\n")
	for i, leader := range processGroups {
		fmt.Printf("Job %d (%d)\n", i, leader)
	}
	return nil
```

### "main.go imports" +=
```go
"strconv"
```
### "Builtin Commands" +=
```go
case "bg":
	i, err := strconv.Atoi(args[0])
	if err != nil {
		return err
	}

	if i >= len(processGroups) {
		return fmt.Errorf("Invalid job id %d", i)
	}
	p, err := os.FindProcess(int(processGroups[i]))
	if err != nil {
		return err
	}
	fmt.Printf("Sending signal %v...\n", syscall.SIGCONT)
	if err := p.Signal(syscall.SIGCONT); err != nil {
		return err
	}
	return nil
```

### "Builtin Commands" +=
```go
case "fg":
	i, err := strconv.Atoi(args[0])
	if err != nil {
		return err
	}

	if i >= len(processGroups) {
		return fmt.Errorf("Invalid job id %d", i)
	}
	p, err := os.FindProcess(int(processGroups[i]))
	if err != nil {
		return err
	}
	fmt.Printf("Sending signal %v...\n", syscall.SIGCONT)
	if err := p.Signal(syscall.SIGCONT); err != nil {
		return err
	}
	var pid uint32 = processGroups[i]
	fmt.Fprintf(os.Stderr, "Resuming %v... !\n", pid)
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

	return ForegroundProcess
```

And now we can use our shell for real, with CTRL-C/CTRL-Z etc going to the
appropriate process.
