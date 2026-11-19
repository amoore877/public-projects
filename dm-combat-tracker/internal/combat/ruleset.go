package combat

import (
	"fmt"
	"strings"
)

// DefaultRuleset is the ruleset of a new encounter, and of a state file
// saved before rulesets existed (RS1).
const DefaultRuleset = "5e"

// Ruleset is the per-encounter choice of game system (RS1). It still rolls
// no dice and automates no rules (R8); it sets the HP floor, the health
// states, nonlethal damage, where sourced effects count down and the
// condition preset. Built-ins and ruleset files share this form (RS9).
type Ruleset struct {
	Name string `json:"name"`
	// HPFloor is the lowest HP a combatant can have (RS2): 0 in 5e, -10 in
	// 3.5e, where anything lower is equally dead.
	HPFloor int `json:"hpFloor"`
	// States are the named health states by HP range (RS3). The first one
	// whose range holds the HP applies; none means the creature is fine.
	States []HealthState `json:"states"`
	// Nonlethal tracks a running nonlethal damage total per combatant, with
	// the staggered and unconscious states it causes (RS4).
	Nonlethal bool `json:"nonlethal"`
	// FixedAnchor pins a sourced effect to the initiative count it started
	// on (RS7, G7, D32): it counts down on its own row, which sorts just
	// before its source on a tie, instead of on its source's turn. Changing
	// the source's initiative then asks whether its effects move too (D33).
	FixedAnchor bool `json:"fixedAnchor"`
	// Conditions is the preset list for the picker (RS8). Custom names are
	// always allowed.
	Conditions []string `json:"conditions"`
}

// HealthState is a named state a combatant is in while its HP is within
// [Min, Max]; a nil bound is open.
type HealthState struct {
	Name   string `json:"name"`
	Tag    string `json:"tag"` // short form for the HP column; defaults to Name
	Min    *int   `json:"min"`
	Max    *int   `json:"max"`
	Acts   bool   `json:"acts"`   // still takes its turns normally (3.5 disabled)
	Remind bool   `json:"remind"` // the status line points it out when the creature's turn starts
}

func (s *HealthState) holds(hp int) bool {
	return (s.Min == nil || hp >= *s.Min) && (s.Max == nil || hp <= *s.Max)
}

// Label is the state's short form for the table, in capitals.
func (s *HealthState) Label() string {
	if s.Tag != "" {
		return strings.ToUpper(s.Tag)
	}
	return strings.ToUpper(s.Name)
}

// State returns the health state for an HP value, or nil for none.
func (r Ruleset) State(hp int) *HealthState {
	for i := range r.States {
		if r.States[i].holds(hp) {
			return &r.States[i]
		}
	}
	return nil
}

// The states nonlethal damage causes (RS4). They apply only while HP itself
// puts the creature in no state.
var (
	staggered   = HealthState{Name: "staggered", Tag: "stagr", Acts: true}
	unconscious = HealthState{Name: "unconscious", Tag: "uncon"}
)

// IsNonlethalState reports whether a state name comes from nonlethal damage
// rather than HP.
func IsNonlethalState(name string) bool {
	return name == staggered.Name || name == unconscious.Name
}

// HealthState returns the combatant's health state under the encounter's
// ruleset, or nil if it has none. A state from HP wins over one from
// nonlethal damage.
func (e *Encounter) HealthState(c *Combatant) *HealthState {
	r := e.Rules()
	if s := r.State(c.HP); s != nil {
		return s
	}
	if r.Nonlethal && c.Nonlethal > 0 {
		switch {
		case c.Nonlethal > c.HP:
			return &unconscious
		case c.Nonlethal == c.HP:
			return &staggered
		}
	}
	return nil
}

// stateChange reports a move between health states since before, if any.
func (e *Encounter) stateChange(c *Combatant, before *HealthState) []Event {
	from, to := stateName(before), stateName(e.HealthState(c))
	if from == to {
		return nil
	}
	return []Event{HealthStateChanged{EventMeta: e.meta(), ID: c.ID, Name: c.Name, From: from, To: to, HP: c.HP, Nonlethal: c.Nonlethal}}
}

func stateName(s *HealthState) string {
	if s == nil {
		return ""
	}
	return s.Name
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
		return nil, fmt.Errorf("%w: %w", ErrInvalid, UnknownRulesetError(name))
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
