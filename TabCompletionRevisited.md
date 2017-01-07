# Improving Tab Completion

We already have a basic tab completion for filenames, but to be a really useful
shell it would be nice if we could customize the behaviour of it and add the
ability for users to have context-sensitive customizable tab completion. For
instance, if the command is `git`, autocompleting to filenames doesn't make much
sense. Better suggestions would be "add" or "commit" or "checkout".

Let's add a builtin command with a name like "autocomplete" to customize the
behaviour. (We also didn't have the infrastructure in our codes to easily add
builtins when we first did tab completion.)

How should our "autocomplete" builtin work? It needs a way to match the current
string (minus the last token, which is being completed..) against some pattern,
and then evaluate if it matched. Regexes seem like an obvious solution. Let's
tokenize the command, remove the last token, and then compare it against a regex.
If any custom autocompleter matched, we'll use those suggestions, otherwise we'll
fall back on the old behaviour. To start, we'll make the list of suggestions
parameters passed to "autocomplete" after the regex. 

So defining a suggestion might be something like

```sh
> autocompete /^git/ add checkout commit
```

In fact, we don't really need the normal regex slash delimitors since we're taking
the first parameter, and maybe we should just make the '^' implicit, because we'll
pretty much always want our regexes to start at the start of the command. That would give us

```sh
> autocompete git add checkout commit
```

which is pretty nice, but then again, maybe there is a use case for completion
suggetions that aren't anchored at the start of the command, so for now we'll
keep the `^` and ditch the `/`.

## Implementing Regex Completion

Recall our previous auto completion implementation was:

### "AutoCompletion Implementation"
```go
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
	// Print BEL to warn that there were no suggestions.
	fmt.Printf("\u0007")
case 1:
	<<<Complete Suggestion>>>
default:
	<<<Display Suggestions>>>
}
return nil
```

It's really only the default "FileSuggestions" in the first switch statement
that we're going to want to change right now. Instead, we'll do something like

### "AutoCompletion Implementation"
```go
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
	<<<Check regex suggestions and break if found>>>
	<<<Check file suggestions>>>
}

switch len(suggestions) {
case 0:
	// Print BEL to warn that there were no suggestions.
	fmt.Printf("\u0007")
case 1:
	<<<Complete Suggestion>>>
default:
	<<<Display Suggestions>>>
}
return nil
```

### "Check file suggestions"
```go
base = tokens[len(tokens)-1]
suggestions = FileSuggestions(base)
```

So how do we "Check regex suggestions and break if found"? We'll want to start
by importing the regex package that we know we're going to use.

### "completion.go imports" +=
```go
"regexp"
```

And we know we're going to need a list of Regex->Suggestion mappings, so let's
define that.

### "completion.go globals" +=
```go
<<<Autocompletion Map>>>
```

### "Autocompletion Map"
```go
var autocompletions map[regexp.Regexp][]Token
```

Now, our check is pretty straight forward. We just range over the map, and
any regex that matches the current command gets added to suggestions (if the
last token matches the suggestion.) Then we'll break if we found any.

### "Check regex suggestions and break if found"
```go
firstpart := strings.Join(tokens[:len(tokens)-1], " ")
lasttoken := tokens[len(tokens)-1]
for re, resuggestions := range autocompletions {
	if re.MatchString(firstpart) {
		for _, val := range resuggestions {
			if strings.HasPrefix(string(val), lasttoken) {
				suggestions = append(suggestions, string(val))
			}
		}
	}
}

if len(suggestions) > 0 {
	break
}
```

(Note that we know the length is at least 2, because of the position in the
switch statement.)

### "completion.go imports" +=
```go
"strings"
```

Now we're getting a compile error: "invalid map key type regexp.Regexp". If
we look into it, we find that regexp.Regexp has a slice under the hood, which
can't be used as a map key in Go because their size isn't fixed. To get around
this, we'll just make our map a map of pointers to regexp.Regexpes.

### "Autocompletion Map"
```go
var autocompletions map[*regexp.Regexp][]Token
```

That was suspiciously easy, but we can't use it yet without defining a way
to set them. Let's define our completion builtin. All we need to do is check
the arguments, create the map if it hasn't been created yet, and populate
the tokens.

### "Builtin Commands" +=
```go
case "autocomplete":
<<<AutoComplete Builtin Command>>>
```

