# Background Processes, Revisited

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

### "Start Processes and Wait"
```go
var pgrp uint32 = uint32(syscall.Getpgrp())
fmt.Fprintf(os.Stderr, "My PGID: %v\n", pgrp)
sysProcAttr := &syscall.SysProcAttr{
	Setpgid: true,
}

for i, c := range cmds {
	fmt.Fprintf(os.Stderr, "Starting %d\n", i)
	c.SysProcAttr = sysProcAttr
	if err := c.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		continue
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
				fmt.Fprintf(os.Stderr, "Err for %v (%v): %v\n", pid1, pg, err)
				if pid1 == 0 && err == nil {
					// We don't want to accidentally remove things from processGroups
					fmt.Fprintf(os.Stderr, "%v is probably stopped\n", pg)
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
					if pg == ForegroundPid && ForegroundPid != 0 {
						<<<Resume Foreground>>>
					}
					fmt.Fprintf(os.Stderr, "%v is stopped\n", pid1)
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

And now we can use our shell for real, with CTRL-C/CTRL-Z etc going to the
appropriate process.
