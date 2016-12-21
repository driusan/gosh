package main

import (
	"bufio"
	"fmt"
	"github.com/pkg/term"
	"io"
	"os"
	"os/exec"
	"syscall"
)

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
	if parsed[0] == "cd" {
		if len(args) == 0 {
			return fmt.Errorf("Must provide an argument to cd")
		}
		return os.Chdir(args[0])
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
				newCmd.Stdin = &ProcessSignaller{
					newCmd.Process,
					syscall.SIGTTIN,
					syscall.SIGTTOU,
					backgroundProcess,
				}
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

type ProcessSignaller struct {
	// The process to signal when Read from
	Proc                    *os.Process
	ReadSignal, WriteSignal os.Signal
	IsBackground            bool
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