### "AutoComplete Builtin Command"
```go
<<<Check autocomplete usage>>>
<<<Create autocomplete map if nil>>>
<<<Add suggestions to map>>>

return nil
```

### "Check autocomplete usage"
```go
if len(args) < 2 {
	return fmt.Errorf("Usage: autocomplete regex value [more values...]")
}
```

### "Create autocomplete map if nil"
```go
if autocompletions == nil {
	autocompletions = make(map[*regexp.Regexp][]Token)
}
```

### "Add suggestions to map"
```go
re, err := regexp.Compile(args[0])
if err != nil {
	return err
}

for _, t := range args[1:] {
	autocompletions[re] = append(autocompletions[re], Token(t))
}
```

The builtin handler is in main.go, so we'll need to import regexp there too.

### "main.go imports" +=
```go
"regexp"
```

Now, there's a problem where the completion isn't suggestion isn't removing the
part that was already typed, so 'git ad<tab>' is completing to 'git adadd'. In
our suggestion code from TabCompletion.md, it trims the variable "base" to
avoid that for files, so we should probably set the "base" variable to lasttoken
when we find a match too (or better yet, just re-use the `base` variable instead
of making a new "lasttoken" variable for the same purpose.)

### "Check regex suggestions and break if found"
```go
firstpart := strings.Join(tokens[:len(tokens)-1], " ")
base = tokens[len(tokens)-1]
for re, resuggestions := range autocompletions {
	if re.MatchString(firstpart) {
		for _, val := range resuggestions {
			if strings.HasPrefix(string(val), base) {
				suggestions = append(suggestions, string(val))
			}
		}
	}
}

if len(suggestions) > 0 {
	break
}
```

In fact, there's a slight problem with our check. The way we've implemented it,
we need to type at least one character. If we just type "git" we won't get our
suggestions until we type at least one more letter.

To do fix that, we'll just checks. The first one will check the entire
command and use a base of "", and the second one will do what we've just done.
Since it's the same regex, we can do it in one loop, but we can only have either
whole command *or* subtoken matches, because of the "base" variable, so once
we find one we'll break out of our autocompletions loop to prevent the risk
of conflicts between an empty and non-empty base.

### "Check regex suggestions and break if found"
```go
firstpart := strings.Join(tokens[:len(tokens)-1], " ")
wholecmd := strings.Join(tokens, " ")
base = tokens[len(tokens)-1]
for re, resuggestions := range autocompletions {
	if re.MatchString(wholecmd) {
		for _, val := range resuggestions {
			// There was no last token, to take the prefix of, so
			// just suggest the whole val.
			suggestions = append(suggestions, string(val))
		}
		base = " "
		break
	} else if re.MatchString(firstpart) {
		for _, val := range resuggestions {
			if strings.HasPrefix(string(val), base) {
				suggestions = append(suggestions, string(val))
			}
		}
	}
}

if len(suggestions) > 0 {
	break
}
```

In fact, we're going to want this behaviour for command suggestions too, so let's
add it to our switch.

### "AutoCompletion Implementation"
```go
tokens := c.Tokenize()
var suggestions []string
var base string
switch len(tokens) {
case 0:
	<<<Check regex suggestions and break if found>>>
	base = ""
	suggestions = CommandSuggestions(base)
case 1:
	<<<Check regex suggestions and break if found>>>
	base = tokens[0]
	suggestions = CommandSuggestions(base)
default:
	<<<Check regex suggestions and break if found>>>
	<<<Check file suggestions>>>
}

switch len(suggestions) {
case 0:
	// Print BEL to warn that there were no suggestions.
	fmt.Printf("\u0007")
case 1:
	<<<Complete Suggestion>>>
default:
	<<<Display Suggestions>>>
}
return nil
```


At this point, there's no point in keeping it in the switch statement, because
it's in every case. Let's take it out:

### "AutoCompletion Implementation"
```go
tokens := c.Tokenize()
var suggestions []string
var base string
<<<Check regex suggestions>>>
if len(suggestions) == 0 {
	switch len(tokens) {
	case 0:
		base = ""
		suggestions = CommandSuggestions(base)
	case 1:
		base = tokens[0]
		suggestions = CommandSuggestions(base)
	default:
		<<<Check file suggestions>>>
	}
}

switch len(suggestions) {
case 0:
	// Print BEL to warn that there were no suggestions.
	fmt.Printf("\u0007")
case 1:
	<<<Complete Suggestion>>>
default:
	<<<Display Suggestions>>>
}
return nil
```

