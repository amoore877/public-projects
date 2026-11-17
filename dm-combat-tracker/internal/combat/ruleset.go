package combat

import (
	"fmt"
	"strings"
)

// DefaultRuleset is the ruleset of a new encounter, and of a state file
// saved before rulesets existed (RS1).
const DefaultRuleset = "5e"

// Ruleset is the per-encounter choice of game system (RS1). It still rolls
// no dice and automates no rules (R8); for now it only sets the condition
// preset. HP rules and duration anchoring join it in later milestones.
type Ruleset struct {
	Name string
	// Conditions is the preset list for the picker (RS8). Custom names are
	// always allowed.
	Conditions []string
}

var builtinRulesets = []Ruleset{
	{
		Name: "5e",
		Conditions: []string{
			"Blinded", "Charmed", "Deafened", "Exhaustion", "Frightened", "Grappled",
			"Incapacitated", "Invisible", "Paralyzed", "Petrified", "Poisoned", "Prone",
			"Restrained", "Stunned", "Unconscious", "Concentrating",
		},
	},
	{
		Name: "3.5e",
		// States that come from HP (disabled, dying, dead, staggered) are
		// left out; they are shown from HP, not picked (RS8).
		Conditions: []string{
			"Blinded", "Confused", "Cowering", "Dazed", "Dazzled", "Deafened",
			"Energy Drained", "Entangled", "Exhausted", "Fascinated", "Fatigued",
			"Flat-Footed", "Frightened", "Grappling", "Helpless", "Invisible",
			"Nauseated", "Panicked", "Paralyzed", "Petrified", "Pinned", "Prone",
			"Shaken", "Sickened", "Stable", "Stunned", "Turned", "Unconscious",
			"Concentrating",
		},
	},
}

// RulesetNames lists the built-in rulesets, default first.
func RulesetNames() []string {
	names := make([]string, len(builtinRulesets))
	for i, r := range builtinRulesets {
		names[i] = r.Name
	}
	return names
}

// LookupRuleset finds a built-in ruleset by name. Case doesn't matter and
// the trailing "e" is optional ("3.5", "5E").
func LookupRuleset(name string) (Ruleset, bool) {
	name = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(name)), "e")
	for _, r := range builtinRulesets {
		if strings.TrimSuffix(r.Name, "e") == name {
			return r, true
		}
	}
	return Ruleset{}, false
}

// Rules returns the encounter's ruleset. An empty or unknown name (which
// store.Load refuses) reads as the default.
func (e *Encounter) Rules() Ruleset {
	if r, ok := LookupRuleset(e.Ruleset); ok {
		return r
	}
	r, _ := LookupRuleset(DefaultRuleset)
	return r
}

// SetRuleset chooses the ruleset. Only allowed during Setup: once combat has
// started, HP already recorded would change meaning (RS1). Choosing the
// current ruleset again does nothing.
func (e *Encounter) SetRuleset(name string) ([]Event, error) {
	r, ok := LookupRuleset(name)
	if !ok {
		return nil, fmt.Errorf("%w: unknown ruleset %q (choose %s)", ErrInvalid, strings.TrimSpace(name), strings.Join(RulesetNames(), " or "))
	}
	from := e.Rules().Name
	if r.Name == from {
		e.Ruleset = r.Name
		return nil, nil
	}
	if e.Status != Setup {
		return nil, fmt.Errorf("%w; the ruleset can't change mid-fight", ErrNotSetup)
	}
	e.Ruleset = r.Name
	return []Event{RulesetChanged{EventMeta: e.meta(), From: from, To: r.Name}}, nil
}

// CanonicalCondition uses the ruleset's spelling of a preset condition for
// a case-insensitive match, and returns any other name as typed.
func (r Ruleset) CanonicalCondition(s string) string {
	for _, c := range r.Conditions {
		if strings.EqualFold(c, s) {
			return c
		}
	}
	return s
}
