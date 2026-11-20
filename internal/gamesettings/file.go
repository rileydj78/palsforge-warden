// Package gamesettings reads and edits PalWorldSettings.ini without disturbing it: the order of
// the options, options Warden doesn't know, and everything outside OptionSettings stay as they are.
package gamesettings

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Pair is one option as written in the file (Raw keeps quotes and formatting).
type Pair struct {
	Key string
	Raw string
}

// File is a parsed PalWorldSettings.ini.
type File struct {
	Path   string
	text   string
	start  int // where the option list starts (after "OptionSettings=(")
	end    int // where it ends (at the closing ")")
	Pairs  []Pair
	Values map[string]string // unquoted values
}

var (
	openRe = regexp.MustCompile(`OptionSettings=\(`)
	keyRe  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)
)

// Read parses the file.
func Read(path string) (*File, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text := string(b)
	loc := openRe.FindStringIndex(text)
	if loc == nil {
		return nil, errors.New("no OptionSettings line in " + path)
	}
	f := &File{Path: path, text: text, start: loc[1], Values: map[string]string{}}
	depth, quoted, cur := 0, false, f.start
	flush := func(endAt int) {
		kv := strings.TrimSpace(text[cur:endAt])
		cur = endAt + 1
		if kv == "" {
			return
		}
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			return
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		f.Pairs = append(f.Pairs, Pair{Key: k, Raw: v})
		f.Values[k] = unquote(v)
	}
	for p := f.start; p < len(text); p++ {
		switch c := text[p]; {
		case c == '"':
			quoted = !quoted
		case quoted:
		case c == '(':
			depth++
		case c == ')' && depth > 0:
			depth--
		case c == ')':
			flush(p)
			f.end = p
			return f, nil
		case c == ',' && depth == 0:
			flush(p)
		}
	}
	return nil, errors.New("the OptionSettings line in " + path + " isn't closed")
}

func unquote(v string) string {
	if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
		return v[1 : len(v)-1]
	}
	return v
}

// Change is one requested edit.
type Change struct {
	Key   string `json:"key"`
	Old   string `json:"old"`
	New   string `json:"new"`
	Label string `json:"label"`
}

// Plan checks requested values and returns the changes that would really be made.
func (f *File) Plan(want map[string]string) ([]Change, error) {
	out := []Change{} // never null in JSON: the panel reads .length
	var bad []string
	for k, v := range want {
		if !keyRe.MatchString(k) {
			bad = append(bad, fmt.Sprintf("%q isn't an option name", k))
			continue
		}
		old, exists := f.Values[k]
		if _, known := Lookup(k); !known && !exists {
			bad = append(bad, fmt.Sprintf("%s isn't an option this server has", k))
			continue
		}
		o, known := Lookup(k)
		if known && o.Locked {
			if v != old {
				bad = append(bad, fmt.Sprintf("%s can't be changed here", o.Label))
			}
			continue
		}
		raw, err := format(o, known, v)
		if err != nil {
			bad = append(bad, err.Error())
			continue
		}
		if exists && unquote(raw) == old {
			continue
		}
		if exists {
			if cur, _ := format(o, known, old); cur == raw {
				continue // same value, written differently (1 vs 1.000000)
			}
		}
		label := k
		if known {
			label = o.Label
		}
		out = append(out, Change{Key: k, Old: old, New: unquote(raw), Label: label})
	}
	if len(bad) > 0 {
		return nil, errors.New(strings.Join(bad, "; "))
	}
	return out, nil
}

