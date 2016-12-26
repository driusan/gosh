package main

import (
	"bufio"
	"fmt"
	"github.com/pkg/term"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"syscall"
	"unsafe"
)

var processGroups []uint32

type Command string

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
	var backgroundProcess bool
	if parsed[len(parsed)-1] == "&" {
		// Strip off the &, it's not part of the command.
		parsed = parsed[:len(parsed)-1]
		backgroundProcess = true
	}
	switch parsed[0] {
	case "cd":
		if len(args) == 0 {
			return fmt.Errorf("Must provide an argument to cd")
		}
		return os.Chdir(args[0])
	case "set":
		if len(args) != 2 {
			return fmt.Errorf("Usage: set var value")
		}
		return os.Setenv(args[0], args[1])
	case "source":
		if len(args) < 1 {
			return fmt.Errorf("Usage: source file [...other files]")
		}

		for _, f := range args {
			SourceFile(f)
		}
		return nil
	}
	// Convert parsed from []string to []Token. We should refactor all the code
	// to use tokens, but for now just do this instead of going back and changing
	// all the references/declarations in every other section of code.
	var parsedtokens []Token
	for _, t := range parsed {
		parsedtokens = append(parsedtokens, Token(t))
	}
	commands := ParseCommands(parsedtokens)
	var cmds []*exec.Cmd
	sysProcAttr := &syscall.SysProcAttr{
		Setpgid: true,
	}
	for i, c := range commands {
		if len(c.Args) == 0 {
			// This should have never happened, there is
			// no command, but let's avoid panicing.
			continue
		}
		newCmd := exec.Command(c.Args[0], c.Args[1:]...)
		newCmd.Stderr = os.Stderr
		newCmd.SysProcAttr = sysProcAttr
		cmds = append(cmds, newCmd)

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
				newCmd.Stdin = os.Stdin /*&ProcessSignaller{
					newCmd.Process,
					syscall.SIGTTIN,
					syscall.SIGTTOU,
					backgroundProcess,
				}*/
			}
		}
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
			if i == len(commands)-1 {
				fmt.Printf("Using STDOUT for %v\n", i)
				newCmd.Stdout = os.Stdout
			}
		}
	}

	var pgrp uint32 = uint32(syscall.Getpgrp())
	var leader uint32 = pgrp
	fmt.Fprintf(os.Stderr, "My PGID: %v My FD: %v\n", pgrp, terminal.GetFD())
	for i, c := range cmds {
		fmt.Fprintf(os.Stderr, "Starting %d\n", i)
		if err := c.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			continue
		}
		if sysProcAttr.Pgid == 0 {
			sysProcAttr.Pgid, _ = syscall.Getpgid(c.Process.Pid)
			pgrp = uint32(sysProcAttr.Pgid)
			processGroups = append(processGroups, pgrp)
		}

		pgid, _ := syscall.Getpgid(c.Process.Pid)
		fmt.Fprintf(os.Stderr, "PID: %v PGID: %v\n", c.Process.Pid, pgid)
	}
	if backgroundProcess {
		// We can't tell if a background process returns an error
		// or not, so we just claim it didn't.
		return nil
	}

	// DEBUG
	fpgrp := 0
	var fd int = terminal.GetFD()
	x, y, errno := syscall.RawSyscall(syscall.SYS_IOCTL, uintptr(fd), syscall.TIOCGPGRP, uintptr(unsafe.Pointer(&fpgrp)))
	fmt.Printf("x: %v, y: %v, err: %v %v (%v)\n", x, y, errno, fpgrp, leader)

	_, _, err1 := syscall.RawSyscall(
		syscall.SYS_IOCTL,
		uintptr(0),
		uintptr(syscall.TIOCSPGRP),
		uintptr(unsafe.Pointer(&pgrp)),
	)
	err2 := cmds[len(cmds)-1].Wait()
	_, _, err3 := syscall.RawSyscall(
		syscall.SYS_IOCTL,
		uintptr(0),
		uintptr(syscall.TIOCSPGRP),
		uintptr(unsafe.Pointer(&leader)),
	)
	fmt.Fprintf(os.Stderr, "IOCTL err %v\nWait err: %v\nresuming: %v %v %v\n", err1, err2, err3)
	for i, pgrp := range processGroups {
		fmt.Fprintf(os.Stderr, "Job %d => %v\n", i, pgrp)
	}
	return err2

}
func PrintPrompt() {
	fmt.Printf("\n> ")
}

type ParsedCommand struct {
	Args   []string
	Stdin  string
	Stdout string
}

func ParseCommands(tokens []Token) []ParsedCommand {
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
			lastCommandStart = i + 1
			foundSpecial = false
			currentCmd = ParsedCommand{}
		}
	}
	return allCommands
}

var terminal *term.Term

func SourceFile(filename string) error {
	f, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer f.Close()
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
		c := Command(line)
		if err := c.HandleCmd(); err != nil {
			return err
		}
	}
}

func main() {
	// Initialize the terminal
	t, err := term.Open("/dev/tty")
	if err != nil {
		panic(err)
	}
	// Restore the previous terminal settings at the end of the program
	defer t.Restore()
	t.SetCbreak()
	PrintPrompt()
	terminal = t
	os.Setenv("SHELL", os.Args[0])
	if u, err := user.Current(); err == nil {
		SourceFile(u.HomeDir + "/.goshrc")
	}
	signal.Ignore(
		//syscall.SIGTTIN,
		syscall.SIGTTOU,
	)
	child := make(chan os.Signal)

	signal.Notify(child, syscall.SIGCHLD)
	/*
		var pgrp uint32 = uint32(syscall.Getpgrp())
		_, _, err1 := syscall.RawSyscall(
			syscall.SYS_IOCTL,
			uintptr(0),
			uintptr(syscall.TIOCSPGRP),
			uintptr(unsafe.Pointer(&pgrp)),
		)
		fmt.Fprintf(os.Stderr, "%v\n", err1)
		c := make(chan os.Signal, 1)
		signal.Notify(c)
	*/
	r := bufio.NewReader(t)
	var cmd Command
	for {
		select {
		case <-child:
			var leader uint32 = uint32(syscall.Getpid())

			_, _, err3 := syscall.RawSyscall(
				syscall.SYS_IOCTL,
				uintptr(0),
				uintptr(syscall.TIOCSPGRP),
				uintptr(unsafe.Pointer(&leader)),
			)
			if err3 != syscall.Errno(0) {
				panic(fmt.Sprintf("Err: %v", err3))
			}

			//println("Received child signal!")
		default:
			c, _, err := r.ReadRune()
			if err != nil {
				continue
			}
			switch c {
			case '\n':
				// The terminal doesn't echo in raw mode,
				// so print the newline itself to the terminal.
				fmt.Printf("\n")

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
				if len(cmd) > 0 {
					cmd = cmd[:len(cmd)-1]
					fmt.Printf("\u0008 \u0008")
				}
			case '\t':
				err := cmd.Complete()
				if err != nil {
					fmt.Fprintf(os.Stderr, "%v\n", err)
				}
			default:
				fmt.Printf("%c", c)
				cmd += Command(c)
			}
		}
	}
}
