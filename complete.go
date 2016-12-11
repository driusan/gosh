package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

func (c *Command) Complete() error {
	tokens := c.Tokenize()
	var suggestions []string
	var base string
	switch len(tokens) {
	case 0:
		base = ""
		suggestions = CommandSuggestions(base)
	case 1:
		base = tokens[0]
		suggestions = CommandSuggestions(base)
	default:
		base = tokens[len(tokens)-1]
		suggestions = FileSuggestions(base)
	}
	switch len(suggestions) {
	case 0:
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
	filedir := filepath.Dir(base)
	fileprefix := filepath.Base(base)
	files, err := ioutil.ReadDir(filedir)
	if err != nil {
		return nil
	}
	var matches []string
	for _, file := range files {
		if name := file.Name(); strings.HasPrefix(name, fileprefix) {
			matches = append(matches, filedir+"/"+name)
		}
	}
	return matches
}