We have still have a couple problems: ranging through a map is defined to be in
random order in Go. If we keep pressing tab with the autocomplete samples that
we used as our motivation, and type 'git show<tab>' we'll see that *sometimes*
it shows the rev-list, and sometimes it doesn't. Because of the "base" variable,
we probably don't have any choice but to keep two slices of suggestions: one for
"whole command" suggestions and one for "partial token" suggestions.

While we're at it, let's live dangerously and get rid of that indented switch
with a goto.

### "AutoCompletion Implementation"
```go
tokens := c.Tokenize()
var psuggestions, wsuggestions []string
var base string

<<<Check regex suggestions>>>
if len(wsuggestions) > 0 || len(psuggestions) > 0 {
	goto foundSuggestions
}

switch len(tokens) {
case 0:
	wsuggestions = CommandSuggestions(base)
case 1:
	base = tokens[0]
	psuggestions = CommandSuggestions(base)
default:
	<<<Check file suggestions>>>
}

foundSuggestions:
switch len(psuggestions) + len(wsuggestions){
case 0:
	// Print BEL to warn that there were no suggestions.
	fmt.Printf("\u0007")
case 1:
	if len(psuggestions) == 1 {
		<<<Complete PSuggestion>>>
	} else {
		<<<Complete WSuggestion>>>
	}
default:
	<<<Display All Suggestions>>>
}
return nil
```

We'll have to redefine some blocks based on previous blocks with new the new 
variable names (and without the break.)

### "Check regex suggestions"
```go
firstpart := strings.Join(tokens[:len(tokens)-1], " ")
wholecmd := strings.Join(tokens, " ")
base = tokens[len(tokens)-1]
for re, resuggestions := range autocompletions {
	if re.MatchString(wholecmd) {
		for _, val := range resuggestions {
			// There was no last token, to take the prefix of, so
			// just suggest the whole val.
			wsuggestions = append(wsuggestions, string(val))
		}
	} else if re.MatchString(firstpart) {
		for _, val := range resuggestions {
			if string(val) != base && strings.HasPrefix(string(val), base) {
				psuggestions = append(psuggestions, string(val))
			}
		}
	}
}
```

### "Complete PSuggestion"
```go
suggest := psuggestions[0]
*c = Command(strings.TrimSpace(string(*c)))
*c = Command(strings.TrimSuffix(string(*c), base))
*c += Command(suggest)

PrintPrompt()
fmt.Printf("%s", *c)
```

### "Complete WSuggestion"
```go
suggest := wsuggestions[0]
*c = Command(strings.TrimSpace(string(*c)))
*c += Command(suggest)

PrintPrompt()
fmt.Printf("%s", *c)
```

### "Display All Suggestions"
```go
fmt.Printf("\n%v\n", append(psuggestions, wsuggestions...))

PrintPrompt()
fmt.Printf("%s", *c)
```
)

### "Check file suggestions"
```go
base = tokens[len(tokens)-1]
psuggestions = FileSuggestions(base)
```

## More flexible suggestions

We now have a basic customizable tab completion implementation, but what if
we want to make it a little more flexible?

Say we wanted `/^git show/` to suggest the output of the command
`git rev-list -n 10 HEAD` and suggest the last 10 commits? We can come up with
a convention like "if the suggestion token starts with a "!", then run the command
(without the "!"), and each line of the command run becomes a suggestion.

To do that, we'll start by adding a check for the first character inside of
our loop.

### "Check regex suggestions"
```go
firstpart := strings.Join(tokens[:len(tokens)-1], " ")
wholecmd := strings.Join(tokens, " ")
base = tokens[len(tokens)-1]
for re, resuggestions := range autocompletions {
	if re.MatchString(wholecmd) {
		for _, val := range resuggestions {
			if len(val) > 2 && val[0] == '!' {
				<<<WSuggest output of running command>>>
			} else {
				// There was no last token, to take the prefix of, so
				// just suggest the whole val.
				// As a special case, we still want to ignore it
				// if the suggestion matches the last token, so
				// that we don't step on psuggestion's feet.
				if string(val) != base {
					wsuggestions = append(wsuggestions, string(val))
				}
			}
		}
	} else if re.MatchString(firstpart) {
		for _, val := range resuggestions {
			// If it's length 1 it's just "!", and we should probably
			// just suggest it literally.
			if len(val) > 2 && val[0] == '!' {
				<<<PSuggest output of running command>>>
			} else if string(val) != base && strings.HasPrefix(string(val), base) {
				psuggestions = append(psuggestions, string(val))
			}
		}
	}
}
```

