package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var autocompletions map[*regexp.Regexp][]Token

func (c *Command) Complete() error {
	tokens := c.Tokenize()
	var psuggestions, wsuggestions []string
	var base string

	var firstpart string
	if len(tokens) > 0 {
		base = tokens[len(tokens)-1]
		firstpart = strings.Join(tokens[:len(tokens)-1], " ")
	}
	wholecmd := strings.Join(tokens, " ")

	for re, resuggestions := range autocompletions {
		if matches := re.FindStringSubmatch(firstpart); matches != nil {
			for _, val := range resuggestions {
				for n, match := range matches {
					val = Token(strings.Replace(string(val), fmt.Sprintf(`\%d`, n), match, -1))
				}

				// If it's length 1 it's just "!", and we should probably
				// just suggest it literally.
				if len(val) > 2 && val[0] == '!' {
					cmd := strings.Fields(string(val[1:]))
					if len(cmd) < 1 {
						continue
					}
					c := exec.Command(cmd[0], cmd[1:]...)
					out, err := c.Output()
					if err != nil {
						println(err.Error())
						continue
					}
					sugs := strings.Split(string(out), "\n")
					for _, val := range sugs {
						if val != base && strings.HasPrefix(val, base) {
							psuggestions = append(psuggestions, val)
						}
					}
				} else if string(val) != base && strings.HasPrefix(string(val), base) {
					psuggestions = append(psuggestions, string(val))
				}
			}
		}

		if len(psuggestions) > 0 {
			continue
		}

		if matches := re.FindStringSubmatch(wholecmd); matches != nil {
			for _, val := range resuggestions {
				for n, match := range matches {
					val = Token(strings.Replace(string(val), fmt.Sprintf(`\%d`, n), match, -1))
				}

				if len(val) > 2 && val[0] == '!' {
					cmd := strings.Fields(string(val[1:]))
					if len(cmd) < 1 {
						continue
					}
					c := exec.Command(cmd[0], cmd[1:]...)
					out, err := c.Output()
					if err != nil {
						println(err.Error())
						continue
					}
					sugs := strings.Split(string(out), "\n")
					for _, val := range sugs {
						if val != base {
							wsuggestions = append(wsuggestions, val)
						}
					}
				} else {
					// There was no last token, to take the prefix of, so
					// just suggest the whole val.
					wsuggestions = append(wsuggestions, string(val))
				}
			}
		}
	}
	if len(psuggestions) > 0 {
		wsuggestions = nil
		goto foundSuggestions
	} else if len(wsuggestions) > 0 {
		goto foundSuggestions
	}

	switch len(tokens) {
	case 0:
		base = ""
		wsuggestions = CommandSuggestions(base)
	case 1:
		base = tokens[0]
		psuggestions = CommandSuggestions(base)
	default:
		base = tokens[len(tokens)-1]
		psuggestions = FileSuggestions(base)
	}

foundSuggestions:
	switch len(psuggestions) + len(wsuggestions) {
	case 0:
		// Print BEL to warn that there were no suggestions.
		fmt.Printf("\u0007")
	case 1:
		if len(psuggestions) == 1 {
			suggest := psuggestions[0]
			*c = Command(strings.TrimSpace(string(*c)))
			*c = Command(strings.TrimSuffix(string(*c), base))
			*c += Command(suggest)

			PrintPrompt()
			fmt.Printf("%s", *c)
		} else {
			suggest := wsuggestions[0]
			*c = Command(strings.TrimSpace(string(*c)))
			*c += Command(suggest)

			PrintPrompt()
			fmt.Printf("%s", *c)
		}
	default:
		suggestions := append(psuggestions, wsuggestions...)

		if len(wsuggestions) == 0 {
			suggest := LongestPrefix(suggestions)
			*c = Command(strings.TrimSpace(string(*c)))
			*c = Command(strings.TrimSuffix(string(*c), base))
			*c += Command(suggest)
		}
		fmt.Printf("\n[")
		for i, s := range suggestions {
			if strings.ContainsAny(s, " \t") {
				fmt.Printf(`"%v"`, s)
			} else {
				fmt.Printf("%v", s)
			}
			if i != len(suggestions)-1 {
				fmt.Printf(" ")
			}
		}
		fmt.Printf("]\n")

		PrintPrompt()
		fmt.Printf("%s", *c)
	}
	return nil
}

func CommandSuggestions(base string) []string {
	paths := strings.Split(os.Getenv("PATH"), ":")
	var matches []string
	for _, path := range paths {
		// We don't care if there's an invalid path in $PATH, so ignore
		// the error.
		files, _ := ioutil.ReadDir(path)
		for _, file := range files {
			if name := file.Name(); strings.HasPrefix(name, base) {
				matches = append(matches, name)
			}
		}
	}
	return matches
}

func FileSuggestions(base string) []string {
	base = replaceTilde(base)
	if files, err := ioutil.ReadDir(base); err == nil {
		// This was a directory, so use the empty string as a prefix.
		fileprefix := ""
		filedir := base
		var matches []string
		for _, file := range files {
			if name := file.Name(); strings.HasPrefix(name, fileprefix) {
				matches = append(matches, filepath.Clean(filedir+"/"+name))
			}
		}
		return matches
	}

	filedir := filepath.Dir(base)
	fileprefix := filepath.Base(base)
	files, err := ioutil.ReadDir(filedir)
	if err != nil {
		return nil
	}

	var matches []string
	for _, file := range files {
		if name := file.Name(); strings.HasPrefix(name, fileprefix) {
			matches = append(matches, filepath.Clean(filedir+"/"+name))
		}
	}
	return matches
}
