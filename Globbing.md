# File Globbing

In order to be really useful as a login shell, we need to support file globbing.

For instance, `ls *.go` should be expanded to `ls [all the files in the directory
with the extenion .go]` or `ls ~/` should display our home directory. The Go
standard lib package `path/filepath` has a `Glob` function, we just need to
decide how to use it (and potentially handle `~`).

Recall that our last HandleCmd implementation was:

### "HandleCmd Implementation"
```go
func (c Command) HandleCmd() error {
	parsed := c.Tokenize()
	<<<Handle no tokens in command case>>>
	<<<Replace environment variables in command>>>
	<<<Handle background inputs>>>
	<<<Handle builtin commands>>>
	<<<Execute command and return>>>
}
```

We probably want to do it after replacing environment variables, in case the
environment variables contain globs.

Let's try.

### "HandleCmd Implementation"
```go
func (c Command) HandleCmd() error {
	parsed := c.Tokenize()
	<<<Handle no tokens in command case>>>
	<<<Replace environment variables in command>>>
	<<<Expand file glob tokens>>>
	<<<Handle background inputs>>>
	<<<Handle builtin commands>>>
	<<<Execute command and return>>>
}
```

Let's just create a new parsed and append the expanded tokens to it as we find
them, then at the end we can set parsed to our new `newparsed` variable in case
any of the other blocks reference it. (In fact, "parsed" isn't used after this
point, "args" is, so let's do it for "args")

### "Expand file glob tokens"
```go
// newargs will be at least len(parsed in size, so start by allocating a slice
// of that capacity
newargs := make([]string, 0, len(args))
for _, token := range args {
	expanded, err := filepath.Glob(token)
	if err != nil || len(expanded) == 0 {
		newargs = append(newargs, token)
		continue
	}
	newargs = append(newargs, expanded...)

}
args = newargs
```

### "main.go imports" +=
```go
"path/filepath"
```

"*", "?" and "[]" work as expected, but "~" doesn't. We'll have to manually
check if an argument starts with "~" and expand it. We should probably make
sure we match `~user/foo` too, so let's use the regexp .

### "Expand file glob tokens"
```go
homedirRe := regexp.MustCompile("^~([a-zA-Z]*)?(/)?")
// newargs will be at least len(parsed in size, so start by allocating a slice
// of that capacity
newargs := make([]string, 0, len(args))
for _, token := range args {
	<<<Replace tilde with homedir in token>>>
	expanded, err := filepath.Glob(token)
	if err != nil || len(expanded) == 0 {
		newargs = append(newargs, token)
		continue
	}
	newargs = append(newargs, expanded...)

}
args = newargs
```

The fact that regexp needs to be compiled every time we call HandleCmd is
annoying, so let's move it into a global that only gets compiled on startup
(and that we can use elsewhere.)

### "Expand file glob tokens"
```go
// newargs will be at least len(parsed in size, so start by allocating a slice
// of that capacity
newargs := make([]string, 0, len(args))
for _, token := range args {
	<<<Replace tilde with homedir in token>>>
	expanded, err := filepath.Glob(token)
	if err != nil || len(expanded) == 0 {
		newargs = append(newargs, token)
		continue
	}
	newargs = append(newargs, expanded...)

}
args = newargs
```

### "main.go globals" +=
```go
var homedirRe *regexp.Regexp = regexp.MustCompile("^~([a-zA-Z]*)?(/*)?")
```

### "Replace tilde with homedir in token"
```
if match := homedirRe.FindStringSubmatch(token); match != nil {
	var u *user.User
	var err error
	if match[1] != "" {
		u, err = user.Lookup(match[1])
	} else {
		u, err = user.Current()
	}
	if err == nil {
		token = strings.Replace(token, match[0], u.HomeDir + "/", 1)
	}
}
```

### "main.go imports" +=
```go
"os/user"
"strings"
```

We can do ls ~/ or ls ~root now, but we have a problem where tab completion
isn't smart enough to look `~` style directories.

Let's move our replacer code into a function so that we don't have to duplicate
it here.

### "main.go funcs" +=
```go
func replaceTilde(s string) string {
	<<<replaceTilde implementation>>>
}
```

### "replaceTilde implementation"
```go
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
```

Then we just can use while expanding in HandleCmd

### "Replace tilde with homedir in token"
```go
token = replaceTilde(token)
```

As for file suggestions, we had this:

### "File Suggestions Implementation"
```go
<<<Check base dir>>>

filedir := filepath.Dir(base)
fileprefix := filepath.Base(base)
files, err := ioutil.ReadDir(filedir)
if err != nil {
	return nil
}

<<<Check files for matches and return>>>
```

We should be able to just blindly replace base with the tilde replaced version
at the start of the function.

### "File Suggestions Implementation"
```go
base = replaceTilde(base)
<<<Check base dir>>>

filedir := filepath.Dir(base)
fileprefix := filepath.Base(base)
files, err := ioutil.ReadDir(filedir)
if err != nil {
	return nil
}

<<<Check files for matches and return>>>
```

This results in the "~" being expanded directly on the commandline when we push
tab. This probably isn't a big deal, and might even a good thing since it might
make people stop believing it's a character with special meaning in filenames
outside of the shell.

Testing this reveals another problem that we've always had: when we tab complete
a directory, the "/" at the end gets duplicated each time we hit tab. We should
be able to just use `filepath.Clean` before appending the file to our matches.

Our code was:

### "Append file match"
```go
if filedir != "/" {
	matches = append(matches, filedir + "/" + name)
} else {
	matches = append(matches, filedir + name)
}
```

And now we might even be able to get rid of the if statement

### "Append file match"
```go
matches = append(matches, filepath.Clean(filedir + "/" + name))
```

(This also fixes an annoyance where "./" would get prepended to the beginning
of file names when tab completing, and makes tab general clean up file paths.)
