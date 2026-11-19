package combat

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"unicode"
)

// ErrUnknownRuleset is returned for a ruleset name that is neither built in
// nor loaded from a file (RS9).
var ErrUnknownRuleset = errors.New("unknown ruleset")

// The built-in rulesets are written in the same form as a ruleset file
// (RS9, D37). The 3.5e condition list leaves out the states that come from
// HP (disabled, dying, dead, staggered); they are shown from HP, not picked
// (RS8).
//
//go:embed rulesets/5e.json rulesets/3.5e.json
var builtinFiles embed.FS

// builtinOrder is the order of RulesetNames: the default first.
var builtinOrder = []string{"rulesets/5e.json", "rulesets/3.5e.json"}

var builtinRulesets = mustParseBuiltins()

// customRulesets are the ones loaded from files at start-up (UseRulesets).
var customRulesets []Ruleset

func mustParseBuiltins() []Ruleset {
	rs := make([]Ruleset, len(builtinOrder))
	for i, path := range builtinOrder {
		data, err := builtinFiles.ReadFile(path)
		if err == nil {
			rs[i], err = ParseRuleset(data)
		}
		if err != nil {
			panic(fmt.Sprintf("built-in ruleset %s: %v", path, err))
		}
	}
	return rs
}

// Limits a ruleset file must keep to so the table still fits at 80 columns.
const (
	maxRulesetName = 10 // the title line
	maxStateLabel  = 5  // the HP column: "DSBLD 0/22" is 11 wide
)

// ParseRuleset reads and checks a ruleset file (RS9). Unknown fields are
// refused, so a misspelt key fails rather than being ignored. The error
// names the first problem found.
func ParseRuleset(data []byte) (Ruleset, error) {
	var r Ruleset
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&r); err != nil {
		return Ruleset{}, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return Ruleset{}, errors.New("unexpected data after the ruleset")
	}
	if err := r.validate(); err != nil {
		return Ruleset{}, err
	}
	return r, nil
}

func (r *Ruleset) validate() error {
	switch {
	case r.Name == "":
		return errors.New(`"name" is required`)
	case len([]rune(r.Name)) > maxRulesetName:
		return fmt.Errorf("name %q is longer than %d characters", r.Name, maxRulesetName)
	case strings.IndexFunc(r.Name, unicode.IsSpace) >= 0:
		return fmt.Errorf("name %q has a space in it", r.Name)
	case r.HPFloor > 0:
		return fmt.Errorf(`"hpFloor" is %d; it can't be above 0`, r.HPFloor)
	}
	for i := range r.States {
		s := &r.States[i]
		s.Name = strings.TrimSpace(s.Name)
		s.Tag = strings.TrimSpace(s.Tag)
		switch {
		case s.Name == "":
			return fmt.Errorf("state %d has no name", i+1)
		case IsNonlethalState(strings.ToLower(s.Name)):
			return fmt.Errorf("state %q is reserved for nonlethal damage", s.Name)
		case len([]rune(s.Label())) > maxStateLabel && s.Tag == "":
			return fmt.Errorf("state %q needs a \"tag\" of at most %d characters for the HP column", s.Name, maxStateLabel)
		case len([]rune(s.Label())) > maxStateLabel:
			return fmt.Errorf("state %q has tag %q; it can be at most %d characters", s.Name, s.Tag, maxStateLabel)
		case s.Min != nil && s.Max != nil && *s.Min > *s.Max:
			return fmt.Errorf("state %q has min %d above max %d", s.Name, *s.Min, *s.Max)
		case s.Max != nil && *s.Max < r.HPFloor:
			return fmt.Errorf("state %q is below the HP floor %d, so it can't happen", s.Name, r.HPFloor)
		}
		for _, o := range r.States[:i] {
			if strings.EqualFold(o.Name, s.Name) {
				return fmt.Errorf("state %q is listed twice", s.Name)
			}
			if overlap(&o, s) {
				return fmt.Errorf("states %q and %q cover some of the same HP", o.Name, s.Name)
			}
		}
	}
	for i, c := range r.Conditions {
		c = strings.TrimSpace(c)
		if c == "" {
			return fmt.Errorf("condition %d is blank", i+1)
		}
		if slices.ContainsFunc(r.Conditions[:i], func(o string) bool { return strings.EqualFold(o, c) }) {
			return fmt.Errorf("condition %q is listed twice", c)
		}
		r.Conditions[i] = c
	}
	return nil
}

// overlap reports whether two states' HP ranges share a value; a nil bound
// is open.
func overlap(a, b *HealthState) bool {
	lo, hi := a.Min, a.Max
	if b.Min != nil && (lo == nil || *b.Min > *lo) {
		lo = b.Min
	}
	if b.Max != nil && (hi == nil || *b.Max < *hi) {
		hi = b.Max
	}
	return lo == nil || hi == nil || *lo <= *hi
}

// rulesetKey is the form names are matched in: case doesn't matter and the
// trailing "e" is optional ("3.5", "5E").
func rulesetKey(name string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(name)), "e")
}

// UseRulesets replaces the rulesets loaded from files. A name that matches
// a built-in or another file's is refused, and nothing changes.
func UseRulesets(rs []Ruleset) error {
	seen := map[string]string{}
	for _, r := range builtinRulesets {
		seen[rulesetKey(r.Name)] = "the built-in " + r.Name
	}
	for _, r := range rs {
		if other, ok := seen[rulesetKey(r.Name)]; ok {
			return fmt.Errorf("ruleset %q has the same name as %s", r.Name, other)
		}
		seen[rulesetKey(r.Name)] = "ruleset " + r.Name
	}
	customRulesets = slices.Clone(rs)
	return nil
}

func allRulesets() []Ruleset {
	return append(slices.Clip(builtinRulesets), customRulesets...)
}

// RulesetNames lists the rulesets: the built-ins, default first, then the
// ones loaded from files.
func RulesetNames() []string {
	var names []string
	for _, r := range allRulesets() {
		names = append(names, r.Name)
	}
	return names
}

// LookupRuleset finds a ruleset by name. Case doesn't matter and the
// trailing "e" is optional ("3.5", "5E").
func LookupRuleset(name string) (Ruleset, bool) {
	key := rulesetKey(name)
	for _, r := range allRulesets() {
		if rulesetKey(r.Name) == key {
			return r, true
		}
	}
	return Ruleset{}, false
}

// UnknownRulesetError says a name isn't a ruleset and lists the choices.
func UnknownRulesetError(name string) error {
	return fmt.Errorf("%w %q (choose %s)", ErrUnknownRuleset, strings.TrimSpace(name), strings.Join(RulesetNames(), " or "))
}
