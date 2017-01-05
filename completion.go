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
	var suggestions []string
	var base string
	switch len(tokens) {
	case 0:
		base = ""
		suggestions = CommandSuggestions(base)
	case 1:
		firstpart := strings.Join(tokens[:len(tokens)-1], " ")
		wholecmd := strings.Join(tokens, " ")
		base = tokens[len(tokens)-1]
		for re, resuggestions := range autocompletions {
			if len(suggestions) == 0 && re.MatchString(wholecmd) {
				for _, val := range resuggestions {
					// There was no last token, to take the prefix of, so
					// just suggest the whole val.
					if len(val) > 2 && val[0] == '!' {
						base = ""
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
							if strings.HasPrefix(val, base) {
								suggestions = append(suggestions, val)
							}
						}
					} else {
						suggestions = append(suggestions, string(val))
					}
				}
				base = ""
				break
			} else if re.MatchString(firstpart) {
				for _, val := range resuggestions {
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
							if strings.HasPrefix(val, base) {
								suggestions = append(suggestions, val)
							}
						}
					} else if strings.HasPrefix(string(val), base) {
						suggestions = append(suggestions, string(val))
					}
				}
			}
		}

		if len(suggestions) > 0 {
			break
		}
		base = tokens[0]
		suggestions = CommandSuggestions(base)
	default:
		firstpart := strings.Join(tokens[:len(tokens)-1], " ")
		wholecmd := strings.Join(tokens, " ")
		base = tokens[len(tokens)-1]
		for re, resuggestions := range autocompletions {
			if len(suggestions) == 0 && re.MatchString(wholecmd) {
				for _, val := range resuggestions {
					// There was no last token, to take the prefix of, so
					// just suggest the whole val.
					if len(val) > 2 && val[0] == '!' {
						base = ""
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
							if strings.HasPrefix(val, base) {
								suggestions = append(suggestions, val)
							}
						}
					} else {
						suggestions = append(suggestions, string(val))
					}
				}
				base = ""
				break
			} else if re.MatchString(firstpart) {
				for _, val := range resuggestions {
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
							if strings.HasPrefix(val, base) {
								suggestions = append(suggestions, val)
							}
						}
					} else if strings.HasPrefix(string(val), base) {
						suggestions = append(suggestions, string(val))
					}
				}
			}
		}

		if len(suggestions) > 0 {
			break
		}
		base = tokens[len(tokens)-1]
		suggestions = FileSuggestions(base)
	}

	switch len(suggestions) {
	case 0:
		// Print BEL to warn that there were no suggestions.
		fmt.Printf("\u0007")
	case 1:
		suggest := suggestions[0]
		*c = Command(strings.TrimSpace(string(*c)))
		*c = Command(strings.TrimSuffix(string(*c), base))
		*c += Command(suggest)
		PrintPrompt()
		fmt.Printf("%s", *c)
	default:
		fmt.Printf("\n%v\n", suggestions)
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
	if files, err := ioutil.ReadDir(base); err == nil {
		// This was a directory, so use the empty string as a prefix.
		fileprefix := ""
		filedir := base
		var matches []string
		for _, file := range files {
			if name := file.Name(); strings.HasPrefix(name, fileprefix) {
				if filedir != "/" {
					matches = append(matches, filedir+"/"+name)
				} else {
					matches = append(matches, filedir+name)
				}
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
			if filedir != "/" {
				matches = append(matches, filedir+"/"+name)
			} else {
				matches = append(matches, filedir+name)
			}
		}
	}
	return matches
}