Now, to get the output of the command we'll can just run os/exec.Output on val[1:],
because we don't need any of the fancy background/foreground semantics we needed
to handle for commands being executed by the user.

### "PSuggest output of running command"
```go
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
```

### "WSuggest output of running command"
```go
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
```


### "completion.go imports" +=
```go
"os/exec"
```

We now have much more flexible tab completion, but can we improve it even more?

Two ideas:
1. Why don't we autocomplete partial matches?
2. Why don't we make the regex subgroup matches available as a variable to the
   suggestions?

## Partial Matching

Partial matching is fairly straight forward. We have a list of matches, and we
want to find the longest common prefix. There's probably a longest common
substring in Go that someone's written, but since we only care about the prefix
case we can do a naive implementation that ranges through our matches.

We'll start by adding a block to our AutoCompletion Implementation right before
Display Suggestions.

### "AutoCompletion Implementation"
```go
tokens := c.Tokenize()
var psuggestions, wsuggestions []string
var base string

<<<Check regex suggestions>>>
if len(wsuggestions) > 0 || len(psuggestions) > 0 {
	goto foundSuggestions
}

switch len(tokens) {
case 0:
	wsuggestions = CommandSuggestions(base)
case 1:
	base = tokens[0]
	psuggestions = CommandSuggestions(base)
default:
	<<<Check file suggestions>>>
}

foundSuggestions:
switch len(psuggestions) + len(wsuggestions){
case 0:
	// Print BEL to warn that there were no suggestions.
	fmt.Printf("\u0007")
case 1:
	if len(psuggestions) == 1 {
		<<<Complete PSuggestion>>>
	} else {
		<<<Complete WSuggestion>>>
	}
default:
	<<<Complete Partial Matches>>>
	<<<Display All Suggestions>>>
}
return nil
```

our suggestions are stored in psuggestions and wsuggestions. We want the longest
prefix in the union of them. Let's start by creating a all of those. 

### "Complete Partial Matches"
```go
suggestions := append(psuggestions, wsuggestions...)
```

Since we had to do something similar for displaying, let's update that code
to use our new union slice.

### "Display All Suggestions"
```go
fmt.Printf("\n%v\n", suggestions)

PrintPrompt()
fmt.Printf("%s", *c)
```


Now, all we need to do is find the prefix. Let's start assume there's already a
LongestPrefix function, and add the completion code first, based on our
Psuggestion code:

### "Complete Partial Matches" +=
```go
suggest := LongestPrefix(suggestions)
*c = Command(strings.TrimSpace(string(*c)))
*c = Command(strings.TrimSuffix(string(*c), base))
*c += Command(suggest)
```

Except that we've reintroduced the regression that caused us to split
wsuggestions from psuggestions in the first place. Let's ensure that 

### "Complete Partial Matches"
```go
```

Then we can design with a table-driven test to make sure our LongestPrefix is
working as expected.

### prefix_test.go
```go
package main

import (
	"testing"
	<<<Prefix Test Imports>>>
)

func TestLongestPrefix(t *testing.T) {
	cases := []struct{
		Val []string
		Expected string
	}{
		<<<LongestPrefix Test Cases>>>
	}
	for i, tc := range cases {
		if got := LongestPrefix(tc.Val); got != tc.Expected {
			t.Errorf("Unexpected prefix for case %d: got %v want %v", i, got, tc.Expected)
		}
	}
}
```

And we should define our stub function:

### prefix.go
```go
package main

import (
	<<<prefix.go imports>>>
)

func LongestPrefix(strs []string) string {
	<<<LongestPrefix Implementation>>>
}
```

And stubs for the blocks we just defined so that it will compile.
### "prefix.go imports"
```go
```

### "Prefix Test Imports"
```go
```

