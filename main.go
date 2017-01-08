package main

import (
	"bufio"
	"errors"
	"fmt"
	"github.com/pkg/term"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

type Command string
type ParsedCommand struct {
	Args   []string
	Stdin  string
	Stdout string
}

var terminal *term.Term
var processGroups []uint32

var ForegroundPid uint32
var ForegroundProcess error = errors.New("Process is a foreground process")
var homedirRe *regexp.Regexp = regexp.MustCompile("^~([a-zA-Z]*)?(/*)?")

func main() {
	// Initialize the terminal
	t, err := term.Open("/dev/tty")
	if err != nil {
		panic(err)
	}
	// Restore the previous terminal settings at the end of the program
	defer t.Restore()
	t.SetCbreak()
	terminal = t

	child := make(chan os.Signal)
	signal.Notify(child, syscall.SIGCHLD)
	signal.Ignore(
		syscall.SIGTTOU,
		syscall.SIGINT,
	)
	os.Setenv("$", "$")
	os.Setenv("SHELL", os.Args[0])
	if u, err := user.Current(); err == nil {
		SourceFile(u.HomeDir + "/.goshrc")
	}
	PrintPrompt()
	r := bufio.NewReader(t)
	var cmd Command
	for {
		c, _, err := r.ReadRune()
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			continue
		}
		switch c {
		case '\n':
			// The terminal doesn't echo in raw mode,
			// so print the newline itself to the terminal.
			fmt.Printf("\n")

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
func (c Command) HandleCmd() error {
	parsed := c.Tokenize()
	if len(parsed) == 0 {
		// There was no command, it's not an error, the user just hit
		// enter.
		PrintPrompt()
		return nil
	}
	args := make([]string, 0, len(parsed))
	for _, val := range parsed[1:] {
		args = append(args, os.ExpandEnv(val))
	}
	// newargs will be at least len(parsed in size, so start by allocating a slice
	// of that capacity
	newargs := make([]string, 0, len(args))
	for _, token := range args {
		token = replaceTilde(token)
		expanded, err := filepath.Glob(token)
		if err != nil || len(expanded) == 0 {
			newargs = append(newargs, token)
			continue
		}
		newargs = append(newargs, expanded...)

	}
	args = newargs
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
		old, _ := os.Getwd()
		err := os.Chdir(args[0])
		if err == nil {
			new, _ := os.Getwd()
			os.Setenv("PWD", new)
			os.Setenv("OLDPWD", old)
		}
		return err
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
	case "jobs":
		fmt.Printf("Job listing:\n\n")
		for i, leader := range processGroups {
			fmt.Printf("Job %d (%d)\n", i, leader)
		}
		return nil
	case "bg":
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
	case "fg":
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

	case "autocomplete":
		if len(args) < 2 {
			return fmt.Errorf("Usage: autocomplete regex value [more values...]")
		}
		if autocompletions == nil {
			autocompletions = make(map[*regexp.Regexp][]Token)
		}
		re, err := regexp.Compile(args[0])
		if err != nil {
			return err
		}

		for _, t := range args[1:] {
			autocompletions[re] = append(autocompletions[re], Token(t))
		}

		return nil
	}
	// Convert parsed from []string to []Token. We should refactor all the code
	// to use tokens, but for now just do this instead of going back and changing
	// all the references/declarations in every other section of code.
	var parsedtokens []Token = []Token{Token(parsed[0])}
	for _, t := range args {
		parsedtokens = append(parsedtokens, Token(t))
	}
	commands := ParseCommands(parsedtokens)
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
				newCmd.Stdout = os.Stdout
			}
		}
	}

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
}
func PrintPrompt() {
	if p := os.Getenv("PROMPT"); p != "" {
		if len(p) > 1 && p[0] == '!' {
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
		} else {
			fmt.Fprintf(os.Stderr, "\n%s", os.ExpandEnv(p))
		}
	} else {
		fmt.Fprintf(os.Stderr, "\n> ")
	}
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
func Wait(ch chan os.Signal) {
	for {
		select {
		case <-ch:
			newPg := make([]uint32, 0, len(processGroups))
			for _, pg := range processGroups {
				var status syscall.WaitStatus
				pid1, err := syscall.Wait4(int(pg), &status, syscall.WNOHANG|syscall.WUNTRACED|syscall.WCONTINUED, nil)
				if pid1 == 0 && err == nil {
					// We don't want to accidentally remove things from processGroups if there was an error
					// from wait.
					newPg = append(newPg, pg)
					continue
				}
				switch {
				case status.Continued():
					newPg = append(newPg, pg)

					if ForegroundPid == 0 {
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
					}
				case status.Stopped():
					newPg = append(newPg, pg)
					if pg == ForegroundPid && ForegroundPid != 0 {
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
					}
					fmt.Fprintf(os.Stderr, "%v is stopped\n", pid1)
				case status.Signaled():
					if pg == ForegroundPid && ForegroundPid != 0 {
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
					}

					fmt.Fprintf(os.Stderr, "%v terminated by signal %v\n", pg, status.StopSignal())
				case status.Exited():
					if pg == ForegroundPid && ForegroundPid != 0 {
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
					} else {
						fmt.Fprintf(os.Stderr, "%v exited (exit status: %v)\n", pid1, status.ExitStatus())
					}
					os.Setenv("?", strconv.Itoa(status.ExitStatus()))
				default:
					newPg = append(newPg, pg)
					fmt.Fprintf(os.Stderr, "Still running: %v: %v\n", pid1, status)
				}
			}
			processGroups = newPg
		}

		if ForegroundPid == 0 {
			return
		}
	}
}
func replaceTilde(s string) string {
	if match := homedirRe.FindStringSubmatch(s); match != nil {
		var u *user.User
		var err error
		if match[1] != "" {
			u, err = user.Lookup(match[1])
		} else {
			u, err = user.Current()
		}
		if err == nil {
			return strings.Replace(s, match[0], u.HomeDir, 1)
		}
	}
	return s
}
