package gamesettings

import (
	"fmt"
	"os"
	"strings"
)

// CheckFile looks for mistakes that make Palworld ignore its settings file (it then quietly runs
// with defaults: no name, no password, default rates): a missing section header, an unclosed or
// broken OptionSettings line, a value the game can't read, an option written twice, or more than
// one OptionSettings line. It returns plain-words problems; none means the file looks right.
func CheckFile(path string) []string {
	b, err := os.ReadFile(path)
	if err != nil {
		return []string{"can't read " + path + ": " + err.Error()}
	}
	text := string(b)
	var out []string
	if !strings.Contains(text, "[/Script/Pal.PalGameWorldSettings]") {
		out = append(out, "the [/Script/Pal.PalGameWorldSettings] line is missing: the game ignores everything below it")
	}
	if n := strings.Count(text, "OptionSettings="); n > 1 {
		out = append(out, fmt.Sprintf("OptionSettings appears %d times: the game uses only one of them", n))
	}
	f, err := Read(path)
	if err != nil {
		return append(out, err.Error()+": the game falls back to its defaults")
	}
	if strings.Count(text[f.start:f.end], `"`)%2 != 0 {
		out = append(out, "a quote is missing in OptionSettings (an odd number of \" marks)")
	}
	seen := map[string]bool{}
	for _, p := range f.Pairs {
		if seen[p.Key] {
			out = append(out, p.Key+" is written twice")
		}
		seen[p.Key] = true
		if strings.ContainsAny(p.Key, " \t\r\n\"") {
			out = append(out, fmt.Sprintf("%q doesn't look like an option name (a comma or quote out of place?)", p.Key))
			continue
		}
		if _, known := Lookup(p.Key); known {
			if err := Validate(p.Key, unquote(p.Raw)); err != nil {
				out = append(out, err.Error())
			}
		}
	}
	return out
}