// format turns a value into what the file holds, checking it against the catalog.
func format(o Option, known bool, v string) (string, error) {
	v = strings.TrimSpace(v)
	if !known {
		if strings.ContainsAny(v, "\",()\r\n") {
			return "", fmt.Errorf("the value for an option Warden doesn't know can't contain quotes, commas or brackets (edit those in the file)")
		}
		return v, nil
	}
	name := o.Label
	switch o.Type {
	case "float", "int":
		n, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return "", fmt.Errorf("%s needs a number", name)
		}
		if o.Max > o.Min && (n < o.Min || n > o.Max) {
			return "", fmt.Errorf("%s must be between %g and %g", name, o.Min, o.Max)
		}
		if o.Type == "int" {
			if n != float64(int64(n)) {
				return "", fmt.Errorf("%s needs a whole number", name)
			}
			return strconv.FormatInt(int64(n), 10), nil
		}
		return strconv.FormatFloat(n, 'f', 6, 64), nil
	case "bool":
		switch strings.ToLower(v) {
		case "true":
			return "True", nil
		case "false":
			return "False", nil
		}
		return "", fmt.Errorf("%s is on or off", name)
	case "enum":
		for _, c := range o.Choices {
			if strings.EqualFold(c, v) {
				return c, nil
			}
		}
		return "", fmt.Errorf("%s must be one of %s", name, strings.Join(o.Choices, ", "))
	default: // string, secret
		if strings.ContainsAny(v, "\"\r\n") {
			return "", fmt.Errorf("%s can't contain quotes or line breaks", name)
		}
		if len(v) > 200 {
			return "", fmt.Errorf("%s is too long", name)
		}
		return `"` + v + `"`, nil
	}
}

// Apply writes the changes (from Plan). The old file is kept as <name>.warden-<time>.bak; the
// new one is written to a temporary file and renamed into place.
func (f *File) Apply(changes []Change) (string, error) {
	idx := map[string]int{}
	for n, p := range f.Pairs {
		idx[p.Key] = n
	}
	pairs := append([]Pair(nil), f.Pairs...)
	for _, c := range changes {
		o, known := Lookup(c.Key)
		raw, err := format(o, known, c.New)
		if err != nil {
			return "", err
		}
		if n, ok := idx[c.Key]; ok {
			pairs[n].Raw = raw
		} else {
			pairs = append(pairs, Pair{Key: c.Key, Raw: raw})
		}
	}
	parts := make([]string, len(pairs))
	for n, p := range pairs {
		parts[n] = p.Key + "=" + p.Raw
	}
	text := f.text[:f.start] + strings.Join(parts, ",") + f.text[f.end:]
	bak := f.Path + ".warden-" + time.Now().Format("20060102-150405") + ".bak"
	if err := os.WriteFile(bak, []byte(f.text), 0o644); err != nil {
		return "", fmt.Errorf("couldn't keep a copy of the old settings: %w", err)
	}
	tmp := f.Path + ".warden-tmp"
	if err := os.WriteFile(tmp, []byte(text), 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, f.Path); err != nil {
		return "", err
	}
	return bak, nil
}

// Validate checks one option name and value the way Plan does, without a file (for settings
// stored ahead of time, such as events).
func Validate(key, value string) error {
	if !keyRe.MatchString(key) {
		return fmt.Errorf("%q isn't an option name", key)
	}
	o, known := Lookup(key)
	if known && o.Locked {
		return fmt.Errorf("%s can't be changed by Warden", o.Label)
	}
	_, err := format(o, known, value)
	return err
}

// ValidateKnown is Validate for options Warden explains only (events and presets must not guess
// option names: a typo would never apply).
func ValidateKnown(key, value string) error {
	if _, ok := Lookup(key); !ok {
		return fmt.Errorf("%s isn't an option Warden knows (check the spelling on the Game settings page)", key)
	}
	return Validate(key, value)
}

// Same says two values mean the same setting (2 and 2.000000, true and True).
func Same(key, a, b string) bool {
	o, known := Lookup(key)
	fa, ea := format(o, known, a)
	fb, eb := format(o, known, b)
	if ea != nil || eb != nil { // e.g. a number outside Warden's range: compare as numbers if they are
		na, e1 := strconv.ParseFloat(strings.TrimSpace(a), 64)
		nb, e2 := strconv.ParseFloat(strings.TrimSpace(b), 64)
		if e1 == nil && e2 == nil {
			return na == nb
		}
		return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
	}
	return fa == fb
}
