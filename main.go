package main

import (
	"bufio"
	"fmt"
	"github.com/pkg/term"
	"io"
	"os"
	"os/exec"
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
	// Allocate a string slice of the length of the arguments
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

func PrintPrompt() {
	fmt.Printf("\n> ")
}

func main() {
	// Initialize the terminal
	t, err := term.Open("/dev/tty")
	if err != nil {
		panic(err)
	}
	// Restore the previous terminal settings at the end of the program
	defer t.Restore()
	PrintPrompt()

	r := bufio.NewReader(t)
	var cmd Command
	for {
		c, _, err := r.ReadRune()
		switch err {
		case nil:
			break
		case io.EOF:
			os.Exit(0)
		default:
			panic(err)
		}
		switch c {
		case '\n':
			if cmd == "exit" {
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
		default:
			cmd += Command(c)
		}
	}
}