### "LongestPrefix Implementation"
```go
return ""
```


Now, we'll define some real test cases.

### "LongestPrefix Test Cases"
```go
// Empty case or nil slice
{ nil, ""},
{ []string{}, "" },

// Prefix of 1 element is itself
{ []string{"a"}, "a"},

// 2 elements with no prefix
{ []string{"a", "b"}, "" },

// 2 elements with a common prefix
{ []string{"aa", "ab"}, "a"},

// multiple elements
{ []string{"aaaa", "aabb", "aaac"}, "aa"},
```

And as expected, our tests fail.

So for our implementation, let's start by assuming that strs[0] is the longest
prefix, then we'll go through the remaining elements, range through them, and
if anything doesn't match, take the smaller prefix and go on to the next element.

(We'll also add a quick check for the empty case to make sure it doesn't crash.)

### "LongestPrefix Implementation"
```go
if len(strs) == 0 {
	return ""
}

prefix := strs[0]
for _, cmp := range strs[1:] {
	for i := range prefix {
		if i > len(cmp) || prefix[i] != cmp[i]{
			prefix = cmp[:i]
			break
		}
	}
}
return prefix
```

And now our partial completions work, except we're reintroduced the regression
which caused us to split psuggestions and wsuggestions in the first place. The
easiest way to handle this is probably do only do prefix suggestions if there
are no whole-command based suggestions (wsuggestions), because if the user hasn't
even started typing one letter of the last token that we're matching, the prefix
completion isn't very useful anyways. (We'll keep the definition of "suggestions"
since it's referenced in the display block, even if it's meaningless for prefix
completions now.)

### "Complete Partial Matches"
```go
suggestions := append(psuggestions, wsuggestions...)

if len(wsuggestions) == 0 {
	suggest := LongestPrefix(suggestions)
	*c = Command(strings.TrimSpace(string(*c)))
	*c = Command(strings.TrimSuffix(string(*c), base))
	*c += Command(suggest)
}
```

## Subgroup Regexp Matching

Subgroup matching shouldn't be difficult to add, because any regexp engine
already implements it. Let's start by replacing MatchString with
FindStringSubmatch when checking our regular expressions, so that we have the
values that we're going to need later.

### "Check regex suggestions"
```go
var firstpart string
if len(tokens) > 0 {
	base = tokens[len(tokens)-1]
	firstpart = strings.Join(tokens[:len(tokens)-1], " ")
}
wholecmd := strings.Join(tokens, " ")
for re, resuggestions := range autocompletions {
	if matches := re.FindStringSubmatch(wholecmd); matches != nil {
		for _, val := range resuggestions {
			if len(val) > 2 && val[0] == '!' {
				<<<WSuggest output of running command>>>
			} else {
				// There was no last token, to take the prefix of, so
				// just suggest the whole val.
				// As a special case, we still want to ignore it
				// if the suggestion matches the last token, so
				// that we don't step on psuggestion's feet.
				if string(val) != base {
					wsuggestions = append(wsuggestions, string(val))
				}
			}
		}
	} else if matches := re.FindStringSubmatch(firstpart); matches != nil {
		for _, val := range resuggestions {
			// If it's length 1 it's just "!", and we should probably
			// just suggest it literally.
			if len(val) > 2 && val[0] == '!' {
				<<<PSuggest output of running command>>>
			} else if string(val) != base && strings.HasPrefix(string(val), base) {
				psuggestions = append(psuggestions, string(val))
			}
		}
	}
}
```

Now, we should just need to replace \n in the string with matches[n] in our
suggestions. The `os` package has an `os.Expand` helper that can do this for $n,
but if we tried to use "$" to represent the submatches, we'd have trouble
interpretting the "autocomplete" builtin when defining our completions, because
the $n would be expanded to an environment variable before it got to us.

Luckily, it's not hard to just use a loop that replaces \n with matches[n]
blindly.

### "Check regex suggestions"
```go
var firstpart string
if len(tokens) > 0 {
	base = tokens[len(tokens)-1]
	firstpart = strings.Join(tokens[:len(tokens)-1], " ")
}
wholecmd := strings.Join(tokens, " ")
for re, resuggestions := range autocompletions {
	if matches := re.FindStringSubmatch(wholecmd); matches != nil {
		for _, val := range resuggestions {
			<<<Expand Matches>>>

			if len(val) > 2 && val[0] == '!' {
				<<<WSuggest output of running command>>>
			} else {
				// There was no last token, to take the prefix of, so
				// just suggest the whole val.
				// As a special case, we still want to ignore it
				// if the suggestion matches the last token, so
				// that we don't step on psuggestion's feet.
				if string(val) != base {
					wsuggestions = append(wsuggestions, string(val))
				}
			}
		}
	} else if matches := re.FindStringSubmatch(firstpart); matches != nil {
		for _, val := range resuggestions {
			<<<Expand Matches>>>

			// If it's length 1 it's just "!", and we should probably
			// just suggest it literally.
			if len(val) > 2 && val[0] == '!' {
				<<<PSuggest output of running command>>>
			} else if string(val) != base && strings.HasPrefix(string(val), base) {
				psuggestions = append(psuggestions, string(val))
			}
		}
	}
}
```

### "Expand Matches"
```go
for n, match := range matches {
	val = Token(strings.Replace(string(val), fmt.Sprintf(`\%d`, n), match, -1))
}
```

This is going to be horribly inefficient as the number of substring matches
increases in a regex, but shouldn't affect the general case where people aren't
using submatches much, so there isn't much need to optimize it right now. The
most powerful use of the substring matches is probably to use it to pass
arguments to an autocomplete suggester program, and the overhead of invoking a
separate program in that case is probably going to dwarf our loop anyways.

Now, \0 (or any submatch) may have a space in it, which makes the display confusing.
Let's update our printing to include quotation marks if the suggestion has a
space or a tab.

### "Display All Suggestions"
```go
fmt.Printf("\n[")
for  i, s := range suggestions {
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
```

We still seem to have a bug where, if regexes match both psuggestions and
wsuggestions, the wsuggestions will always get suggested even if they don't
match the partially typed string. To fix that, let's just set wsuggestions to
nil if there are any psuggestions to make sure they get priority.

### "AutoCompletion Implementation"
```go
tokens := c.Tokenize()
var psuggestions, wsuggestions []string
var base string

<<<Check regex suggestions>>>
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
	<<<Check file suggestions>>>
}

foundSuggestions:
switch len(psuggestions) + len(wsuggestions){
case 0:
	// Print BEL to warn that there were no suggestions.
	fmt.Printf("\u0007")
case 1:
	if len(psuggestions) == 1 {
		<<<Complete PSuggestion>>>
	} else {
		<<<Complete WSuggestion>>>
	}
default:
	<<<Complete Partial Matches>>>
	<<<Display All Suggestions>>>
}
return nil
```

That still didn't do it because we're doing something silly and psuggestions
in an else if block after wsuggestions, so let's fix that. Now that we have the
psuggestion always takes precedence over wsuggestions logic, we can just skip
checking wsuggestions if there's any psuggestions found.

### "Check regex suggestions"
```go
var firstpart string
if len(tokens) > 0 {
	base = tokens[len(tokens)-1]
	firstpart = strings.Join(tokens[:len(tokens)-1], " ")
}
wholecmd := strings.Join(tokens, " ")

for re, resuggestions := range autocompletions {
	if matches := re.FindStringSubmatch(firstpart); matches != nil {
		for _, val := range resuggestions {
			<<<Expand Matches>>>

			// If it's length 1 it's just "!", and we should probably
			// just suggest it literally.
			if len(val) > 2 && val[0] == '!' {
				<<<PSuggest output of running command>>>
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
			<<<Expand Matches>>>

			if len(val) > 2 && val[0] == '!' {
				<<<WSuggest output of running command>>>
			} else {
				// There was no last token, to take the prefix of, so
				// just suggest the whole val.
				wsuggestions = append(wsuggestions, string(val))
			}
		}
	}
}
```


## Conclusions

We now have an `autocomplete` builtin which can suggest autocompletions for user
input based on comparing what they've typed to a regular expression. The
suggestions can either be simple strings, based on (sub) matches of the regex, 
or based on invoking a separate program (potentially with arguments, which may
or may not come from the regex).

We have the potential for great, customizable tab completion, we just need
some examples.

I've included a sample goshrc in this repo with some simple examples that I use
in my `~/.goshrc` file. Feel free to send pull requests to it with anything else
you find useful, and be as creative as you'd like.
